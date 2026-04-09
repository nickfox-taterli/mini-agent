package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// frontendURL 是前端基础 URL, 由 InitConfig 在启动时设置.
// 工具函数使用此值将本地文件路径转换为可访问的 HTTP URL.
var frontendURL string

// uploadDateDir 是当前日期的上传子目录 (相对于 frontend 根目录).
// 格式: upload/YYYY/MM/DD
var uploadDateRelPath string

// InitConfig 在 main.go 启动时调用, 设置前端 URL.
// 必须在任何工具执行之前调用.
func InitConfig(url string) {
	frontendURL = url
	now := time.Now()
	uploadDateRelPath = fmt.Sprintf("upload/%s", now.Format("2006/01/02"))
}

// GetFrontendURL 返回当前前端基础 URL.
func GetFrontendURL() string {
	return frontendURL
}

// ResolveFrontendUploadDirExported 导出供 server 包使用的上传目录解析函数.
func ResolveFrontendUploadDirExported() (string, error) {
	return resolveFrontendUploadDir()
}

// resolveFrontendUploadDir 返回按日期分区的上传目录的绝对路径.
// 路径格式: <frontend>/upload/YYYY/MM/DD/
// 如果目录不存在会自动创建.
func resolveFrontendUploadDir() (string, error) {
	frontendDir, err := resolveFrontendDir([]string{
		filepath.Join("..", "frontend"),
		filepath.Join("..", "..", "frontend"),
		"frontend",
	})
	if err != nil {
		return "", err
	}
	uploadDir := filepath.Join(frontendDir, uploadDateRelPath)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}
	return uploadDir, nil
}

// BuildFileURL 根据目录和文件名构建完整的 HTTP URL.
func BuildFileURL(uploadDir string, filename string) string {
	// uploadDir 是由 resolveFrontendUploadDir 动态解析的路径, 格式: <frontend>/upload/YYYY/MM/DD/
	// 需要: http://127.0.0.1:18889/upload/2026/04/14/filename
	frontendDir, err := resolveFrontendDir([]string{
		filepath.Join("..", "frontend"),
		filepath.Join("..", "..", "frontend"),
		"frontend",
	})
	if err != nil {
		return ""
	}
	relPath, err := filepath.Rel(frontendDir, filepath.Join(uploadDir, filename))
	if err != nil {
		return ""
	}
	// 将路径分隔符转为 URL 分隔符
	urlPath := filepath.ToSlash(relPath)
	return frontendURL + "/" + urlPath
}
