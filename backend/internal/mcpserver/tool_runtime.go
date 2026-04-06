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
	case "write_frontend_temp_file":
		var in writeFrontendTempFileInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid write_frontend_temp_file arguments: %w", err)
			}
		}
		out, err := writeFrontendTempFileToDisk(in)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"path":       out.Path,
			"size_bytes": out.SizeBytes,
			"created":    out.Created,
		}, nil
	case "minimax-xlsx", "minimax_xlsx":
		var in minimaxXlsxInput
		if rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &in); err != nil {
				return nil, fmt.Errorf("invalid minimax-xlsx arguments: %w", err)
			}
		}
		out, err := minimaxXlsxToDisk(in)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"path":       out.Path,
			"size_bytes": out.SizeBytes,
			"created":    out.Created,
			"rows":       out.Rows,
			"columns":    out.Columns,
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
				"skill_dir":        out.SkillDir,
				"exit_code":        out.ExitCode,
				"stdout":           out.Stdout,
				"stderr":           out.Stderr,
				"duration_ms":      out.DurationMs,
				"frontend_tmp_dir": out.FrontendTmpDir,
				"error":            err.Error(),
			}, nil
		}
		return map[string]any{
			"skill_dir":        out.SkillDir,
			"exit_code":        out.ExitCode,
			"stdout":           out.Stdout,
			"stderr":           out.Stderr,
			"duration_ms":      out.DurationMs,
			"frontend_tmp_dir": out.FrontendTmpDir,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported tool: %s", name)
	}
}
