package mcpserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type webFetchInput struct {
	URL             string `json:"url" jsonschema:"Target URL to fetch"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty" jsonschema:"Request timeout in seconds, default 30, max 120"`
	MaxLength       int    `json:"max_length,omitempty" jsonschema:"Maximum content length in characters, default 100000"`
}

type webFetchOutput struct {
	URL            string `json:"url" jsonschema:"Requested URL"`
	StatusCode     int    `json:"status_code" jsonschema:"HTTP status code"`
	ContentType    string `json:"content_type" jsonschema:"Response Content-Type header"`
	Content        string `json:"content" jsonschema:"Response body text (truncated if exceeds max_length)"`
	ContentLength  int    `json:"content_length" jsonschema:"Actual response body length in bytes"`
	Truncated      bool   `json:"truncated" jsonschema:"Whether content was truncated due to max_length"`
	Error          string `json:"error,omitempty" jsonschema:"Error message if request failed"`
}

func webFetch(_ context.Context, _ *mcp.CallToolRequest, in webFetchInput) (*mcp.CallToolResult, webFetchOutput, error) {
	out := webFetchLocal(in)
	return nil, out, nil
}

func webFetchLocal(in webFetchInput) webFetchOutput {
	targetURL := strings.TrimSpace(in.URL)
	if targetURL == "" {
		return webFetchOutput{Error: "url is required"}
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return webFetchOutput{URL: targetURL, Error: "invalid url: must be a valid http or https URL"}
	}

	timeoutSeconds := in.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	if timeoutSeconds > 120 {
		timeoutSeconds = 120
	}

	maxLength := in.MaxLength
	if maxLength <= 0 {
		maxLength = 100000
	}
	if maxLength > 500000 {
		maxLength = 500000
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return webFetchOutput{URL: targetURL, Error: fmt.Sprintf("create request: %v", err)}
	}
	req.Header.Set("User-Agent", "agent-web-fetch/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return webFetchOutput{URL: targetURL, Error: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	// 只处理文本内容; 二进制内容给出提示.
	if !isTextContentType(contentType) {
		return webFetchOutput{
			URL:         targetURL,
			StatusCode:  resp.StatusCode,
			ContentType: contentType,
			Error:       fmt.Sprintf("content-type is not text: %s", contentType),
		}
	}

	// 限制读取大小, 防止超大响应导致内存问题.
	const maxReadBytes = 5 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxReadBytes)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return webFetchOutput{URL: targetURL, StatusCode: resp.StatusCode, ContentType: contentType, Error: fmt.Sprintf("read body: %v", err)}
	}

	bodyText := string(bodyBytes)
	truncated := false
	if len(bodyText) > maxLength {
		bodyText = bodyText[:maxLength]
		truncated = true
	}

	return webFetchOutput{
		URL:           targetURL,
		StatusCode:    resp.StatusCode,
		ContentType:   contentType,
		Content:       bodyText,
		ContentLength: len(bodyBytes),
		Truncated:     truncated,
	}
}

func isTextContentType(ct string) bool {
	ct = strings.ToLower(ct)
	// 处理可能带 charset 的 Content-Type, 如 text/html; charset=utf-8
	beforeSemi, _, _ := strings.Cut(ct, ";")
	beforeSemi = strings.TrimSpace(beforeSemi)
	switch beforeSemi {
	case "text/html", "text/plain", "text/css", "text/javascript", "application/javascript",
		"application/json", "application/xml", "text/xml", "application/rss+xml",
		"application/atom+xml", "text/markdown", "application/x-yaml", "text/yaml":
		return true
	}
	// 以 text/ 开头的都视为文本
	if strings.HasPrefix(beforeSemi, "text/") {
		return true
	}
	// 空 Content-Type 也尝试当作文本处理
	if beforeSemi == "" {
		return true
	}
	return false
}
