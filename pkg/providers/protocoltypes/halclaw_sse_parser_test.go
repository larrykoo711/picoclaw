package protocoltypes

import (
	"strings"
	"testing"
)

func TestParseSSEStream_Standard(t *testing.T) {
	input := "data: hello\ndata: world\ndata: [DONE]\n"
	ch := ParseSSEStream(strings.NewReader(input))

	var results []string
	for s := range ch {
		results = append(results, s)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0] != "hello" {
		t.Errorf("results[0] = %q, want %q", results[0], "hello")
	}
	if results[1] != "world" {
		t.Errorf("results[1] = %q, want %q", results[1], "world")
	}
}

func TestParseSSEStream_EmptyLinesAndComments(t *testing.T) {
	input := ": keepalive\n\ndata: first\n\n: another comment\ndata: second\ndata: [DONE]\n"
	ch := ParseSSEStream(strings.NewReader(input))

	var results []string
	for s := range ch {
		results = append(results, s)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0] != "first" || results[1] != "second" {
		t.Errorf("results = %v, want [first, second]", results)
	}
}

func TestParseSSEStream_NoSpace(t *testing.T) {
	// Some servers emit "data:payload" without a space.
	input := "data:{\"content\":\"hi\"}\ndata:[DONE]\n"
	ch := ParseSSEStream(strings.NewReader(input))

	var results []string
	for s := range ch {
		results = append(results, s)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0] != `{"content":"hi"}` {
		t.Errorf("results[0] = %q, want %q", results[0], `{"content":"hi"}`)
	}
}

func TestParseSSEStream_EOF(t *testing.T) {
	// Stream ends with EOF instead of [DONE].
	input := "data: one\ndata: two\n"
	ch := ParseSSEStream(strings.NewReader(input))

	var results []string
	for s := range ch {
		results = append(results, s)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestParseSSEStream_EmptyData(t *testing.T) {
	// "data: " with only whitespace should be skipped.
	input := "data:  \ndata: real\ndata: [DONE]\n"
	ch := ParseSSEStream(strings.NewReader(input))

	var results []string
	for s := range ch {
		results = append(results, s)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0] != "real" {
		t.Errorf("results[0] = %q, want %q", results[0], "real")
	}
}

func TestParseSSEStream_NonDataLines(t *testing.T) {
	// Lines like "event:" or "id:" should be ignored.
	input := "event: message\nid: 1\ndata: payload\ndata: [DONE]\n"
	ch := ParseSSEStream(strings.NewReader(input))

	var results []string
	for s := range ch {
		results = append(results, s)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0] != "payload" {
		t.Errorf("results[0] = %q, want %q", results[0], "payload")
	}
}
