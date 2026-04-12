package mcpserver

import (
	"encoding/json"
	"fmt"
)

// toolDisplayNames 将原始工具名映射为用户友好的显示名.
var toolDisplayNames = map[string]string{
	"get_system_time":           "获取系统时间",
	"run_skill_bash":            "Skill 执行",
	"web_fetch":                 "网页抓取",
	"convert_local_path_to_url": "路径转换",
	"minimax_web_search":        "网络搜索",
	"minimax_understand_image":  "图片理解",
}

// ToolDisplayName 返回工具的友好显示名, 未匹配时返回原始名.
func ToolDisplayName(name string) string {
	if display, ok := toolDisplayNames[name]; ok {
		return display
	}
	return name
}

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
	case "run_skill_bash":
		var in runSkillBashInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid run_skill_bash arguments: %w", err)
			}
		}
		out, err := runSkillBashLocal(in)
		if err != nil {
			return map[string]any{
				"skill_dir":           out.SkillDir,
				"exit_code":           out.ExitCode,
				"stdout":              out.Stdout,
				"stderr":              out.Stderr,
				"duration_ms":         out.DurationMs,
				"frontend_upload_dir": out.FrontendUploadDir,
				"error":               err.Error(),
			}, nil
		}
		return map[string]any{
			"skill_dir":           out.SkillDir,
			"exit_code":           out.ExitCode,
			"stdout":              out.Stdout,
			"stderr":              out.Stderr,
			"duration_ms":         out.DurationMs,
			"frontend_upload_dir": out.FrontendUploadDir,
		}, nil
	case "web_fetch":
		var in webFetchInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid web_fetch arguments: %w", err)
			}
		}
		out := webFetchLocal(in)
		result := map[string]any{
			"url":            out.URL,
			"status_code":    out.StatusCode,
			"content_type":   out.ContentType,
			"content":        out.Content,
			"content_length": out.ContentLength,
			"truncated":      out.Truncated,
		}
		if out.Error != "" {
			result["error"] = out.Error
		}
		return result, nil
	case "convert_local_path_to_url":
		var in convertLocalPathToURLInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid convert_local_path_to_url arguments: %w", err)
			}
		}
		out, err := convertLocalPathToURLLocal(in)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"local_path": out.LocalPath,
			"url":        out.URL,
		}, nil
	case "minimax_web_search":
		var in minimaxWebSearchInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid minimax_web_search arguments: %w", err)
			}
		}
		return minimaxWebSearchLocal(in.Query)
	case "minimax_understand_image":
		var in minimaxUnderstandImageInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid minimax_understand_image arguments: %w", err)
			}
		}
		return minimaxUnderstandImageLocal(in.ImageURL, in.Prompt)
	default:
		return nil, fmt.Errorf("unsupported tool: %s", name)
	}
}
