package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type convertLocalPathToURLInput struct {
	LocalPath string `json:"local_path" jsonschema:"Absolute local file path to convert into a frontend URL."`
}

type convertLocalPathToURLOutput struct {
	LocalPath string `json:"local_path" jsonschema:"Normalized absolute local file path."`
	URL       string `json:"url" jsonschema:"Frontend URL mapped from local_path."`
}

func convertLocalPathToURL(_ context.Context, _ *mcp.CallToolRequest, in convertLocalPathToURLInput) (*mcp.CallToolResult, convertLocalPathToURLOutput, error) {
	out, err := convertLocalPathToURLLocal(in)
	if err != nil {
		return nil, convertLocalPathToURLOutput{}, err
	}
	return nil, out, nil
}

func convertLocalPathToURLLocal(in convertLocalPathToURLInput) (convertLocalPathToURLOutput, error) {
	localPath := strings.TrimSpace(in.LocalPath)
	if localPath == "" {
		return convertLocalPathToURLOutput{}, fmt.Errorf("local_path is required")
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return convertLocalPathToURLOutput{}, fmt.Errorf("abs local_path: %w", err)
	}

	frontendDir, err := resolveFrontendDir([]string{
		filepath.Join("..", "frontend"),
		filepath.Join("..", "..", "frontend"),
		"frontend",
	})
	if err != nil {
		return convertLocalPathToURLOutput{}, err
	}

	relPath, err := filepath.Rel(frontendDir, absPath)
	if err != nil {
		return convertLocalPathToURLOutput{}, fmt.Errorf("rel path: %w", err)
	}
	relPath = filepath.Clean(relPath)
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return convertLocalPathToURLOutput{}, fmt.Errorf("local_path is outside frontend directory")
	}

	urlPath := filepath.ToSlash(relPath)
	if frontendURL == "" {
		return convertLocalPathToURLOutput{}, fmt.Errorf("frontend url is not initialized")
	}

	return convertLocalPathToURLOutput{
		LocalPath: absPath,
		URL:       frontendURL + "/" + strings.TrimPrefix(urlPath, "/"),
	}, nil
}
