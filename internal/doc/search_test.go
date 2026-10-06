package doc

import (
	"strings"
	"testing"
	"unicode/utf8"
)

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

			got, ok := searchPageText(tt.text, tt.query)
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
