package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveFrontendTmpDir 已弃用, 请使用 resolveFrontendUploadDir.
// 保留用于向后兼容 (测试).
func resolveFrontendTmpDir() (string, error) {
	frontendDir, err := resolveFrontendDir([]string{
		filepath.Join("..", "frontend"),
		filepath.Join("..", "..", "frontend"),
		"frontend",
	})
	if err != nil {
		return "", err
	}
	tmp := filepath.Join(frontendDir, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", fmt.Errorf("create frontend tmp dir: %w", err)
	}
	return tmp, nil
}

func resolveFrontendDir(candidates []string) (string, error) {
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "package.json")); err != nil {
			continue
		}
		return abs, nil
	}
	return "", fmt.Errorf("cannot resolve frontend directory from candidates: %v", candidates)
}

func resolveSkillsRootDir() (string, error) {
	return resolveExistingDir([]string{
		"skills",
		filepath.Join("..", "skills"),
	})
}

func resolveExistingDir(candidates []string) (string, error) {
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("cannot resolve directory from candidates: %v", candidates)
}
