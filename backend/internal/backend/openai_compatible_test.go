package backend

import "testing"

func TestNormalizeDelta_Cumulative(t *testing.T) {
	prev := ""

	delta, next := normalizeDelta(prev, "Hello")
	if delta != "Hello" || next != "Hello" {
		t.Fatalf("unexpected first chunk: delta=%q next=%q", delta, next)
	}

	delta, next = normalizeDelta(next, "Hello, wor")
	if delta != ", wor" || next != "Hello, wor" {
		t.Fatalf("unexpected second chunk: delta=%q next=%q", delta, next)
	}

	delta, next = normalizeDelta(next, "Hello, world")
	if delta != "ld" || next != "Hello, world" {
		t.Fatalf("unexpected third chunk: delta=%q next=%q", delta, next)
	}
}

func TestNormalizeDelta_Incremental(t *testing.T) {
	prev := ""

	delta, next := normalizeDelta(prev, "Hello")
	if delta != "Hello" || next != "Hello" {
		t.Fatalf("unexpected first chunk: delta=%q next=%q", delta, next)
	}

	delta, next = normalizeDelta(next, ", ")
	if delta != ", " || next != "Hello, " {
		t.Fatalf("unexpected second chunk: delta=%q next=%q", delta, next)
	}

	delta, next = normalizeDelta(next, "world")
	if delta != "world" || next != "Hello, world" {
		t.Fatalf("unexpected third chunk: delta=%q next=%q", delta, next)
	}
}

func TestThinkTagSplitter_SplitAcrossChunks(t *testing.T) {
	s := &thinkTagSplitter{}

	r1, c1 := s.Feed("<thi")
	if r1 != "" || c1 != "" {
		t.Fatalf("expected no output for partial tag, got reasoning=%q content=%q", r1, c1)
	}

	r2, c2 := s.Feed("nk>思考一</thi")
	if r2 != "思考一" || c2 != "" {
		t.Fatalf("unexpected second output: reasoning=%q content=%q", r2, c2)
	}

	r3, c3 := s.Feed("nk>回答")
	if r3 != "" || c3 != "回答" {
		t.Fatalf("unexpected third output: reasoning=%q content=%q", r3, c3)
	}
}

func TestThinkTagSplitter_ContentOnly(t *testing.T) {
	s := &thinkTagSplitter{}
	r, c := s.Feed("纯正文输出")
	if r != "" || c != "纯正文输出" {
		t.Fatalf("unexpected output: reasoning=%q content=%q", r, c)
	}
}

func TestThinkTagSplitter_FullTagInOneChunk(t *testing.T) {
	s := &thinkTagSplitter{}
	r, c := s.Feed("<think>abc</think>xyz")
	if r != "abc" || c != "xyz" {
		t.Fatalf("unexpected output: reasoning=%q content=%q", r, c)
	}
}

func TestDecodeThinkTagEscapes(t *testing.T) {
	got := decodeThinkTagEscapes(`\\u003cthink\\u003eabc\\u003c/think\\u003exyz`)
	if got != "<think>abc</think>xyz" {
		t.Fatalf("unexpected decode result: %q", got)
	}
}
