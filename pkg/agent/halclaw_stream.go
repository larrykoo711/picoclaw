package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// StreamCallback is called for each streaming event during an LLM call.
// The desktop client subscribes to this via the message bus.
type StreamCallback func(event providers.StreamEvent)

// streamLLMCall attempts to use streaming if the provider supports it,
// otherwise falls back to the non-streaming Chat method.
// It accumulates all streaming events into a complete LLMResponse.
func streamLLMCall(
	ctx context.Context,
	provider providers.LLMProvider,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
	callback StreamCallback,
) (*providers.LLMResponse, error) {
	// Try streaming if provider supports it
	sp, ok := provider.(providers.StreamingProvider)
	if !ok {
		// Fallback to non-streaming
		return provider.Chat(ctx, messages, tools, model, options)
	}

	ch, err := sp.ChatStream(ctx, messages, tools, model, options)
	if err != nil {
		return nil, err
	}

	return accumulateStream(ctx, ch, callback)
}

// accumulateStream reads all events from a streaming channel and builds
// a complete LLMResponse. Each event is also forwarded to the callback.
func accumulateStream(
	ctx context.Context,
	ch <-chan providers.StreamEvent,
	callback StreamCallback,
) (*providers.LLMResponse, error) {
	var content strings.Builder
	var reasoning strings.Builder
	var toolCalls []providers.ToolCall
	var finishReason string
	var usage *providers.UsageInfo
	var lastErr error

	// Track tool call accumulation by index
	toolCallMap := make(map[int]*toolCallAccumulator)

	for event := range ch {
		// Forward to callback
		if callback != nil {
			callback(event)
		}

		switch event.Type {
		case "content":
			content.WriteString(event.Content)

		case "reasoning":
			reasoning.WriteString(event.ReasoningContent)

		case "tool_call":
			if event.ToolCallDelta != nil {
				tc := event.ToolCallDelta
				acc, exists := toolCallMap[tc.Index]
				if !exists {
					acc = &toolCallAccumulator{Index: tc.Index}
					toolCallMap[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Name != "" {
					acc.Name = tc.Name
				}
				if tc.Arguments != "" {
					acc.Arguments.WriteString(tc.Arguments)
				}
			}

		case "done":
			if event.FinishReason != "" {
				finishReason = event.FinishReason
			}
			if event.Usage != nil {
				usage = event.Usage
			}

		case "error":
			if event.Error != nil {
				lastErr = event.Error
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("stream error: %w", lastErr)
	}

	// Convert accumulated tool calls
	for i := 0; i < len(toolCallMap); i++ {
		acc, ok := toolCallMap[i]
		if !ok {
			continue
		}
		args := make(map[string]any)
		argsStr := acc.Arguments.String()
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args["raw"] = argsStr
			}
		}
		toolCalls = append(toolCalls, providers.ToolCall{
			ID:        acc.ID,
			Name:      acc.Name,
			Arguments: args,
		})
	}

	if finishReason == "" {
		finishReason = "stop"
	}

	return &providers.LLMResponse{
		Content:          content.String(),
		ReasoningContent: reasoning.String(),
		ToolCalls:        toolCalls,
		FinishReason:     finishReason,
		Usage:            usage,
	}, nil
}

// toolCallAccumulator collects streaming fragments of a single tool call.
type toolCallAccumulator struct {
	Index     int
	ID        string
	Name      string
	Arguments strings.Builder
}

// streamCallback returns a StreamCallback for the current agent loop iteration.
// Currently a no-op placeholder — the desktop client will hook into this via
// the message bus in a future stage.
func (al *AgentLoop) streamCallback(ctx context.Context, opts processOptions) StreamCallback {
	return func(event providers.StreamEvent) {
		// Future: publish streaming events to bus for desktop UI consumption
		// e.g. al.bus.PublishStreamEvent(ctx, opts.SessionKey, event)
	}
}
