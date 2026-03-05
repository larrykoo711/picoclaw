package openai_compat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatStream_ContentDelta(t *testing.T) {
	// Mock SSE server that sends content deltas
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	var contents []string
	var gotDone bool
	for ev := range ch {
		switch ev.Type {
		case "content":
			contents = append(contents, ev.Content)
		case "done":
			gotDone = true
		}
	}

	if len(contents) != 2 || contents[0] != "Hello" || contents[1] != " world" {
		t.Errorf("contents = %v, want [Hello,  world]", contents)
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestChatStream_ToolCallDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"SF\"}"}}]},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "weather?"}}, nil, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	var toolCallEvents int
	var gotID, gotName bool
	for ev := range ch {
		if ev.Type == "tool_call" && ev.ToolCallDelta != nil {
			toolCallEvents++
			if ev.ToolCallDelta.ID == "call_1" {
				gotID = true
			}
			if ev.ToolCallDelta.Name == "get_weather" {
				gotName = true
			}
		}
	}

	if toolCallEvents < 3 {
		t.Errorf("got %d tool_call events, want >= 3", toolCallEvents)
	}
	if !gotID {
		t.Error("expected tool call with ID call_1")
	}
	if !gotName {
		t.Error("expected tool call with name get_weather")
	}
}

func TestChatStream_ReasoningDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"choices":[{"delta":{"reasoning_content":"Let me think"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":"The answer is 42"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "think"}}, nil, "model", nil)
	if err != nil {
		t.Fatalf("ChatStream() error = %v", err)
	}

	var gotReasoning, gotContent bool
	for ev := range ch {
		if ev.Type == "reasoning" && ev.ReasoningContent == "Let me think" {
			gotReasoning = true
		}
		if ev.Type == "content" && ev.Content == "The answer is 42" {
			gotContent = true
		}
	}

	if !gotReasoning {
		t.Error("expected reasoning event")
	}
	if !gotContent {
		t.Error("expected content event")
	}
}

func TestChatStream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "gpt-4o", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
