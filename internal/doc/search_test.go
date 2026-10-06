package doc

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// mustNormalizeQuery prepares a test query the way SearchDocument does.
func mustNormalizeQuery(t *testing.T, query SearchQuery) normalizedQuery {
	t.Helper()
	normalized, err := normalizeQuery(query)
	if err != nil {
		t.Fatalf("normalizeQuery(%+v): %v", query, err)
	}
	return normalized
}

func TestSearchPageText(t *testing.T) {
	const (
		kelvin  = "K" // KELVIN SIGN: 3 bytes, lowercases to 1-byte "k"
		aStroke = "Ⱥ" // LATIN CAPITAL LETTER A WITH STROKE: 2 bytes, lowercases to 3-byte "ⱥ"
	)

	tests := []struct {
		name, text, query string
		match             string // matched text as it appears in the page text
		want              string
	}{
		{
			name:  "curly apostrophe before match",
			text:  "Indigenous People’s Day, then follow up with the planning committee",
			query: "up",
			match: "up",
			want:  "’s Day, then follow up with the planning committee",
		},
		{
			name:  "kelvin signs before match",
			text:  strings.Repeat(kelvin, 30) + " needle tail",
			query: "needle",
			match: "needle",
			want:  strings.Repeat(kelvin, 19) + " needle tail",
		},
		{
			name:  "a with stroke before match",
			text:  strings.Repeat(aStroke, 40) + "x",
			query: "x",
			match: "x",
			want:  strings.Repeat(aStroke, 20) + "x",
		},
		{
			name:  "kelvin sign inside match",
			text:  "Measured in " + kelvin + "elvin today",
			query: "kelvin",
			match: kelvin + "elvin",
			want:  "Measured in " + kelvin + "elvin today",
		},
		{
			// Lowercasing maps Σ to σ but leaves final ς unchanged; simple
			// case folding equates all three.
			name:  "greek final sigma matched by case folding",
			text:  "ΟΔΟΣ end",
			query: "οδος",
			match: "ΟΔΟΣ",
			want:  "ΟΔΟΣ end",
		},
		{
			name:  "match shorter in page text than in query",
			text:  "aaaaa" + aStroke + strings.Repeat("b", 45),
			query: "ⱥ",
			match: aStroke,
			want:  "aaaaa" + aStroke + strings.Repeat("b", 40),
		},
		{
			name:  "context counted in characters",
			text:  strings.Repeat("é", 30) + "needle" + strings.Repeat("ü", 50),
			query: "NEEDLE",
			match: "needle",
			want:  strings.Repeat("é", 20) + "needle" + strings.Repeat("ü", 40),
		},
		{
			name:  "whitespace collapsed in snippet",
			text:  "Line one\nLine  two needle\tthree",
			query: "Needle",
			match: "needle",
			want:  "Line one Line two needle three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("searchPageText(%q, %q) panicked: %v", tt.text, tt.query, r)
				}
			}()

			got, ok := searchPageText(tt.text, mustNormalizeQuery(t, SearchQuery{Text: tt.query}))
			if !ok {
				t.Fatalf("searchPageText(%q, %q) found no match", tt.text, tt.query)
			}
			if !utf8.ValidString(got) {
				t.Errorf("snippet is not valid UTF-8: %q", got)
			}
			if !strings.Contains(got, tt.match) {
				t.Errorf("snippet %q does not contain matched text %q", got, tt.match)
			}
			if got != tt.want {
				t.Errorf("snippet = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSearchPageTextMatching(t *testing.T) {
	const noMatch = ""

	tests := []struct {
		name  string
		text  string
		query SearchQuery
		want  string // snippet; noMatch when the page must not match
	}{
		{
			name:  "query spans page line break",
			text:  "Fri 9 Mon 12\r\n2026 Oct 05 Mon Notes",
			query: SearchQuery{Text: "12 2026"},
			want:  "Fri 9 Mon 12 2026 Oct 05 Mon Notes",
		},
		{
			name:  "query whitespace run matches page line break",
			text:  "Fri 9 Mon 12\r\n2026 Oct 05 Mon Notes",
			query: SearchQuery{Text: "Mon  12\n2026"},
			want:  "Fri 9 Mon 12 2026 Oct 05 Mon Notes",
		},
		{
			name:  "page whitespace run of mixed spaces",
			text:  "Top \t Priority",
			query: SearchQuery{Text: "top priority"},
			want:  "Top Priority",
		},
		{
			name:  "query leading and trailing whitespace ignored",
			text:  "Mon Notes\r\nOct 05",
			query: SearchQuery{Text: "  Oct 05\r\n"},
			want:  "Mon Notes Oct 05",
		},
		{
			name:  "page whitespace is not removed",
			text:  "Mon 122026",
			query: SearchQuery{Text: "12 2026"},
			want:  noMatch,
		},
		{
			name:  "query whitespace is not removed",
			text:  "Mon 12\r\n2026",
			query: SearchQuery{Text: "122026"},
			want:  noMatch,
		},
		{
			name:  "snippet context counts page text characters",
			text:  strings.Repeat("x", 25) + "\r\n\r\nneedle",
			query: SearchQuery{Text: "needle"},
			want:  strings.Repeat("x", 16) + " needle",
		},
		{
			name:  "snippet trailing context counts page text characters",
			text:  "needle\r\n\r\n" + strings.Repeat("x", 50),
			query: SearchQuery{Text: "needle"},
			want:  "needle " + strings.Repeat("x", 36),
		},
		{
			name:  "invalid UTF-8 bytes keep their page offsets",
			text:  "a\xff\r\n\r\nneedle\xfe end",
			query: SearchQuery{Text: "needle"},
			want:  "a\xff needle\xfe end",
		},
		{
			name:  "substring matches longer token",
			text:  "Mon 5 Oct 12 Notes",
			query: SearchQuery{Text: "Oct 1"},
			want:  "Mon 5 Oct 12 Notes",
		},
		{
			name:  "word rejects longer numbers",
			text:  "Oct 10 Sat 11\r\nOct 12 Oct 19",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  noMatch,
		},
		{
			name:  "word skips longer token for later whole token",
			text:  strings.Repeat("Oct 12 ", 5) + "Wed\r\nOct 1, 2026 Notes",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  "Oct 12 Oct 12 Wed Oct 1, 2026 Notes",
		},
		{
			name:  "word matches before punctuation",
			text:  "Thursday Oct 1. Notes",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  "Thursday Oct 1. Notes",
		},
		{
			name:  "word matches at end of text",
			text:  "Thursday\r\nOct 1",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  "Thursday Oct 1",
		},
		{
			name:  "word matches at start of text",
			text:  "Oct 1 Fri 2",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  "Oct 1 Fri 2",
		},
		{
			name:  "word matches across page line break",
			text:  "Oct\r\n1\r\nNotes",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  "Oct 1 Notes",
		},
		{
			name:  "word matches under case folding",
			text:  "THURSDAY OCT 1",
			query: SearchQuery{Text: "oct 1", WholeWord: true},
			want:  "THURSDAY OCT 1",
		},
		{
			name:  "word rejects following letter",
			text:  "Friday Notes",
			query: SearchQuery{Text: "Fri", WholeWord: true},
			want:  noMatch,
		},
		{
			name:  "word rejects preceding letter",
			text:  "Friday Notes",
			query: SearchQuery{Text: "day", WholeWord: true},
			want:  noMatch,
		},
		{
			name:  "word rejects following non-ASCII digit",
			text:  "Oct 1١",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  noMatch,
		},
		{
			// U+0301 COMBINING ACUTE ACCENT continues the preceding letter.
			name:  "word rejects following combining mark",
			text:  "café menu",
			query: SearchQuery{Text: "cafe", WholeWord: true},
			want:  noMatch,
		},
		{
			name:  "word accepts surrounding symbols",
			text:  "Notes (Oct 1) done",
			query: SearchQuery{Text: "Oct 1", WholeWord: true},
			want:  "Notes (Oct 1) done",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := searchPageText(tt.text, mustNormalizeQuery(t, tt.query))
			if tt.want == noMatch {
				if ok {
					t.Fatalf("searchPageText(%q, %+v) matched with snippet %q, want no match", tt.text, tt.query, got)
				}
				return
			}
			if !ok {
				t.Fatalf("searchPageText(%q, %+v) found no match, want snippet %q", tt.text, tt.query, tt.want)
			}
			if got != tt.want {
				t.Errorf("snippet = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSearchPageTextWholeWordDate searches daily planner pages for October 1
// through 19, shaped as their PDF text layer extracts: a header line, then the
// day's sections on later lines.
func TestSearchPageTextWholeWordDate(t *testing.T) {
	for _, wholeWord := range []bool{false, true} {
		query := SearchQuery{Text: "Oct 1", WholeWord: wholeWord}
		normalized := mustNormalizeQuery(t, query)
		var matched []int
		for day := 1; day <= 19; day++ {
			text := fmt.Sprintf("Oct %d Fri 2 Mon 5\r\nThursday Notes 2026\r\nTop Priority", day)
			if _, ok := searchPageText(text, normalized); ok {
				matched = append(matched, day)
			}
		}

		want := []int{1, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
		if wholeWord {
			want = []int{1}
		}
		if !slices.Equal(matched, want) {
			t.Errorf("%+v matched days %v, want %v", query, matched, want)
		}
	}
}
