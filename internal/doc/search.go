package doc

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Maximum snippet context around a match, in runes (Unicode code points).
const (
	snippetRunesBefore = 20
	snippetRunesAfter  = 40
)

// SearchQuery is the text to find on document pages and how to match it.
type SearchQuery struct {
	Text string
	// WholeWord keeps only matches that no letter, decimal digit, or
	// combining mark immediately precedes or follows.
	WholeWord bool
}

var errBlankSearchQuery = errors.New("query must not be empty or only whitespace")

// normalizedQuery is a SearchQuery in the form matching compares, prepared
// once per search and applied to every page.
type normalizedQuery struct {
	text      string
	wholeWord bool
}

// normalizeQuery normalizes query.Text with normalizeSearchText and rejects a
// query left with no text, which would match every page.
func normalizeQuery(query SearchQuery) (normalizedQuery, error) {
	text := normalizeSearchText(query.Text)
	if text == "" {
		return normalizedQuery{}, errBlankSearchQuery
	}
	return normalizedQuery{text: text, wholeWord: query.WholeWord}, nil
}

// ValidateSearchQuery rejects a query that normalizes to no text, which would
// match every page.
func ValidateSearchQuery(query string) error {
	_, err := normalizeQuery(SearchQuery{Text: query})
	return err
}

// normalizeSearchText returns s in the form in which snippets show page text
// and matching compares page text and queries; see collapseWhitespace.
func normalizeSearchText(s string) string {
	return collapseWhitespace(s, nil)
}

// searchPageText reports whether query occurs in a page's text and returns a
// snippet around the first occurrence. Matching sees the page text the way
// snippets show it: every whitespace run is one space and leading and
// trailing whitespace is dropped. Matched runes compare under Unicode simple
// case folding.
func searchPageText(text string, query normalizedQuery) (snippet string, ok bool) {
	var offsets []int
	page := collapseWhitespace(text, &offsets)

	for from := 0; ; {
		start, end, ok := indexFold(page[from:], query.text)
		if !ok {
			return "", false
		}
		start, end = from+start, from+end
		if !query.wholeWord || isWholeWord(page, start, end) {
			return snippetAround(text, offsets[start], offsets[end]), true
		}
		if start == len(page) {
			return "", false
		}
		_, size := utf8.DecodeRuneInString(page[start:])
		from = start + size
	}
}

// collapseWhitespace returns text with every whitespace run, as
// strings.Fields splits on, replaced by one space and leading and trailing
// whitespace dropped; non-space bytes are copied unchanged. When offsets is
// not nil, it receives, for each byte offset of the result plus its length,
// the corresponding byte offset in text. A collapsed space maps to the end of
// the content before its run, and the result's length maps to the end of the
// last non-space rune, so a match that starts and ends on non-space runes maps
// to exactly the bytes it covers in text.
func collapseWhitespace(text string, offsets *[]int) string {
	var b strings.Builder
	b.Grow(len(text))
	if offsets != nil {
		*offsets = make([]int, 0, len(text)+1)
	}

	contentEnd := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsSpace(r) {
			if contentEnd > 0 && contentEnd < i {
				if offsets != nil {
					*offsets = append(*offsets, contentEnd)
				}
				b.WriteByte(' ')
			}
			if offsets != nil {
				for j := i; j < i+size; j++ {
					*offsets = append(*offsets, j)
				}
			}
			b.WriteString(text[i : i+size])
			contentEnd = i + size
		}
		i += size
	}
	if offsets != nil {
		*offsets = append(*offsets, contentEnd)
	}
	return b.String()
}

// isWholeWord reports whether s[start:end] has no letter, decimal digit, or
// combining mark immediately before or after it, so the match neither extends
// nor is extended by a longer run of word characters ("Oct 1" in "Oct 10").
// Combining marks count because they belong to the preceding base character.
func isWholeWord(s string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); isWordRune(r) {
			return false
		}
	}
	if end < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

// indexFold returns the byte offsets in s of the first substring, starting at
// a rune boundary, that equals substr under Unicode simple case folding as
// defined by strings.EqualFold; ok reports whether such a substring exists.
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
// runes before and snippetRunesAfter runes after, stepping by whole runes so a
// cut never splits a multi-byte character, with runs of whitespace collapsed to
// single spaces and leading/trailing whitespace trimmed. start and end must be
// rune boundaries of text.
func snippetAround(text string, start, end int) string {
	for i := 0; i < snippetRunesBefore && start > 0; i++ {
		_, size := utf8.DecodeLastRuneInString(text[:start])
		start -= size
	}
	for i := 0; i < snippetRunesAfter && end < len(text); i++ {
		_, size := utf8.DecodeRuneInString(text[end:])
		end += size
	}
	return normalizeSearchText(text[start:end])
}
