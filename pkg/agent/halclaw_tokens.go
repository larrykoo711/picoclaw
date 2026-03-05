package agent

import (
	"unicode"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// estimateMessagesTokens estimates the total token count for a message history,
// using CJK-aware heuristics that are more accurate than the upstream chars/2.5.
//
// Heuristics:
//   - CJK characters (U+4E00..U+9FFF, U+3400..U+4DBF, U+F900..U+FAFF, etc.): ~1.5 tokens/char
//   - ASCII/Latin: ~4 chars/token (0.25 tokens/char)
//   - Each message adds ~4 tokens overhead (role, separators)
//   - ToolCall arguments are also counted
func estimateMessagesTokens(messages []providers.Message) int {
	total := 0
	for _, m := range messages {
		total += estimateStringTokens(m.Content)
		total += estimateStringTokens(m.ReasoningContent)
		for _, tc := range m.ToolCalls {
			total += estimateStringTokens(tc.Name)
			for _, v := range tc.Arguments {
				if s, ok := v.(string); ok {
					total += estimateStringTokens(s)
				}
			}
		}
		total += 4 // per-message overhead
	}
	return total
}

// estimateStringTokens estimates token count for a single string.
func estimateStringTokens(s string) int {
	if s == "" {
		return 0
	}

	var cjkChars, otherChars int
	for _, r := range s {
		if isCJK(r) {
			cjkChars++
		} else {
			otherChars++
		}
	}

	// CJK: ~1.5 tokens per character (multiply by 3, divide by 2)
	// Other: ~0.25 tokens per character (divide by 4)
	cjkTokens := cjkChars * 3 / 2
	otherTokens := otherChars / 4

	return cjkTokens + otherTokens
}

// isCJK returns true if the rune is a CJK character.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r)
}
