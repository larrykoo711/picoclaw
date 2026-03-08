package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeOpenClawMetadata_MultiLine(t *testing.T) {
	frontmatter := `name: spotify-player
description: Terminal Spotify playback/search via spogo (preferred) or spotify_player.
homepage: https://www.spotify.com
metadata:
  {
    "openclaw":
      {
        "emoji": "🎵",
        "requires": { "anyBins": ["spogo", "spotify_player"] },
        "install":
          [
            {
              "id": "brew",
              "kind": "brew",
              "formula": "spogo",
              "bins": ["spogo"],
              "label": "Install spogo (brew)",
            },
          ],
      },
  }`

	sl := &SkillsLoader{}
	meta := &SkillMetadata{}
	sl.mergeOpenClawMetadata(frontmatter, meta)

	if meta.Emoji != "🎵" {
		t.Errorf("emoji: got %q, want %q", meta.Emoji, "🎵")
	}
	if meta.Requires == nil {
		t.Fatal("requires is nil")
	}
	if len(meta.Requires.AnyBins) != 2 {
		t.Fatalf("anyBins: got %d, want 2", len(meta.Requires.AnyBins))
	}
	if meta.Requires.AnyBins[0] != "spogo" || meta.Requires.AnyBins[1] != "spotify_player" {
		t.Errorf("anyBins: got %v", meta.Requires.AnyBins)
	}
	if len(meta.Install) != 1 {
		t.Fatalf("install: got %d actions, want 1", len(meta.Install))
	}
	if meta.Install[0].Kind != "brew" || meta.Install[0].Formula != "spogo" {
		t.Errorf("install[0]: got kind=%q formula=%q", meta.Install[0].Kind, meta.Install[0].Formula)
	}
}

func TestMergeOpenClawMetadata_SingleLine(t *testing.T) {
	frontmatter := `name: skill-creator
description: Create or update AgentSkills.
metadata: { "openclaw": { "emoji": "🛠️" } }`

	sl := &SkillsLoader{}
	meta := &SkillMetadata{}
	sl.mergeOpenClawMetadata(frontmatter, meta)

	if meta.Emoji != "🛠️" {
		t.Errorf("emoji: got %q, want %q", meta.Emoji, "🛠️")
	}
}

func TestMergeOpenClawMetadata_WithBins(t *testing.T) {
	frontmatter := `name: himalaya
description: CLI to manage emails.
metadata:
  {
    "openclaw":
      {
        "emoji": "📧",
        "requires": { "bins": ["himalaya"] },
        "install":
          [
            {
              "id": "brew",
              "kind": "brew",
              "formula": "himalaya",
              "bins": ["himalaya"],
              "label": "Install Himalaya (brew)",
            },
          ],
      },
  }`

	sl := &SkillsLoader{}
	meta := &SkillMetadata{}
	sl.mergeOpenClawMetadata(frontmatter, meta)

	if meta.Requires == nil {
		t.Fatal("requires is nil")
	}
	if len(meta.Requires.Bins) != 1 || meta.Requires.Bins[0] != "himalaya" {
		t.Errorf("bins: got %v, want [himalaya]", meta.Requires.Bins)
	}
	if len(meta.Install) != 1 {
		t.Fatalf("install: got %d, want 1", len(meta.Install))
	}
}

func TestMergeOpenClawMetadata_NoMetadata(t *testing.T) {
	frontmatter := `name: simple-skill
description: A simple skill.`

	sl := &SkillsLoader{}
	meta := &SkillMetadata{}
	sl.mergeOpenClawMetadata(frontmatter, meta)

	if meta.Emoji != "" || meta.Requires != nil || meta.Install != nil {
		t.Error("expected no metadata changes for frontmatter without metadata key")
	}
}

func TestGetSkillMetadata_Integration(t *testing.T) {
	// Create a temp skill file with openclaw metadata.
	// ListSkills scans {workspace}/skills/{name}/SKILL.md
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0o755)

	content := `---
name: test-skill
description: A test skill with dependencies.
homepage: https://example.com
metadata:
  {
    "openclaw":
      {
        "emoji": "🧪",
        "requires": { "bins": ["curl"], "anyBins": ["jq", "yq"] },
        "install":
          [
            {
              "id": "brew-jq",
              "kind": "brew",
              "formula": "jq",
              "bins": ["jq"],
              "label": "Install jq (brew)",
            },
          ],
      },
  }
---

# Test Skill

This is a test.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)

	sl := NewSkillsLoader(dir, "", "")
	skills := sl.ListSkills()

	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}

	s := skills[0]
	if s.Emoji != "🧪" {
		t.Errorf("emoji: got %q, want %q", s.Emoji, "🧪")
	}
	if s.Requires == nil {
		t.Fatal("requires is nil")
	}
	if len(s.Requires.Bins) != 1 || s.Requires.Bins[0] != "curl" {
		t.Errorf("bins: got %v", s.Requires.Bins)
	}
	if len(s.Requires.AnyBins) != 2 {
		t.Errorf("anyBins: got %v", s.Requires.AnyBins)
	}
	if len(s.Install) != 1 {
		t.Errorf("install: got %d, want 1", len(s.Install))
	}
}
