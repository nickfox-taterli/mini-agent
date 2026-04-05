package mcpserver

import (
	"encoding/json"
	"fmt"
)

// ExecuteToolByJSON 按工具名执行本地 MCP 工具, 返回结构化结果.
func ExecuteToolByJSON(name string, rawArgs string) (map[string]any, error) {
	switch name {
	case "get_system_time":
		// 该工具当前无参数, 仅校验 JSON 基本合法性.
		if rawArgs != "" {
			var tmp map[string]any
			if err := json.Unmarshal([]byte(rawArgs), &tmp); err != nil {
				return nil, fmt.Errorf("invalid get_system_time arguments: %w", err)
			}
		}
		out, err := getSystemTimeOutput()
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"now_unix":      out.NowUnix,
			"now_rfc3339":   out.NowRFC3339,
			"now_local":     out.NowLocal,
			"timezone_name": out.TimezoneName,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported tool: %s", name)
	}
}

