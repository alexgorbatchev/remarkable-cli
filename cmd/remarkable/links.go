package main

import (
	"fmt"
	"strconv"
	"strings"

	render "github.com/alexgorbatchev/go-remarkable-render"
	"github.com/alexgorbatchev/remarkable-cli/internal/agent"
)

// linkFields returns the link index, target page, URI, text, and rectangle
// that doc links prints for l in both output modes. The target is "-" when l
// leads to no page of the document.
func linkFields(l render.PageLink) []string {
	target := "-"
	if l.TargetPage >= 0 {
		target = strconv.Itoa(l.TargetPage)
	}
	return []string{strconv.Itoa(l.Index), target, printableURI(l.URI), agent.PrintableText(l.Text), formatRect(l.Rect)}
}

// formatRect returns r as "left,bottom,right,top" in the PDF points the
// library reports. Each coordinate is the shortest decimal that parses back to
// its float32 value, so a /Rect entry written as 98.6 prints as 98.6 rather
// than as the shortest decimal of that value widened to float64,
// 98.5999984741211. 'f' never switches to exponent notation. PDFium reads a
// real number beyond the float32 range as an infinity, which prints as +Inf
// or -Inf.
func formatRect(r render.Rect) string {
	coordinates := []float32{r.Left, r.Bottom, r.Right, r.Top}
	fields := make([]string, len(coordinates))
	for i, v := range coordinates {
		fields[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return strings.Join(fields, ",")
}

// printableURI returns uri with every byte that could split a TSV field or a
// table row, or reach the terminal as a control, percent-encoded as RFC 3986
// section 2.1 defines (an uppercase "%XX" per byte). A PDF /URI string is raw
// bytes that PDFium does not validate. printableURI encodes each byte of an
// invalid UTF-8 sequence, and each UTF-8 byte of the ASCII space or of a
// character agent.DecodeChar does not report printable. The ASCII space and
// controls are never allowed unencoded in a URI or an IRI. For a rejected
// character in RFC 3987's ucschar or iprivate set, such as a non-ASCII space
// or a line separator, other than the bidirectional formatting characters RFC
// 3987 section 4.1 forbids, encoding its UTF-8 bytes is the IRI-to-URI mapping
// of RFC 3987 section 3.1, which locates the same resource. The rest, those
// bidirectional formatting characters and characters outside both ucschar and
// iprivate, such as C1 controls and tag characters, cannot appear in a valid
// IRI; they are encoded only to keep the output printable. Every other
// character, '%' and non-ASCII letters included, is kept, so a URI that needs
// no encoding is returned unchanged. net/url has no function for this: its
// escape functions also encode reserved characters such as '/' and '?', and
// url.Parse rejects ASCII control characters.
func printableURI(uri string) string {
	var b strings.Builder
	for i := 0; i < len(uri); {
		r, size, printable := agent.DecodeChar(uri[i:])
		if printable && r != ' ' {
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
