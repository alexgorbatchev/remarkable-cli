package main

import (
	"testing"

	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
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

// labelledLinksContent is the content stream of uriLinksPDF page 1. It draws,
// in 12-point Helvetica, "Notes" and "Standup" on the baseline y=150, and
// "Ctrl" followed by ESC (\033) and "[31m", and "a\b", on the baseline y=100.
const labelledLinksContent = "BT /F1 12 Tf 20 150 Td (Notes) Tj ET\n" +
	"BT /F1 12 Tf 100 150 Td (Standup) Tj ET\n" +
	"BT /F1 12 Tf 20 100 Td (Ctrl\\033[31m) Tj ET\n" +
	"BT /F1 12 Tf 100 100 Td (a\\\\b) Tj ET"

// overflowingReal is a PDF real, a number written with a decimal point,
// beyond the float32 range (about 3.4e38). PDFium reads it as +Inf.
const overflowingReal = "999999999999999999999999999999999999999999999999.5"

// uriLinksPDF returns the urilinks-1 PDF. Page 0 has no text and links, in
// order, to page 1, to printableLinkURIPDF, through a launch action, and to
// unsafeLinkURIPDF. Page 1 draws labelledLinksContent and links, in order,
// "Notes" to page 0; "Standup", through a /Rect with fractional coordinates
// listing its top-right corner first, to a URI; "Ctrl" with ESC and "a\b" to
// page 0; and an area without text to page 0. Seven more links to page 0 cover
// no text and test /Rect parsing: one without a /Rect; one whose right edge is
// overflowingReal; one whose left edge is its negation; a three-element /Rect;
// a /Rect whose last element is a name; a /Rect holding the largest unsigned
// integer PDFium reads (4294967295) and the next one; and a /Rect holding the
// smallest signed integer PDFium reads, the next one below it, and a signed
// integer above the signed 32-bit range.
func uriLinksPDF() []byte {
	toPage0 := "/Dest [" + pdfPageRef(0) + " /Fit]"
	return buildLinkPDF([]pdfPage{
		{links: []pdfLink{
			{rect: "10 10 50 50", target: "/Dest [" + pdfPageRef(1) + " /Fit]"},
			{rect: "60 10 100 50", target: "/A << /S /URI /URI (" + printableLinkURIPDF + ") >>"},
			{rect: "110 10 150 50", target: "/A << /S /Launch /F (notes.txt) >>"},
			{rect: "10 60 50 100", target: "/A << /S /URI /URI (" + unsafeLinkURIPDF + ") >>"},
		}},
		{content: labelledLinksContent, links: []pdfLink{
			{rect: "18 146 52 162", target: toPage0},
			{rect: "147.9 161.7 98.6 145.3", target: "/A << /S /URI /URI (https://example.com/standup) >>"},
			{rect: "18 96 90 112", target: toPage0},
			{rect: "98 96 130 112", target: toPage0},
			{rect: "20 20 60 40", target: toPage0},
			{target: toPage0},
			{rect: "0 0 " + overflowingReal + " 10", target: toPage0},
			{rect: "-" + overflowingReal + " 0 0 10", target: toPage0},
			{rect: "1 2 3", target: toPage0},
			{rect: "1 2 3 /Foo", target: toPage0},
			{rect: "0 0 4294967295 4294967296", target: toPage0},
			{rect: "-2147483648 -2147483649 +2147483648 10", target: toPage0},
		}},
	})
}

// runDocLinks runs doc links on page of urilinks-1 with AGENT set to agent
// and fails the test unless the output is want.
func runDocLinks(t *testing.T, agent, page, want string) {
	t.Helper()
	t.Setenv("AGENT", agent)
	ts, _ := setupCLITestEnv(t)
	defer ts.Close()
	out, err := executeRoot("doc", "links", "urilinks-1", "--page", page, "--no-cache")
	if err != nil {
		t.Fatalf("doc links failed: %v; output: %q", err, out)
	}
	if out != want {
		t.Errorf("doc links output =\n%s\nwant\n%s", out, want)
	}
}

// TestDocLinksTargetsAndURIs lists the links of uriLinksPDF page 0: an
// internal link keeps its 0-based target and #page fragment, a URI action
// prints "-" with its URI, a launch action prints "-" with an empty URI, and
// URI bytes that could break a TSV field or reach the terminal as controls are
// percent-encoded while a printable URI, '%' and "é" included, is unchanged.
// The page has no text, so every link has an empty TEXT.
func TestDocLinksTargetsAndURIs(t *testing.T) {
	cases := []struct {
		agent string
		want  string
	}{
		{"1", "0\t1\t#page=2\t\t10,10,50,50\n" +
			"1\t-\thttps://example.com/docs?q=café&p=100%25#top\t\t60,10,100,50\n" +
			"2\t-\t\t\t110,10,150,50\n" +
			"3\t-\t" + unsafeLinkURI + "\t\t10,60,50,100\n"},
		{"0", "┌─────────┬─────────────┬───────────────────────────────────────────────────────────────────────────────┬──────┬───────────────┐\n" +
			"│ LINK  # │ TARGET PAGE │                                      URI                                      │ TEXT │     RECT      │\n" +
			"├─────────┼─────────────┼───────────────────────────────────────────────────────────────────────────────┼──────┼───────────────┤\n" +
			"│ 0       │ 1           │ #page=2                                                                       │      │ 10,10,50,50   │\n" +
			"│ 1       │ -           │ https://example.com/docs?q=café&p=100%25#top                                  │      │ 60,10,100,50  │\n" +
			"│ 2       │ -           │                                                                               │      │ 110,10,150,50 │\n" +
			"│ 3       │ -           │ " + unsafeLinkURI + " │      │ 10,60,50,100  │\n" +
			"└─────────┴─────────────┴───────────────────────────────────────────────────────────────────────────────┴──────┴───────────────┘\n"},
	}
	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			runDocLinks(t, tc.agent, "0", tc.want)
		})
	}
}

