package mcpserver

import "testing"

func TestClampTimeout(t *testing.T) {
	if got := clampTimeout(0, 120, 600); got != 120 {
		t.Fatalf("expected default timeout 120, got %d", got)
	}
	if got := clampTimeout(700, 120, 600); got != 600 {
		t.Fatalf("expected clamped timeout 600, got %d", got)
	}
	if got := clampTimeout(30, 120, 600); got != 30 {
		t.Fatalf("expected explicit timeout 30, got %d", got)
	}
}

func TestSanitizeSessionID(t *testing.T) {
	got := sanitizeSessionID("  Session_01../@#")
	if got != "session-01" {
		t.Fatalf("unexpected sanitized session id: %s", got)
	}
}

func TestCodeLanguageMeta(t *testing.T) {
	meta, err := codeLanguageMeta("cpp")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if meta.FileName != "main.cpp" {
		t.Fatalf("unexpected filename: %s", meta.FileName)
	}
	if _, err := codeLanguageMeta("ruby"); err == nil {
		t.Fatalf("expected unsupported language error")
	}
}
