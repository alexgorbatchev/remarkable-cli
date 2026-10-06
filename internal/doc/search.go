package doc

import (
	"strings"
	"unicode/utf8"
)

// Snippet context around a match, counted in characters (runes) so that a cut
// never splits a multi-byte character.
const (
	snippetRunesBefore = 20
	snippetRunesAfter  = 40
)

// searchPageText reports whether query occurs case-insensitively in a page's
// text and returns a snippet around the first occurrence.
func searchPageText(text, query string) (snippet string, ok bool) {
	start, end, ok := indexFold(text, query)
	if !ok {
		return "", false
	}
	return snippetAround(text, start, end), true
}

// indexFold returns the byte offsets in s of the first substring that equals
// substr under Unicode simple case folding, as defined by strings.EqualFold.
// EqualFold compares rune by rune, so a match spans exactly as many runes as
// substr. Sliding a window of that many runes over s keeps both offsets on
// rune boundaries of s itself, unlike offsets taken from a case-mapped copy
// whose byte length can differ from s.
func indexFold(s, substr string) (start, end int, ok bool) {
	for range utf8.RuneCountInString(substr) {
		if end == len(s) {
			return 0, 0, false
		}
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
	}

	for {
		if strings.EqualFold(s[start:end], substr) {
			return start, end, true
		}
		if end == len(s) {
			return 0, 0, false
		}
		_, size := utf8.DecodeRuneInString(s[start:])
		start += size
		_, size = utf8.DecodeRuneInString(s[end:])
		end += size
	}
}

// snippetAround returns text[start:end] widened by up to snippetRunesBefore
// runes before and snippetRunesAfter runes after, with runs of whitespace
// collapsed to single spaces. start and end must be rune boundaries of text.
func snippetAround(text string, start, end int) string {
	for i := 0; i < snippetRunesBefore && start > 0; i++ {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		start -= size
	}
	for i := 0; i < snippetRunesAfter && end < len(text); i++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
	}
	return strings.Join(strings.Fields(text[start:end]), " ")
}
