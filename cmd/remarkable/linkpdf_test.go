package main

import (
	"bytes"
	"fmt"
	"strings"
)

// firstPageObject is the object number of page 0 in a PDF built by
// buildLinkPDF: object 1 is the catalog and object 2 the page tree.
const firstPageObject = 3

// pdfLink is a link annotation on a page built by buildLinkPDF.
type pdfLink struct {
	// rect is the annotation's /Rect as written: [Left Bottom Right Top].
	rect [4]float64

	// target is the annotation's /Dest or /A entry, such as
	// "/Dest [" + pdfPageRef(1) + " /Fit]" or "/A << /S /URI /URI (x) >>".
	target string
}

// pdfPage is a 200x200-point page built by buildLinkPDF.
type pdfPage struct {
	// content is the page's content stream, which may draw text in Helvetica
	// as font /F1; empty means the page has no content.
	content string

	links []pdfLink
}

// pdfPageRef returns the indirect reference to page i of a PDF built by
// buildLinkPDF, for use in a link target.
func pdfPageRef(i int) string {
	return fmt.Sprintf("%d 0 R", firstPageObject+i)
}

// buildLinkPDF returns a PDF 1.4 file with the given pages and a
// cross-reference table that addresses every object at its byte offset.
// Objects are numbered: catalog, page tree, the pages in order, the font,
// then each page's link annotations and content stream.
func buildLinkPDF(pages []pdfPage) []byte {
	fontObject := firstPageObject + len(pages)
	objects := make([]string, fontObject)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = pdfPageRef(i)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))
	objects[fontObject-1] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"

	add := func(object string) string {
		objects = append(objects, object)
		return fmt.Sprintf("%d 0 R", len(objects))
	}
	for i, page := range pages {
		annots := make([]string, len(page.links))
		for j, l := range page.links {
			annots[j] = add(fmt.Sprintf("<< /Type /Annot /Subtype /Link /Rect [%g %g %g %g] %s >>",
				l.rect[0], l.rect[1], l.rect[2], l.rect[3], l.target))
		}
		contents := ""
		if page.content != "" {
			contents = " /Contents " + add(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(page.content), page.content))
		}
		objects[firstPageObject-1+i] = fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 %d 0 R >> >>%s /Annots [%s] >>",
			fontObject, contents, strings.Join(annots, " "))
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return buf.Bytes()
}
