package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxImageSize = 20 * 1024 * 1024 // 20MB

type minimaxUnderstandImageInput struct {
	ImageURL string `json:"image_url" jsonschema:"Image URL (http/https) or local file path to analyze"`
	Prompt   string `json:"prompt" jsonschema:"What to analyze or describe about the image"`
}

func minimaxUnderstandImage(_ context.Context, _ *mcp.CallToolRequest, in minimaxUnderstandImageInput) (*mcp.CallToolResult, minimaxUnderstandImageInput, error) {
	result, err := minimaxUnderstandImageLocal(in.ImageURL, in.Prompt)
	if err != nil {
		return nil, minimaxUnderstandImageInput{}, err
	}
	b, _ := json.Marshal(result)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, in, nil
}

func minimaxUnderstandImageLocal(imageURL, prompt string) (map[string]any, error) {
	if minimaxKeyMgr == nil {
		return nil, fmt.Errorf("minimax tools not initialized: no API keys configured")
	}
	if imageURL == "" {
		return nil, fmt.Errorf("image_url is required")
	}
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	dataURL, err := resolveImageToBase64DataURL(imageURL)
	if err != nil {
		return nil, fmt.Errorf("resolve image: %w", err)
	}

	return doMiniMaxImageUnderstand(dataURL, prompt)
}

func doMiniMaxImageUnderstand(dataURL, prompt string) (map[string]any, error) {
	key := minimaxKeyMgr.NextKey()
	url := minimaxAPIHost + "/v1/coding_plan/vlm"

	body := map[string]any{
		"prompt":    prompt,
		"image_url": dataURL,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal vlm body: %w", err)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	resp, err := doPostWithKey(httpClient, url, key, bodyJSON)
	if err != nil {
		return nil, err
	}

	// 401/403 时重试一次
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		minimaxKeyMgr.ReportError(key)
		log.Printf("[minimax-vlm] key auth failed (status %d), retrying with next key", resp.StatusCode)
		resp.Body.Close()

		key = minimaxKeyMgr.NextKey()
		resp, err = doPostWithKey(httpClient, url, key, bodyJSON)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read vlm response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vlm API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse vlm response: %w", err)
	}

	// 检查 API 级错误码
	baseResp, _ := result["base_resp"].(map[string]any)
	if statusCode, _ := baseResp["status_code"].(float64); statusCode != 0 {
		statusMsg, _ := baseResp["status_msg"].(string)
		return nil, fmt.Errorf("vlm API error: status_code=%.0f, status_msg=%s", statusCode, statusMsg)
	}

	content, _ := result["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("vlm API returned empty content")
	}

	return map[string]any{
		"content":   content,
		"image_url": dataURL[:min(50, len(dataURL))] + "...",
		"prompt":    prompt,
	}, nil
}

// doPostWithKey 发送带认证头的 POST 请求.
func doPostWithKey(client *http.Client, url, key string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("MM-API-Source", "Minimax-MCP")

	log.Printf("[minimax-vlm] POST %s prompt_len=%d body_len=%d", url, len(body), len(body))
	return client.Do(req)
}

// resolveImageToBase64DataURL 将图片 URL/路径解析为 base64 data URL.
// 优先级:
// 1. data: 前缀 -> 直接透传
// 2. 前端 URL (http://127.0.0.1:18889/...) -> 映射为本地路径直接读取
// 3. 外部 HTTP URL -> 下载后编码
// 4. 本地路径 -> 直接读取
func resolveImageToBase64DataURL(imageURL string) (string, error) {
	// 已是 data URL, 直接透传
	if strings.HasPrefix(imageURL, "data:") {
		return imageURL, nil
	}

	// 前端 URL -> 映射为本地路径
	if strings.HasPrefix(imageURL, frontendURL+"/") {
		localPath, err := frontendURLToLocalPath(imageURL)
		if err == nil {
			return readImageAsDataURL(localPath)
		}
		// 映射失败, 回退到 HTTP 下载
		log.Printf("[minimax-vlm] frontend URL mapping failed: %v, falling back to HTTP download", err)
	}

	// HTTP/HTTPS URL -> 下载后编码
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return downloadImageAsDataURL(imageURL)
	}

	// 本地路径 -> 直接读取
	return readImageAsDataURL(imageURL)
}

// frontendURLToLocalPath 将前端 URL 映射为本地文件路径.
// 例如: http://127.0.0.1:18889/upload/2026/04/15/abc.png -> frontend/upload/2026/04/15/abc.png
func frontendURLToLocalPath(fileURL string) (string, error) {
	// 去掉前端 URL 前缀, 得到相对路径
	relPath := strings.TrimPrefix(fileURL, frontendURL+"/")

	// 解析前端目录
	frontendDir, err := resolveFrontendDir([]string{
		filepath.Join("..", "frontend"),
		filepath.Join("..", "..", "frontend"),
		"frontend",
	})
	if err != nil {
		return "", fmt.Errorf("resolve frontend dir: %w", err)
	}

	localPath := filepath.Join(frontendDir, relPath)
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("file not found: %s", localPath)
	}

	log.Printf("[minimax-vlm] mapped frontend URL to local path: %s -> %s", fileURL, localPath)
	return localPath, nil
}

// downloadImageAsDataURL 下载 HTTP 图片并编码为 base64 data URL.
func downloadImageAsDataURL(imageURL string) (string, error) {
	log.Printf("[minimax-vlm] downloading image from URL: %s", imageURL)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		return "", fmt.Errorf("read image data: %w", err)
	}
	if len(data) > maxImageSize {
		return "", fmt.Errorf("image too large: %d bytes (max %d)", len(data), maxImageSize)
	}

	format := detectImageFormatFromContentType(resp.Header.Get("Content-Type"))
	if format == "" {
		format = "jpeg"
	}

	return buildDataURL(format, data), nil
}

// readImageAsDataURL 读取本地图片文件并编码为 base64 data URL.
func readImageAsDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read image file: %w", err)
	}
	if len(data) > maxImageSize {
		return "", fmt.Errorf("image too large: %d bytes (max %d)", len(data), maxImageSize)
	}

	format := detectImageFormatFromExt(path)
	return buildDataURL(format, data), nil
}

// buildDataURL 构建 data:image/{format};base64,{data} 字符串.
func buildDataURL(format string, data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:image/%s;base64,%s", format, encoded)
}

// detectImageFormatFromContentType 从 Content-Type 头检测图片格式.
func detectImageFormatFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	if strings.Contains(ct, "png") {
		return "png"
	}
	if strings.Contains(ct, "webp") {
		return "webp"
	}
	if strings.Contains(ct, "jpeg") || strings.Contains(ct, "jpg") {
		return "jpeg"
	}
	return ""
}

// detectImageFormatFromExt 从文件扩展名检测图片格式.
func detectImageFormatFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "png"
	case ".webp":
		return "webp"
	case ".jpg", ".jpeg":
		return "jpeg"
	default:
		return "jpeg"
	}
}
