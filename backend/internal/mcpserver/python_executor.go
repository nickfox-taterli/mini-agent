package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pythonSessionInitInput struct {
	SessionID     string `json:"session_id,omitempty" jsonschema:"会话 ID, 可选. 不传则自动生成"`
	PythonVersion string `json:"python_version,omitempty" jsonschema:"Python 版本, 可选 3.10/3.11/3.12, 默认 3.11"`
}

type pythonSessionInitOutput struct {
	SessionID    string `json:"session_id" jsonschema:"会话 ID"`
	Container    string `json:"container" jsonschema:"容器名称"`
	WorkspaceDir string `json:"workspace_dir" jsonschema:"宿主机共享目录绝对路径"`
	PythonImage  string `json:"python_image" jsonschema:"使用的 Python 镜像"`
	Created      bool   `json:"created" jsonschema:"是否为新建会话"`
}

type pythonInstallPackagesInput struct {
	SessionID      string   `json:"session_id" jsonschema:"会话 ID"`
	Packages       []string `json:"packages" jsonschema:"需要安装的包列表, 例如 ['numpy','pandas==2.2.0']"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty" jsonschema:"安装超时秒数, 默认 120, 最大 600"`
}

type pythonInstallPackagesOutput struct {
	SessionID   string `json:"session_id"`
	ExitCode    int    `json:"exit_code"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	DurationMs  int64  `json:"duration_ms"`
	InstalledTo string `json:"installed_to"`
}

type pythonRunCodeInput struct {
	SessionID      string   `json:"session_id" jsonschema:"会话 ID"`
	Code           string   `json:"code,omitempty" jsonschema:"Python 代码内容, 与 file_path 二选一"`
	FilePath       string   `json:"file_path,omitempty" jsonschema:"工作目录相对文件路径, 与 code 二选一"`
	Args           []string `json:"args,omitempty" jsonschema:"传递给脚本的参数"`
	Stdin          string   `json:"stdin,omitempty" jsonschema:"标准输入内容"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty" jsonschema:"执行超时秒数, 默认 120, 最大 600"`
}

type pythonRunCodeOutput struct {
	SessionID    string   `json:"session_id"`
	ExitCode     int      `json:"exit_code"`
	Stdout       string   `json:"stdout"`
	Stderr       string   `json:"stderr"`
	DurationMs   int64    `json:"duration_ms"`
	Artifacts    []string `json:"artifacts"`
	ExecutedFile string   `json:"executed_file"`
}

type pythonSessionCloseInput struct {
	SessionID string `json:"session_id" jsonschema:"会话 ID"`
}

type pythonSessionCloseOutput struct {
	SessionID string `json:"session_id"`
	Closed    bool   `json:"closed"`
}

func pythonSessionInit(_ context.Context, _ *mcp.CallToolRequest, in pythonSessionInitInput) (*mcp.CallToolResult, pythonSessionInitOutput, error) {
	out, err := pythonSessionInitLocal(in)
	if err != nil {
		return nil, pythonSessionInitOutput{}, err
	}
	return nil, out, nil
}

