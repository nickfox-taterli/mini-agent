package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type systemTimeOutput struct {
	NowUnix      int64  `json:"now_unix" jsonschema:"当前 Unix 时间戳,单位秒"`
	NowRFC3339   string `json:"now_rfc3339" jsonschema:"当前时间, RFC3339 格式"`
	NowLocal     string `json:"now_local" jsonschema:"当前本地时间字符串"`
	TimezoneName string `json:"timezone_name" jsonschema:"系统时区名称"`
}

func NewHTTPHandler() http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "agent-time-mcp",
		Version: "v1.0.0",
	}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_system_time",
		Description: "Get current system time.",
	}, getSystemTime)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "write_frontend_temp_file",
		Description: "Write a generated file into frontend temporary directory. Use content_base64 for binary files.",
	}, writeFrontendTempFile)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "minimax-xlsx",
		Description: "Create a real .xlsx file in frontend temporary directory with optional sample data and current system time.",
	}, minimaxXlsx)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_skill_bash",
		Description: "Run a bash command inside backend/skills/<skill_name>. Use env SKILL_DIR and FRONTEND_TMP_DIR for paths.",
	}, runSkillBash)

	// 使用 stateless 模式, 让前端用最少请求即可调用工具.
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return srv
	}, &mcp.StreamableHTTPOptions{
		JSONResponse: true,
		Stateless:    true,
	})
}

func getSystemTime(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemTimeOutput, error) {
	out, err := getSystemTimeOutput()
	if err != nil {
		return nil, systemTimeOutput{}, err
	}
	return nil, out, nil
}

func getSystemTimeOutput() (systemTimeOutput, error) {
	now := time.Now()
	return systemTimeOutput{
		NowUnix:      now.Unix(),
		NowRFC3339:   now.Format(time.RFC3339),
		NowLocal:     now.Format("2006-01-02 15:04:05"),
		TimezoneName: now.Location().String(),
	}, nil
}

type writeFrontendTempFileInput struct {
	FileName      string `json:"file_name" jsonschema:"Relative file name under frontend temp directory"`
	TextContent   string `json:"text_content,omitempty" jsonschema:"Text content to write"`
	ContentBase64 string `json:"content_base64,omitempty" jsonschema:"Base64 encoded bytes for binary files"`
	Overwrite     bool   `json:"overwrite,omitempty" jsonschema:"Overwrite existing file, default false"`
}

type writeFrontendTempFileOutput struct {
	Path      string `json:"path" jsonschema:"Absolute path of written file"`
	SizeBytes int    `json:"size_bytes" jsonschema:"Output file size in bytes"`
	Created   bool   `json:"created" jsonschema:"Whether file was newly created"`
}

func writeFrontendTempFile(_ context.Context, _ *mcp.CallToolRequest, in writeFrontendTempFileInput) (*mcp.CallToolResult, writeFrontendTempFileOutput, error) {
	out, err := writeFrontendTempFileToDisk(in)
	if err != nil {
		return nil, writeFrontendTempFileOutput{}, err
	}
	return nil, out, nil
}

func writeFrontendTempFileToDisk(in writeFrontendTempFileInput) (writeFrontendTempFileOutput, error) {
	fileName := strings.TrimSpace(in.FileName)
	if fileName == "" {
		return writeFrontendTempFileOutput{}, fmt.Errorf("file_name is required")
	}
	if filepath.IsAbs(fileName) {
		return writeFrontendTempFileOutput{}, fmt.Errorf("file_name must be relative")
	}

	cleanName := filepath.Clean(fileName)
	if strings.HasPrefix(cleanName, "..") || strings.Contains(cleanName, "../") || cleanName == "." {
		return writeFrontendTempFileOutput{}, fmt.Errorf("invalid file_name: path traversal is not allowed")
	}

	var data []byte
	if strings.TrimSpace(in.ContentBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.ContentBase64))
		if err != nil {
			return writeFrontendTempFileOutput{}, fmt.Errorf("decode content_base64: %w", err)
		}
		data = decoded
	} else {
		data = []byte(in.TextContent)
	}

	return writeFrontendTempBytes(cleanName, data, in.Overwrite)
}

func writeFrontendTempBytes(cleanName string, data []byte, overwrite bool) (writeFrontendTempFileOutput, error) {
	rootAbs, err := resolveFrontendTmpDir()
	if err != nil {
		return writeFrontendTempFileOutput{}, fmt.Errorf("resolve frontend tmp dir: %w", err)
	}

	targetPath := filepath.Join(rootAbs, cleanName)
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return writeFrontendTempFileOutput{}, fmt.Errorf("create target dir: %w", err)
	}

	if !overwrite {
		if _, err := os.Stat(targetPath); err == nil {
			return writeFrontendTempFileOutput{}, fmt.Errorf("target file already exists: %s", targetPath)
		}
	}

	created := true
	if _, err := os.Stat(targetPath); err == nil {
		created = false
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return writeFrontendTempFileOutput{}, fmt.Errorf("write file: %w", err)
	}

	return writeFrontendTempFileOutput{
		Path:      targetPath,
		SizeBytes: len(data),
		Created:   created,
	}, nil
}
