package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

// linkFields returns the link index, target page, and URI that doc links
// prints for l in both output modes. The target is "-" when l leads to no page
// of the document.
func linkFields(l doc.PageLink) []string {
	target := "-"
	if l.TargetPage >= 0 {
		target = strconv.Itoa(l.TargetPage)
	}
	return []string{strconv.Itoa(l.Index), target, printableURI(l.URI)}
}

// printableURI returns uri with every byte that could split a TSV field or a
// table row, or reach the terminal as a control, percent-encoded as RFC 3986
// section 2.1 defines (an uppercase "%XX" per byte). A PDF /URI string is raw
// bytes that PDFium does not validate. printableURI encodes each byte of an
// invalid UTF-8 sequence, and each UTF-8 byte of a space or of a character
// unicode.IsPrint rejects: controls, format characters such as bidirectional
// overrides, non-ASCII spaces, and line and paragraph separators. Spaces and
// controls are never allowed unencoded in a URI or an IRI; for the non-ASCII
// characters, encoding their UTF-8 bytes is the IRI-to-URI mapping of RFC 3987
// section 3.1, which identifies the same resource. Every other character, '%'
// and non-ASCII letters included, is kept, so a URI that needs no encoding is
// returned unchanged. net/url has no function for this: its escape functions
// also encode reserved characters such as '/' and '?', and url.Parse rejects
// control characters.
func printableURI(uri string) string {
	var b strings.Builder
	for i := 0; i < len(uri); {
		r, size := utf8.DecodeRuneInString(uri[i:])
		valid := r != utf8.RuneError || size > 1
		if valid && r != ' ' && unicode.IsPrint(r) {
			b.WriteString(uri[i : i+size])
		} else {
			for _, c := range []byte(uri[i : i+size]) {
				fmt.Fprintf(&b, "%%%02X", c)
			}
		}
		i += size
	}
	return b.String()
}
