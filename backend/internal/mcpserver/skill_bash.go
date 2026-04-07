package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type runSkillBashInput struct {
	SkillName      string `json:"skill_name" jsonschema:"Skill directory name under backend/skills, for example minimax-xlsx"`
	Command        string `json:"command" jsonschema:"Bash command executed inside skill directory"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"Command timeout in seconds, default 120, max 600"`
}

type runSkillBashOutput struct {
	SkillDir          string `json:"skill_dir" jsonschema:"Absolute skill working directory"`
	ExitCode          int    `json:"exit_code" jsonschema:"Process exit code"`
	Stdout            string `json:"stdout" jsonschema:"Captured stdout"`
	Stderr            string `json:"stderr" jsonschema:"Captured stderr"`
	DurationMs        int64  `json:"duration_ms" jsonschema:"Execution duration in milliseconds"`
	FrontendUploadDir string `json:"frontend_upload_dir" jsonschema:"Absolute frontend upload directory path"`
	FrontendTmpDir    string `json:"frontend_tmp_dir" jsonschema:"Deprecated: use frontend_upload_dir"`
}

func runSkillBash(_ context.Context, _ *mcp.CallToolRequest, in runSkillBashInput) (*mcp.CallToolResult, runSkillBashOutput, error) {
	out, err := runSkillBashLocal(in)
	if err != nil {
		return nil, runSkillBashOutput{}, err
	}
	return nil, out, nil
}

func runSkillBashLocal(in runSkillBashInput) (runSkillBashOutput, error) {
	skillName := strings.TrimSpace(in.SkillName)
	if skillName == "" {
		return runSkillBashOutput{}, fmt.Errorf("skill_name is required")
	}
	if strings.Contains(skillName, "/") || strings.Contains(skillName, "\\") || strings.Contains(skillName, "..") {
		return runSkillBashOutput{}, fmt.Errorf("invalid skill_name")
	}
	command := strings.TrimSpace(in.Command)
	if command == "" {
		return runSkillBashOutput{}, fmt.Errorf("command is required")
	}

	timeoutSeconds := in.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}
	if timeoutSeconds > 600 {
		timeoutSeconds = 600
	}

	skillsRoot, err := resolveSkillsRootDir()
	if err != nil {
		return runSkillBashOutput{}, fmt.Errorf("resolve skills root dir: %w", err)
	}
	skillDir := filepath.Join(skillsRoot, skillName)
	info, err := os.Stat(skillDir)
	if err != nil || !info.IsDir() {
		return runSkillBashOutput{}, fmt.Errorf("skill not found: %s", skillName)
	}
	frontendTmpDir, err := resolveFrontendTmpDir()
	if err != nil {
		return runSkillBashOutput{}, fmt.Errorf("resolve frontend tmp dir: %w", err)
	}
	frontendUploadDir, err := resolveFrontendUploadDir()
	if err != nil {
		return runSkillBashOutput{}, fmt.Errorf("resolve frontend upload dir: %w", err)
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = skillDir
	cmd.Env = append(os.Environ(),
		"SKILL_DIR="+skillDir,
		"FRONTEND_TMP_DIR="+frontendTmpDir,
		"FRONTEND_UPLOAD_DIR="+frontendUploadDir,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		var ee *exec.ExitError
		if ok := errors.As(runErr, &ee); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
		if ctx.Err() == context.DeadlineExceeded {
			stderr.WriteString("\ncommand timed out")
		}
	}

	out := runSkillBashOutput{
		SkillDir:          skillDir,
		ExitCode:          exitCode,
		Stdout:            stdout.String(),
		Stderr:            stderr.String(),
		DurationMs:        time.Since(start).Milliseconds(),
		FrontendTmpDir:    frontendTmpDir,
		FrontendUploadDir: frontendUploadDir,
	}
	if runErr != nil {
		return out, fmt.Errorf("command failed with exit_code=%d", exitCode)
	}
	return out, nil
}
