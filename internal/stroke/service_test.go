package stroke

import (
	"strings"
	"testing"

	"github.com/alexgorbatchev/go-rmscene"
)

func TestInspectAndExport(t *testing.T) {
	// Build minimal valid v6 rm file with 1 line
	header := []byte(rmscene.HeaderV6)
	// We can test with invalid bytes first
	_, err := Inspect([]byte("invalid"))
	if err == nil {
		t.Fatal("expected error on invalid header")
	}

	// Valid header with no blocks
	stats, err := Inspect(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Lines != 0 || stats.Points != 0 {
		t.Errorf("expected 0 lines and 0 points, got %d and %d", stats.Lines, stats.Points)
	}

	// Test SVG Export with valid header
	svg, err := ExportSVG(header, 100, 200)
	if err != nil {
		t.Fatalf("unexpected error exporting SVG: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Errorf("expected <svg tag, got: %s", svg)
	}
}
