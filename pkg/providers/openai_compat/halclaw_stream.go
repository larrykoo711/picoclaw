package openai_compat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

// ChatStream sends a streaming chat request and returns a channel of StreamEvents.
// It reuses Chat's request-building logic with "stream": true, then parses the
// SSE response into typed events.
func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan protocoltypes.StreamEvent, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	if !p.passthroughModel {
		model = normalizeModel(model, p.apiBase)
	}

	requestBody := map[string]any{
		"model":    model,
		"messages": serializeMessages(messages),
		"stream":   true,
	}

	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	if maxTokens, ok := asInt(options["max_tokens"]); ok {
		fieldName := p.maxTokensField
		if fieldName == "" {
			fieldName = "max_tokens"
		}
		requestBody[fieldName] = maxTokens
	}

	if temperature, ok := asFloat(options["temperature"]); ok {
		requestBody["temperature"] = temperature
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed:\n  Status: %d\n  Body:   %s", resp.StatusCode, string(body))
	}

	ch := make(chan protocoltypes.StreamEvent, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		sseCh := protocoltypes.ParseSSEStream(resp.Body)
		for payload := range sseCh {
			events := parseStreamChunk(payload)
			for _, ev := range events {
				select {
				case ch <- ev:
				case <-ctx.Done():
					return
				}
			}
		}

		// Send done event
		select {
		case ch <- protocoltypes.StreamEvent{Type: "done"}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// streamChunk is the wire format for a single SSE chunk from OpenAI-compatible APIs.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function *struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *protocoltypes.UsageInfo `json:"usage"`
}

// parseStreamChunk parses a single SSE JSON payload into StreamEvents.
func parseStreamChunk(payload string) []protocoltypes.StreamEvent {
	var chunk streamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return []protocoltypes.StreamEvent{{
			Type:  "error",
			Error: fmt.Errorf("failed to parse stream chunk: %w", err),
		}}
	}

	var events []protocoltypes.StreamEvent

	for _, choice := range chunk.Choices {
		// Content delta
		if choice.Delta.Content != "" {
			events = append(events, protocoltypes.StreamEvent{
				Type:    "content",
				Content: choice.Delta.Content,
			})
		}

		// Reasoning delta
		reasoning := choice.Delta.ReasoningContent
		if reasoning == "" {
			reasoning = choice.Delta.Reasoning
		}
		if reasoning != "" {
			events = append(events, protocoltypes.StreamEvent{
				Type:             "reasoning",
				ReasoningContent: reasoning,
			})
		}

		// Tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			name := ""
			args := ""
			id := tc.ID
			if tc.Function != nil {
				name = tc.Function.Name
				args = tc.Function.Arguments
			}
			events = append(events, protocoltypes.StreamEvent{
				Type: "tool_call",
				ToolCallDelta: &protocoltypes.ToolCallDelta{
					Index:     tc.Index,
					ID:        id,
					Name:      name,
					Arguments: args,
				},
			})
		}

		// Finish reason
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			events = append(events, protocoltypes.StreamEvent{
				Type:         "done",
				FinishReason: *choice.FinishReason,
				Usage:        chunk.Usage,
			})
		}
	}

	return events
}
