package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	render "github.com/alexgorbatchev/go-remarkable-render"

	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

// linkFields returns the link index, target page, URI, text, and rectangle
// that doc links prints for l in both output modes. The target is "-" when l
// leads to no page of the document.
func linkFields(l doc.PageLink) []string {
	target := "-"
	if l.TargetPage >= 0 {
		target = strconv.Itoa(l.TargetPage)
	}
	return []string{strconv.Itoa(l.Index), target, printableURI(l.URI), printableText(l.Text), formatRect(l.Rect)}
}

// formatRect returns r as "left,bottom,right,top" in the PDF points the
// library reports. Each coordinate is the shortest decimal that parses back to
// its float32 value, so a /Rect entry written as 98.6 prints as 98.6 rather
// than as the shortest decimal of that value widened to float64,
// 98.5999984741211. 'f' never switches to exponent notation.
func formatRect(r render.Rect) string {
	coordinates := []float32{r.Left, r.Bottom, r.Right, r.Top}
	fields := make([]string, len(coordinates))
	for i, v := range coordinates {
		fields[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return strings.Join(fields, ",")
}

// decodeChar decodes the first character of s, which must not be empty. It
// returns utf8.RuneError and size 1 for an invalid UTF-8 sequence, as
// utf8.DecodeRuneInString does. printable reports a valid character that
// unicode.IsPrint accepts: a letter, mark, number, punctuation character,
// symbol, or the ASCII space. Any other character, such as a control, a format
// character like a bidirectional override, a non-ASCII space, or a line or
// paragraph separator, could split a TSV field or a table row, reach the
// terminal as a control, or be invisible.
func decodeChar(s string) (r rune, size int, printable bool) {
	r, size = utf8.DecodeRuneInString(s)
	valid := r != utf8.RuneError || size > 1
	return r, size, valid && unicode.IsPrint(r)
}

// printableURI returns uri with every byte that could split a TSV field or a
// table row, or reach the terminal as a control, percent-encoded as RFC 3986
// section 2.1 defines (an uppercase "%XX" per byte). A PDF /URI string is raw
// bytes that PDFium does not validate. printableURI encodes each byte of an
// invalid UTF-8 sequence, and each UTF-8 byte of the ASCII space or of a
// character decodeChar does not report printable. The ASCII space and
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
		r, size, printable := decodeChar(uri[i:])
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

// printableText returns text with every character decodeChar does not report
// printable, and every backslash, replaced by its Go escape sequence. The
// library collapses white space in link text, but other controls, such as
// ESC, and format characters remain. A rejected character takes the escape
// strconv.QuoteRune writes for it (\a, \b, \f, \n, \r, \t, or \v; \xNN for
// another ASCII control; otherwise \uNNNN, or \UNNNNNNNN above U+FFFF; hex
// digits in lowercase), a backslash becomes \\,
// and each byte of an invalid UTF-8 sequence becomes \xNN as in strconv.Quote.
// Every other character, the space, '%', and quotation marks included, is
// kept, so text that needs no escaping is returned unchanged, and each
// backslash in the result starts an escape that strconv.UnquoteChar with
// quote 0 decodes. Percent-encoding, as printableURI uses, would make a
// literal '%' in prose ambiguous; strconv.Quote would also escape '"' and add
// quotation marks around every value.
func printableText(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		r, size, printable := decodeChar(text[i:])
		switch {
		case printable && r != '\\':
			b.WriteString(text[i : i+size])
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, text[i])
		default:
			quoted := strconv.QuoteRune(r)
			// Drop the single quotation marks around the character literal.
			b.WriteString(quoted[1 : len(quoted)-1])
		}
		i += size
	}
	return b.String()
}