func pythonSessionInitLocal(in pythonSessionInitInput) (pythonSessionInitOutput, error) {
	if dockerRT == nil {
		return pythonSessionInitOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	version := strings.TrimSpace(in.PythonVersion)
	if version == "" {
		version = "3.11"
	}
	if version != "3.10" && version != "3.11" && version != "3.12" {
		return pythonSessionInitOutput{}, fmt.Errorf("unsupported python_version: %s", version)
	}
	image := "python:" + version + "-slim"
	s, created, err := dockerRT.ensureSession("python", in.SessionID, image)
	if err != nil {
		return pythonSessionInitOutput{}, err
	}
	return pythonSessionInitOutput{
		SessionID:    s.ID,
		Container:    s.ContainerName,
		WorkspaceDir: s.WorkspaceDir,
		PythonImage:  s.Image,
		Created:      created,
	}, nil
}

func pythonInstallPackages(_ context.Context, _ *mcp.CallToolRequest, in pythonInstallPackagesInput) (*mcp.CallToolResult, pythonInstallPackagesOutput, error) {
	out, err := pythonInstallPackagesLocal(in)
	if err != nil {
		return nil, out, err
	}
	return nil, out, nil
}

func pythonInstallPackagesLocal(in pythonInstallPackagesInput) (pythonInstallPackagesOutput, error) {
	if dockerRT == nil {
		return pythonInstallPackagesOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return pythonInstallPackagesOutput{}, fmt.Errorf("session_id is required")
	}
	s, err := dockerRT.getSession(sessionID)
	if err != nil {
		return pythonInstallPackagesOutput{}, err
	}
	if s.Kind != "python" {
		return pythonInstallPackagesOutput{}, fmt.Errorf("session is not python kind")
	}
	res, installErr := dockerRT.installPythonPackages(s, in.Packages, in.TimeoutSeconds)
	out := pythonInstallPackagesOutput{
		SessionID:   s.ID,
		ExitCode:    res.ExitCode,
		Stdout:      res.Stdout,
		Stderr:      res.Stderr,
		DurationMs:  res.DurationMs,
		InstalledTo: filepath.ToSlash(filepath.Join(s.WorkspaceDir, ".deps")),
	}
	if installErr != nil {
		return out, fmt.Errorf("pip install failed with exit_code=%d", out.ExitCode)
	}
	return out, nil
}

func pythonRunCode(_ context.Context, _ *mcp.CallToolRequest, in pythonRunCodeInput) (*mcp.CallToolResult, pythonRunCodeOutput, error) {
	out, err := pythonRunCodeLocal(in)
	if err != nil {
		return nil, out, err
	}
	return nil, out, nil
}

func pythonRunCodeLocal(in pythonRunCodeInput) (pythonRunCodeOutput, error) {
	if dockerRT == nil {
		return pythonRunCodeOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return pythonRunCodeOutput{}, fmt.Errorf("session_id is required")
	}
	s, err := dockerRT.getSession(sessionID)
	if err != nil {
		return pythonRunCodeOutput{}, err
	}
	if s.Kind != "python" {
		return pythonRunCodeOutput{}, fmt.Errorf("session is not python kind")
	}

	executedFile := ""
	if strings.TrimSpace(in.Code) != "" && strings.TrimSpace(in.FilePath) != "" {
		return pythonRunCodeOutput{}, fmt.Errorf("code and file_path are mutually exclusive")
	}
	if strings.TrimSpace(in.Code) == "" && strings.TrimSpace(in.FilePath) == "" {
		return pythonRunCodeOutput{}, fmt.Errorf("either code or file_path is required")
	}
	if strings.TrimSpace(in.Code) != "" {
		executedFile = "main.py"
		target := filepath.Join(s.WorkspaceDir, executedFile)
		if err := os.WriteFile(target, []byte(in.Code), 0o644); err != nil {
			return pythonRunCodeOutput{}, fmt.Errorf("write python code file: %w", err)
		}
	} else {
		clean, err := sanitizeRelativeFilePath(in.FilePath)
		if err != nil {
			return pythonRunCodeOutput{}, err
		}
		executedFile = clean
		target := filepath.Join(s.WorkspaceDir, clean)
		if _, err := os.Stat(target); err != nil {
			return pythonRunCodeOutput{}, fmt.Errorf("file_path not found: %s", clean)
		}
	}

	args := []string{"env", "PYTHONPATH=/workspace/.deps", "python", executedFile}
	args = append(args, in.Args...)
	res, runErr := dockerRT.execInSession(s, in.TimeoutSeconds, in.Stdin, args...)
	artifacts, _ := dockerRT.listArtifacts(s)
	out := pythonRunCodeOutput{
		SessionID:    s.ID,
		ExitCode:     res.ExitCode,
		Stdout:       res.Stdout,
		Stderr:       res.Stderr,
		DurationMs:   res.DurationMs,
		Artifacts:    artifacts,
		ExecutedFile: executedFile,
	}
	if runErr != nil {
		return out, fmt.Errorf("python execution failed with exit_code=%d", out.ExitCode)
	}
	return out, nil
}

func pythonSessionClose(_ context.Context, _ *mcp.CallToolRequest, in pythonSessionCloseInput) (*mcp.CallToolResult, pythonSessionCloseOutput, error) {
	out, err := pythonSessionCloseLocal(in)
	if err != nil {
		return nil, pythonSessionCloseOutput{}, err
	}
	return nil, out, nil
}

func pythonSessionCloseLocal(in pythonSessionCloseInput) (pythonSessionCloseOutput, error) {
	if dockerRT == nil {
		return pythonSessionCloseOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return pythonSessionCloseOutput{}, fmt.Errorf("session_id is required")
	}
	closed, err := dockerRT.closeSession(sessionID)
	if err != nil {
		return pythonSessionCloseOutput{}, err
	}
	return pythonSessionCloseOutput{SessionID: sessionID, Closed: closed}, nil
}

func sanitizeRelativeFilePath(v string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(v))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid file_path")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "..\\") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("file_path must be relative and stay in workspace")
	}
	return filepath.ToSlash(clean), nil
}
