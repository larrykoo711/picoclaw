package agent

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// mockStreamingProvider implements both LLMProvider and HalclawStreamingProvider.
type mockStreamingProvider struct {
	events []providers.StreamEvent
}

func (m *mockStreamingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "non-streaming fallback", FinishReason: "stop"}, nil
}

func (m *mockStreamingProvider) GetDefaultModel() string { return "mock" }

func (m *mockStreamingProvider) ChatStream(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (<-chan providers.StreamEvent, error) {
	ch := make(chan providers.StreamEvent, len(m.events))
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// mockNonStreamingProvider only implements LLMProvider.
type mockNonStreamingProvider struct{}

func (m *mockNonStreamingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "non-streaming response", FinishReason: "stop"}, nil
}

func (m *mockNonStreamingProvider) GetDefaultModel() string { return "mock" }

func TestStreamLLMCall_WithStreaming(t *testing.T) {
	provider := &mockStreamingProvider{
		events: []providers.StreamEvent{
			{Type: "content", Content: "Hello"},
			{Type: "content", Content: " world"},
			{Type: "done", FinishReason: "stop", Usage: &providers.UsageInfo{
				PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
			}},
		},
	}

	var callbackEvents int
	callback := func(event providers.StreamEvent) {
		callbackEvents++
	}

	resp, err := streamLLMCall(context.Background(), provider, nil, nil, "model", nil, callback)
	if err != nil {
		t.Fatalf("streamLLMCall() error = %v", err)
	}

	if resp.Content != "Hello world" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello world")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Errorf("Usage.TotalTokens = %v, want 15", resp.Usage)
	}
	if callbackEvents != 3 {
		t.Errorf("callbackEvents = %d, want 3", callbackEvents)
	}
}

func TestStreamLLMCall_FallbackToChat(t *testing.T) {
	provider := &mockNonStreamingProvider{}

	resp, err := streamLLMCall(context.Background(), provider, nil, nil, "model", nil, nil)
	if err != nil {
		t.Fatalf("streamLLMCall() error = %v", err)
	}

	if resp.Content != "non-streaming response" {
		t.Errorf("Content = %q, want %q", resp.Content, "non-streaming response")
	}
}

func TestStreamLLMCall_ToolCalls(t *testing.T) {
	provider := &mockStreamingProvider{
		events: []providers.StreamEvent{
			{Type: "tool_call", ToolCallDelta: &providers.ToolCallDelta{
				Index: 0, ID: "call_1", Name: "get_weather",
			}},
			{Type: "tool_call", ToolCallDelta: &providers.ToolCallDelta{
				Index: 0, Arguments: `{"city":`,
			}},
			{Type: "tool_call", ToolCallDelta: &providers.ToolCallDelta{
				Index: 0, Arguments: `"SF"}`,
			}},
			{Type: "done", FinishReason: "tool_calls"},
		},
	}

	resp, err := streamLLMCall(context.Background(), provider, nil, nil, "model", nil, nil)
	if err != nil {
		t.Fatalf("streamLLMCall() error = %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("ToolCalls[0].ID = %q, want %q", tc.ID, "call_1")
	}
	if tc.Name != "get_weather" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", tc.Name, "get_weather")
	}
	if tc.Arguments["city"] != "SF" {
		t.Errorf("ToolCalls[0].Arguments[city] = %v, want SF", tc.Arguments["city"])
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_calls")
	}
}

func TestStreamLLMCall_Reasoning(t *testing.T) {
	provider := &mockStreamingProvider{
		events: []providers.StreamEvent{
			{Type: "reasoning", ReasoningContent: "Let me think..."},
			{Type: "content", Content: "42"},
			{Type: "done", FinishReason: "stop"},
		},
	}

	resp, err := streamLLMCall(context.Background(), provider, nil, nil, "model", nil, nil)
	if err != nil {
		t.Fatalf("streamLLMCall() error = %v", err)
	}

	if resp.ReasoningContent != "Let me think..." {
		t.Errorf("ReasoningContent = %q, want %q", resp.ReasoningContent, "Let me think...")
	}
	if resp.Content != "42" {
		t.Errorf("Content = %q, want %q", resp.Content, "42")
	}
}

func TestStreamLLMCall_Error(t *testing.T) {
	provider := &mockStreamingProvider{
		events: []providers.StreamEvent{
			{Type: "content", Content: "partial"},
			{Type: "error", Error: context.DeadlineExceeded},
		},
	}

	_, err := streamLLMCall(context.Background(), provider, nil, nil, "model", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
