package anthropicprovider

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/sipeed/picoclaw/pkg/providers/protocoltypes"
)

// ChatStream sends a streaming chat request using the Anthropic SDK and returns
// a channel of StreamEvents. It converts SDK streaming events into the unified
// StreamEvent format used by the agent layer.
func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan protocoltypes.StreamEvent, error) {
	var opts []option.RequestOption
	if p.tokenSource != nil {
		tok, err := p.tokenSource()
		if err != nil {
			return nil, fmt.Errorf("refreshing token: %w", err)
		}
		opts = append(opts, option.WithAuthToken(tok))
	}

	params, err := buildParams(messages, tools, model, options)
	if err != nil {
		return nil, err
	}

	stream := p.client.Messages.NewStreaming(ctx, params, opts...)

	ch := make(chan protocoltypes.StreamEvent, 32)

	go func() {
		defer close(ch)
		defer stream.Close()

		// Track cumulative usage from message_start and message_delta
		var inputTokens, outputTokens int64

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "message_start":
				// Capture initial usage from message_start
				if event.Message.Usage.InputTokens > 0 {
					inputTokens = event.Message.Usage.InputTokens
				}

			case "content_block_start":
				block := event.ContentBlock
				switch block.Type {
				case "tool_use":
					emit(ch, ctx, protocoltypes.StreamEvent{
						Type: "tool_call",
						ToolCallDelta: &protocoltypes.ToolCallDelta{
							Index: int(event.Index),
							ID:    block.ID,
							Name:  block.Name,
						},
					})
				}

			case "content_block_delta":
				delta := event.Delta
				switch delta.Type {
				case "text_delta":
					if delta.Text != "" {
						emit(ch, ctx, protocoltypes.StreamEvent{
							Type:    "content",
							Content: delta.Text,
						})
					}
				case "thinking_delta":
					if delta.Thinking != "" {
						emit(ch, ctx, protocoltypes.StreamEvent{
							Type:             "reasoning",
							ReasoningContent: delta.Thinking,
						})
					}
				case "input_json_delta":
					if delta.PartialJSON != "" {
						emit(ch, ctx, protocoltypes.StreamEvent{
							Type: "tool_call",
							ToolCallDelta: &protocoltypes.ToolCallDelta{
								Index:     int(event.Index),
								Arguments: delta.PartialJSON,
							},
						})
					}
				}

			case "message_delta":
				// Accumulate final usage
				if event.Usage.OutputTokens > 0 {
					outputTokens = event.Usage.OutputTokens
				}
				finishReason := mapStopReason(event.Delta.StopReason)
				emit(ch, ctx, protocoltypes.StreamEvent{
					Type:         "done",
					FinishReason: finishReason,
					Usage: &protocoltypes.UsageInfo{
						PromptTokens:     int(inputTokens),
						CompletionTokens: int(outputTokens),
						TotalTokens:      int(inputTokens + outputTokens),
					},
				})
			}
		}

		if err := stream.Err(); err != nil {
			emit(ch, ctx, protocoltypes.StreamEvent{Type: "error", Error: err})
		}
	}()

	return ch, nil
}

func mapStopReason(reason anthropic.StopReason) string {
	switch reason {
	case anthropic.StopReasonToolUse:
		return "tool_calls"
	case anthropic.StopReasonMaxTokens:
		return "length"
	case anthropic.StopReasonEndTurn:
		return "stop"
	default:
		return string(reason)
	}
}

// emit sends an event on ch, respecting context cancellation.
func emit(ch chan<- protocoltypes.StreamEvent, ctx context.Context, ev protocoltypes.StreamEvent) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}
