package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type codeSessionInitInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"会话 ID, 可选. 不传则自动生成"`
}

type codeSessionInitOutput struct {
	SessionID    string `json:"session_id"`
	Container    string `json:"container"`
	WorkspaceDir string `json:"workspace_dir"`
	Image        string `json:"image"`
	Created      bool   `json:"created"`
}

type codeRunInput struct {
	SessionID      string   `json:"session_id" jsonschema:"会话 ID"`
	Language       string   `json:"language" jsonschema:"语言: shell|c|cpp|java|php"`
	SourceCode     string   `json:"source_code" jsonschema:"源代码"`
	Stdin          string   `json:"stdin,omitempty"`
	Args           []string `json:"args,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

type codeRunOutput struct {
	SessionID   string   `json:"session_id"`
	Language    string   `json:"language"`
	ExitCode    int      `json:"exit_code"`
	Stdout      string   `json:"stdout"`
	Stderr      string   `json:"stderr"`
	DurationMs  int64    `json:"duration_ms"`
	Artifacts   []string `json:"artifacts"`
	CommandLine string   `json:"command_line"`
}

type codeSessionCloseInput struct {
	SessionID string `json:"session_id"`
}

type codeSessionCloseOutput struct {
	SessionID string `json:"session_id"`
	Closed    bool   `json:"closed"`
}

func codeSessionInit(_ context.Context, _ *mcp.CallToolRequest, in codeSessionInitInput) (*mcp.CallToolResult, codeSessionInitOutput, error) {
	out, err := codeSessionInitLocal(in)
	if err != nil {
		return nil, codeSessionInitOutput{}, err
	}
	return nil, out, nil
}

func codeSessionInitLocal(in codeSessionInitInput) (codeSessionInitOutput, error) {
	if dockerRT == nil {
		return codeSessionInitOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	s, created, err := dockerRT.ensureSession("code", in.SessionID, "mcp-code-runner:latest")
	if err != nil {
		return codeSessionInitOutput{}, err
	}
	return codeSessionInitOutput{
		SessionID:    s.ID,
		Container:    s.ContainerName,
		WorkspaceDir: s.WorkspaceDir,
		Image:        "mcp-code-runner:latest",
		Created:      created,
	}, nil
}

func codeRun(_ context.Context, _ *mcp.CallToolRequest, in codeRunInput) (*mcp.CallToolResult, codeRunOutput, error) {
	out, err := codeRunLocal(in)
	if err != nil {
		return nil, out, err
	}
	return nil, out, nil
}

func codeRunLocal(in codeRunInput) (codeRunOutput, error) {
	if dockerRT == nil {
		return codeRunOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return codeRunOutput{}, fmt.Errorf("session_id is required")
	}
	lang := strings.ToLower(strings.TrimSpace(in.Language))
	if lang == "" {
		return codeRunOutput{}, fmt.Errorf("language is required")
	}
	if strings.TrimSpace(in.SourceCode) == "" {
		return codeRunOutput{}, fmt.Errorf("source_code is required")
	}
	s, err := dockerRT.getSession(sessionID)
	if err != nil {
		return codeRunOutput{}, err
	}
	if s.Kind != "code" {
		return codeRunOutput{}, fmt.Errorf("session is not code kind")
	}

	meta, err := codeLanguageMeta(lang)
	if err != nil {
		return codeRunOutput{}, err
	}
	target := filepath.Join(s.WorkspaceDir, meta.FileName)
	if err := os.WriteFile(target, []byte(in.SourceCode), 0o644); err != nil {
		return codeRunOutput{}, fmt.Errorf("write source file: %w", err)
	}
	cmd := meta.Command
	if len(in.Args) > 0 {
		cmd = cmd + " " + shellJoin(in.Args)
	}
	res, runErr := dockerRT.execInSession(s, in.TimeoutSeconds, in.Stdin, "bash", "-lc", cmd)
	artifacts, _ := dockerRT.listArtifacts(s)
	out := codeRunOutput{
		SessionID:   s.ID,
		Language:    lang,
		ExitCode:    res.ExitCode,
		Stdout:      res.Stdout,
		Stderr:      res.Stderr,
		DurationMs:  res.DurationMs,
		Artifacts:   artifacts,
		CommandLine: cmd,
	}
	if runErr != nil {
		return out, fmt.Errorf("code run failed with exit_code=%d", out.ExitCode)
	}
	return out, nil
}

func codeSessionClose(_ context.Context, _ *mcp.CallToolRequest, in codeSessionCloseInput) (*mcp.CallToolResult, codeSessionCloseOutput, error) {
	out, err := codeSessionCloseLocal(in)
	if err != nil {
		return nil, codeSessionCloseOutput{}, err
	}
	return nil, out, nil
}

func codeSessionCloseLocal(in codeSessionCloseInput) (codeSessionCloseOutput, error) {
	if dockerRT == nil {
		return codeSessionCloseOutput{}, fmt.Errorf("docker runtime is disabled")
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return codeSessionCloseOutput{}, fmt.Errorf("session_id is required")
	}
	closed, err := dockerRT.closeSession(sessionID)
	if err != nil {
		return codeSessionCloseOutput{}, err
	}
	return codeSessionCloseOutput{SessionID: sessionID, Closed: closed}, nil
}

type codeLangMeta struct {
	FileName string
	Command  string
}

func codeLanguageMeta(lang string) (codeLangMeta, error) {
	switch lang {
	case "shell":
		return codeLangMeta{FileName: "main.sh", Command: "chmod +x main.sh && ./main.sh"}, nil
	case "c":
		return codeLangMeta{FileName: "main.c", Command: "gcc main.c -O2 -o main && ./main"}, nil
	case "cpp":
		return codeLangMeta{FileName: "main.cpp", Command: "g++ main.cpp -O2 -std=c++17 -o main && ./main"}, nil
	case "java":
		return codeLangMeta{FileName: "Main.java", Command: "javac -encoding UTF-8 Main.java && java Main"}, nil
	case "php":
		return codeLangMeta{FileName: "main.php", Command: "php main.php"}, nil
	default:
		return codeLangMeta{}, fmt.Errorf("unsupported language: %s", lang)
	}
}

func shellJoin(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.ReplaceAll(a, "'", "'\"'\"'")
		out = append(out, "'"+a+"'")
	}
	return strings.Join(out, " ")
}
