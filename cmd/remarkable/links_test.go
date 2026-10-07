package main

import (
	"testing"
)

// unsafeLinkURI is the escaped URI of link 3 in uri_links.pdf, whose /URI
// string holds a tab, line feed, carriage return, ESC, space, DEL, U+202E
// (right-to-left override), U+0085 (next line), U+00A0 (no-break space), and
// the byte 0xFF, which is not valid UTF-8.
const unsafeLinkURI = "https://example.com/a%09b%0Ac%0Dd%1B[31me%20f%7Fg%E2%80%AEh%C2%85i%C2%A0j%FFk"

// TestDocLinksTargetsAndURIs lists the links of uri_links.pdf page 0: an
// internal link keeps its 0-based target and #page fragment, a URI action
// prints "-" with its URI, a launch action prints "-" with an empty URI, and
// URI bytes that could break a TSV field or reach the terminal as controls are
// percent-encoded while a printable URI, '%' and "é" included, is unchanged.
func TestDocLinksTargetsAndURIs(t *testing.T) {
	cases := []struct {
		agent string
		want  string
	}{
		{"1", "0\t1\t#page=2\n" +
			"1\t-\thttps://example.com/docs?q=café&p=100%25#top\n" +
			"2\t-\t\n" +
			"3\t-\t" + unsafeLinkURI + "\n"},
		{"0", "┌─────────┬─────────────┬───────────────────────────────────────────────────────────────────────────────┐\n" +
			"│ LINK  # │ TARGET PAGE │                                      URI                                      │\n" +
			"├─────────┼─────────────┼───────────────────────────────────────────────────────────────────────────────┤\n" +
			"│ 0       │ 1           │ #page=2                                                                       │\n" +
			"│ 1       │ -           │ https://example.com/docs?q=café&p=100%25#top                                  │\n" +
			"│ 2       │ -           │                                                                               │\n" +
			"│ 3       │ -           │ " + unsafeLinkURI + " │\n" +
			"└─────────┴─────────────┴───────────────────────────────────────────────────────────────────────────────┘\n"},
	}
	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			t.Setenv("AGENT", tc.agent)
			ts, _ := setupCLITestEnv(t)
			defer ts.Close()
			out, err := executeRoot("doc", "links", "urilinks-1", "--page", "0", "--no-cache")
			if err != nil {
				t.Fatalf("doc links failed: %v; output: %q", err, out)
			}
			if out != tc.want {
				t.Errorf("doc links output =\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}

// TestPrintableURI covers the encoding rule on inputs the fixture PDF does not
// hold: a printable URI is returned unchanged, U+FFFD written as valid UTF-8
// is printable and kept, a truncated UTF-8 sequence is encoded byte by byte,
// and a soft hyphen, a zero-width space, and a line separator are encoded.
func TestPrintableURI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"printable", "mailto:a@example.com?subject=%E2%9C%93", "mailto:a@example.com?subject=%E2%9C%93"},
		{"valid replacement character", "https://example.com/�", "https://example.com/�"},
		{"truncated sequence", "https://example.com/\xE2\x80", "https://example.com/%E2%80"},
		{"invisible characters", "a­b​c d", "a%C2%ADb%E2%80%8Bc%E2%80%A8d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := printableURI(tc.in); got != tc.want {
				t.Errorf("printableURI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
