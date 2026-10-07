package main

import (
	"testing"
)

// URIs of the URI action links of uriLinksPDF as its PDF literal strings
// write them. The octal escapes name the UTF-8 bytes of "é" (\303\251), U+202E
// right-to-left override (\342\200\256), U+0085 next line (\302\205), and
// U+00A0 no-break space (\302\240); \377 is a byte that is never valid UTF-8.
// unsafeLinkURIPDF also holds a tab, line feed, carriage return, ESC, space,
// and DEL. unsafeLinkURI is what doc links prints for it.
const (
	printableLinkURIPDF = "https://example.com/docs?q=caf\\303\\251&p=100%25#top"
	unsafeLinkURIPDF    = "https://example.com/a\\011b\\012c\\015d\\033[31me f\\177g\\342\\200\\256h\\302\\205i\\302\\240j\\377k"
	unsafeLinkURI       = "https://example.com/a%09b%0Ac%0Dd%1B[31me%20f%7Fg%E2%80%AEh%C2%85i%C2%A0j%FFk"
)

// uriLinksPDF returns the urilinks-1 PDF. Page 0 links, in order, to page 1,
// to printableLinkURIPDF, through a launch action, and to unsafeLinkURIPDF.
func uriLinksPDF() []byte {
	return buildLinkPDF([]pdfPage{
		{links: []pdfLink{
			{rect: [4]float64{10, 10, 50, 50}, target: "/Dest [" + pdfPageRef(1) + " /Fit]"},
			{rect: [4]float64{60, 10, 100, 50}, target: "/A << /S /URI /URI (" + printableLinkURIPDF + ") >>"},
			{rect: [4]float64{110, 10, 150, 50}, target: "/A << /S /Launch /F (notes.txt) >>"},
			{rect: [4]float64{10, 60, 50, 100}, target: "/A << /S /URI /URI (" + unsafeLinkURIPDF + ") >>"},
		}},
		{},
	})
}

// TestDocLinksTargetsAndURIs lists the links of uriLinksPDF page 0: an
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
		{"valid replacement character", "https://example.com/\uFFFD", "https://example.com/\uFFFD"},
		{"truncated sequence", "https://example.com/\xE2\x80", "https://example.com/%E2%80"},
		{"invisible characters", "a\u00ADb\u200Bc\u2028d", "a%C2%ADb%E2%80%8Bc%E2%80%A8d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := printableURI(tc.in); got != tc.want {
				t.Errorf("printableURI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
