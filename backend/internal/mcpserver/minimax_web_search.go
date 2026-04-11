package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type minimaxWebSearchInput struct {
	Query string `json:"query" jsonschema:"Search query, 3-5 keywords for best results"`
}

func minimaxWebSearch(_ context.Context, _ *mcp.CallToolRequest, in minimaxWebSearchInput) (*mcp.CallToolResult, minimaxWebSearchInput, error) {
	result, err := minimaxWebSearchLocal(in.Query)
	if err != nil {
		return nil, minimaxWebSearchInput{}, err
	}
	// 将结果序列化为 JSON 返回
	b, _ := json.Marshal(result)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, in, nil
}

func minimaxWebSearchLocal(query string) (map[string]any, error) {
	if minimaxKeyMgr == nil {
		return nil, fmt.Errorf("minimax tools not initialized: no API keys configured")
	}
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	result, err := doMiniMaxWebSearch(query)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func doMiniMaxWebSearch(query string) (map[string]any, error) {
	key := minimaxKeyMgr.NextKey()
	url := minimaxAPIHost + "/v1/coding_plan/search"

	body := map[string]any{"q": query}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal search body: %w", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("MM-API-Source", "Minimax-MCP")

	log.Printf("[minimax-search] POST %s q=%q", url, query)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}

	// 401/403 时重试一次
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		minimaxKeyMgr.ReportError(key)
		log.Printf("[minimax-search] key auth failed (status %d), retrying with next key", resp.StatusCode)

		key = minimaxKeyMgr.NextKey()
		req2, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyJSON))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+key)
		req2.Header.Set("MM-API-Source", "Minimax-MCP")

		resp2, err := httpClient.Do(req2)
		if err != nil {
			return nil, fmt.Errorf("search retry failed: %w", err)
		}
		defer resp2.Body.Close()
		respBody, err = io.ReadAll(resp2.Body)
		if err != nil {
			return nil, fmt.Errorf("read search retry response: %w", err)
		}
		resp = resp2
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	// 检查 API 级错误码
	baseResp, _ := result["base_resp"].(map[string]any)
	if statusCode, _ := baseResp["status_code"].(float64); statusCode != 0 {
		statusMsg, _ := baseResp["status_msg"].(string)
		return nil, fmt.Errorf("search API error: status_code=%.0f, status_msg=%s", statusCode, statusMsg)
	}

	return result, nil
}
