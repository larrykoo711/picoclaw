package agent

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestEstimateStringTokens_ASCII(t *testing.T) {
	// 20 ASCII chars → ~5 tokens (20/4)
	s := "Hello, this is test."
	got := estimateStringTokens(s)
	if got < 4 || got > 6 {
		t.Errorf("estimateStringTokens(%q) = %d, want ~5", s, got)
	}
}

func TestEstimateStringTokens_CJK(t *testing.T) {
	// 4 CJK chars → ~6 tokens (4*3/2)
	s := "你好世界"
	got := estimateStringTokens(s)
	if got != 6 {
		t.Errorf("estimateStringTokens(%q) = %d, want 6", s, got)
	}
}

func TestEstimateStringTokens_Mixed(t *testing.T) {
	// "Hello你好" → 5 ASCII (5/4=1) + 2 CJK (2*3/2=3) = 4
	s := "Hello你好"
	got := estimateStringTokens(s)
	if got < 3 || got > 5 {
		t.Errorf("estimateStringTokens(%q) = %d, want ~4", s, got)
	}
}

func TestEstimateStringTokens_Empty(t *testing.T) {
	if got := estimateStringTokens(""); got != 0 {
		t.Errorf("estimateStringTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	messages := []providers.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "你好，请帮我翻译这段话。"},
	}

	got := estimateMessagesTokens(messages)
	// system: 28 ASCII chars → 7 tokens + 4 overhead = 11
	// user: 11 CJK chars → ~16 tokens + 1 punctuation → ~16-17 + 4 overhead = ~20
	// Total: ~31
	if got < 20 || got > 40 {
		t.Errorf("estimateMessagesTokens() = %d, want ~30", got)
	}
}

func TestEstimateMessagesTokens_WithToolCalls(t *testing.T) {
	messages := []providers.Message{
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []providers.ToolCall{
				{
					ID:   "call_1",
					Name: "get_weather",
					Arguments: map[string]any{
						"city": "北京",
					},
				},
			},
		},
	}

	got := estimateMessagesTokens(messages)
	// Tool name: 11 chars → 2 tokens
	// Arg "北京": 2 CJK → 3 tokens
	// Overhead: 4
	// Total: ~9
	if got < 5 || got > 15 {
		t.Errorf("estimateMessagesTokens() with tool calls = %d, want ~9", got)
	}
}

func TestIsCJK(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'中', true},
		{'A', false},
		{'한', true},   // Hangul
		{'あ', true},   // Hiragana
		{'ア', true},   // Katakana
		{'1', false},
		{' ', false},
	}

	for _, tt := range tests {
		if got := isCJK(tt.r); got != tt.want {
			t.Errorf("isCJK(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}