// TestDocLinksTextAndRect lists the links of uriLinksPDF page 1. Each link
// prints the page text inside its rectangle, with ESC and the backslash
// escaped, and its rectangle as left,bottom,right,top in PDF points: corners
// listed in either order are normalized, a fractional coordinate prints the
// shortest decimal of its float32 value (98.6, not 98.5999984741211), and a
// link over no text has an empty TEXT. A link without a /Rect, or with one
// that is not a four-element array, prints 0,0,0,0; a real number beyond the
// float32 range prints +Inf or -Inf; and an element that is not a number, an
// unsigned integer above 4294967295, or a signed integer outside the signed
// 32-bit range reads as 0 before the corners are ordered.
func TestDocLinksTextAndRect(t *testing.T) {
	cases := []struct {
		agent string
		want  string
	}{
		{"1", "0\t0\t#page=1\tNotes\t18,146,52,162\n" +
			"1\t-\thttps://example.com/standup\tStandup\t98.6,145.3,147.9,161.7\n" +
			"2\t0\t#page=1\tCtrl\\x1b[31m\t18,96,90,112\n" +
			"3\t0\t#page=1\ta\\\\b\t98,96,130,112\n" +
			"4\t0\t#page=1\t\t20,20,60,40\n" +
			"5\t0\t#page=1\t\t0,0,0,0\n" +
			"6\t0\t#page=1\t\t0,0,+Inf,10\n" +
			"7\t0\t#page=1\t\t-Inf,0,0,10\n" +
			"8\t0\t#page=1\t\t0,0,0,0\n" +
			"9\t0\t#page=1\t\t1,0,3,2\n" +
			"10\t0\t#page=1\t\t0,0,4294967300,0\n" +
			"11\t0\t#page=1\t\t-2147483600,0,0,10\n"},
		{"0", "┌─────────┬─────────────┬─────────────────────────────┬──────────────┬────────────────────────┐\n" +
			"│ LINK  # │ TARGET PAGE │             URI             │     TEXT     │          RECT          │\n" +
			"├─────────┼─────────────┼─────────────────────────────┼──────────────┼────────────────────────┤\n" +
			"│ 0       │ 0           │ #page=1                     │ Notes        │ 18,146,52,162          │\n" +
			"│ 1       │ -           │ https://example.com/standup │ Standup      │ 98.6,145.3,147.9,161.7 │\n" +
			"│ 2       │ 0           │ #page=1                     │ Ctrl\\x1b[31m │ 18,96,90,112           │\n" +
			"│ 3       │ 0           │ #page=1                     │ a\\\\b         │ 98,96,130,112          │\n" +
			"│ 4       │ 0           │ #page=1                     │              │ 20,20,60,40            │\n" +
			"│ 5       │ 0           │ #page=1                     │              │ 0,0,0,0                │\n" +
			"│ 6       │ 0           │ #page=1                     │              │ 0,0,+Inf,10            │\n" +
			"│ 7       │ 0           │ #page=1                     │              │ -Inf,0,0,10            │\n" +
			"│ 8       │ 0           │ #page=1                     │              │ 0,0,0,0                │\n" +
			"│ 9       │ 0           │ #page=1                     │              │ 1,0,3,2                │\n" +
			"│ 10      │ 0           │ #page=1                     │              │ 0,0,4294967300,0       │\n" +
			"│ 11      │ 0           │ #page=1                     │              │ -2147483600,0,0,10     │\n" +
			"└─────────┴─────────────┴─────────────────────────────┴──────────────┴────────────────────────┘\n"},
	}
	for _, tc := range cases {
		t.Run(tc.agent, func(t *testing.T) {
			runDocLinks(t, tc.agent, "1", tc.want)
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

// TestPrintableText covers the escaping rule on inputs the fixture PDF does
// not hold. Printable text, the space, '%', '"', and letters outside ASCII
// included, is unchanged; a backslash doubles; a character unicode.IsPrint
// rejects takes the escape strconv.QuoteRune gives it; and each byte of an
// invalid UTF-8 sequence becomes \xNN, while U+FFFD written as valid UTF-8 is
// kept. Decoding each escape with strconv.UnquoteChar restores the input.
func TestPrintableText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"printable", `Café "Notes" 100% done`, `Café "Notes" 100% done`},
		{"backslash", `C:\notes\x1b`, `C:\\notes\\x1b`},
		{"C0 controls and DEL", "a\tb\nc\ad\x1be\x7ff", `a\tb\nc\ad\x1be\x7ff`},
		{"C1 control", "a\u009bb", `a\u009bb`},
		{"format characters", "a\u00ADb\u200Bc\u202Ed", `a\u00adb\u200bc\u202ed`},
		{"line separator", "a\u2028b", `a\u2028b`},
		{"private use", "a\uE000b", `a\ue000b`},
		{"supplementary format character", "a\U000E0001b", `a\U000e0001b`},
		{"valid replacement character", "a\uFFFDb", "a\uFFFDb"},
		{"invalid byte", "a\xFFb", `a\xffb`},
		{"truncated sequence", "a\xE2\x80", `a\xe2\x80`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := agent.PrintableText(tc.in)
			if got != tc.want {
				t.Errorf("printableText(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if decoded := unescapeText(t, got); decoded != tc.in {
				t.Errorf("decoding printableText(%q) = %q gives %q", tc.in, got, decoded)
			}
		})
	}
}

// unescapeText decodes text escaped by printableText with strconv.UnquoteChar,
// which returns a \xNN escape as a byte and any other escape or character as a
// code point.
func unescapeText(t *testing.T, s string) string {
	t.Helper()
	decoded, err := agent.UnescapeText(s)
	if err != nil {
		t.Fatalf("agent.UnescapeText(%q): %v", s, err)
	}
	return decoded
}
