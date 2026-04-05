package mcpserver

import (
	"context"
	"net/http"
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
