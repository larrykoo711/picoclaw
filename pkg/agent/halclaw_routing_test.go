package agent

import (
	"os"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/routing"
)

// TestLightThinkingLevel_FromConfig verifies that a light model without
// thinking_level in model_list resolves to ThinkingOff (same logic as primary).
func TestLightThinkingLevel_FromConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "routing-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{ModelName: "primary", Model: "openai/claude-sonnet", ThinkingLevel: "medium"},
			{ModelName: "glm-5", Model: "openai/glm-5"}, // no thinking_level
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "primary",
				MaxTokens:         4096,
				MaxToolIterations: 10,
				Routing: &config.RoutingConfig{
					Enabled:    true,
					LightModel: "glm-5",
					Threshold:  0.35,
				},
			},
		},
	}

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})

	if agent.LightThinkingLevel != ThinkingOff {
		t.Errorf("LightThinkingLevel = %q, want %q (model has no thinking_level config)",
			agent.LightThinkingLevel, ThinkingOff)
	}
	// Primary should still have its own thinking level.
	if agent.ThinkingLevel != ThinkingMedium {
		t.Errorf("ThinkingLevel = %q, want %q", agent.ThinkingLevel, ThinkingMedium)
	}
}

// TestLightThinkingLevel_ExplicitConfig verifies that a light model with
// explicit thinking_level in model_list is correctly parsed.
func TestLightThinkingLevel_ExplicitConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "routing-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{ModelName: "primary", Model: "openai/claude-sonnet", ThinkingLevel: "medium"},
			{ModelName: "light", Model: "openai/light-model", ThinkingLevel: "low"},
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "primary",
				MaxTokens:         4096,
				MaxToolIterations: 10,
				Routing: &config.RoutingConfig{
					Enabled:    true,
					LightModel: "light",
					Threshold:  0.35,
				},
			},
		},
	}

	agent := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})

	if agent.LightThinkingLevel != ThinkingLow {
		t.Errorf("LightThinkingLevel = %q, want %q", agent.LightThinkingLevel, ThinkingLow)
	}
}

// fixedClassifier always returns a predetermined score for testing.
type fixedClassifier struct{ score float64 }

func (f *fixedClassifier) Score(_ routing.Features) float64 { return f.score }

// TestSelectCandidates_RespectsModelConfig verifies that when routing selects
// the light model, the returned routedModel carries the light model's own
// ThinkingLevel (from model_list config, not the primary's).
func TestSelectCandidates_RespectsModelConfig(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	agent := &AgentInstance{
		ID:            "test",
		Model:         "primary-model",
		ThinkingLevel: ThinkingMedium,
		Candidates: []providers.FallbackCandidate{
			{Provider: "openai", Model: "primary-model"},
		},
		LightCandidates: []providers.FallbackCandidate{
			{Provider: "openai", Model: "glm-5"},
		},
		LightThinkingLevel: ThinkingOff, // light model has no thinking
		// Use a Router with a fixed classifier that always returns low score
		// (below threshold → selects light model).
		Router: routing.NewWithClassifier(routing.RouterConfig{
			LightModel: "glm-5",
			Threshold:  0.35,
		}, &fixedClassifier{score: 0.1}),
	}

	routed := al.selectCandidates(agent, "hi", nil)

	if routed.Model != "glm-5" {
		t.Errorf("Model = %q, want %q", routed.Model, "glm-5")
	}
	if routed.ThinkingLevel != ThinkingOff {
		t.Errorf("ThinkingLevel = %q, want %q (light model should use its own config)",
			routed.ThinkingLevel, ThinkingOff)
	}
	if len(routed.Candidates) != 1 || routed.Candidates[0].Model != "glm-5" {
		t.Errorf("Candidates = %v, want light candidates", routed.Candidates)
	}
}

// TestSelectCandidates_PrimaryKeepsOwnConfig verifies that when routing selects
// the primary model, the returned routedModel carries the primary's ThinkingLevel.
func TestSelectCandidates_PrimaryKeepsOwnConfig(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	agent := &AgentInstance{
		ID:            "test",
		Model:         "primary-model",
		ThinkingLevel: ThinkingMedium,
		Candidates: []providers.FallbackCandidate{
			{Provider: "openai", Model: "primary-model"},
		},
		LightCandidates: []providers.FallbackCandidate{
			{Provider: "openai", Model: "glm-5"},
		},
		LightThinkingLevel: ThinkingOff,
		// Fixed classifier returns high score (above threshold → primary).
		Router: routing.NewWithClassifier(routing.RouterConfig{
			LightModel: "glm-5",
			Threshold:  0.35,
		}, &fixedClassifier{score: 0.9}),
	}

	routed := al.selectCandidates(agent, "explain quantum computing in detail with code examples", nil)

	if routed.Model != "primary-model" {
		t.Errorf("Model = %q, want %q", routed.Model, "primary-model")
	}
	if routed.ThinkingLevel != ThinkingMedium {
		t.Errorf("ThinkingLevel = %q, want %q (primary should keep its own config)",
			routed.ThinkingLevel, ThinkingMedium)
	}
	if len(routed.Candidates) != 1 || routed.Candidates[0].Model != "primary-model" {
		t.Errorf("Candidates = %v, want primary candidates", routed.Candidates)
	}
}
