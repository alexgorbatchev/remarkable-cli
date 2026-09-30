package stroke

import (
	"strings"
	"testing"

	"github.com/alexgorbatchev/go-rmscene"
)

func TestInspectAndExport(t *testing.T) {
	// 1. ReadFile error
	_, err := ReadFile("/nonexistent/file.rm")
	if err == nil {
		t.Error("expected error on nonexistent file")
	}

	// 2. ReadFile and Inspect real strokes fixture
	fixturePath := "testdata/oct1_notes_strokes.rm"
	rmBytes, err := ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	stats, err := Inspect(rmBytes)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if stats.Lines != 393 || stats.Points != 12457 {
		t.Errorf("expected 393 lines and 12457 points, got %d and %d", stats.Lines, stats.Points)
	}
	if len(stats.Tools) == 0 || len(stats.Colors) == 0 {
		t.Errorf("expected tools and colors to be populated")
	}

	// 3. Inspect invalid bytes
	_, err = Inspect([]byte("invalid header"))
	if err == nil {
		t.Fatal("expected error on invalid header")
	}

	// 4. Valid header with no blocks
	header := []byte(rmscene.HeaderV6)
	statsEmpty, err := Inspect(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statsEmpty.Lines != 0 || statsEmpty.Points != 0 {
		t.Errorf("expected 0 lines and 0 points, got %d and %d", statsEmpty.Lines, statsEmpty.Points)
	}

	// 5. Test SVG Export with dimensions
	svg, err := ExportSVG(header, 100, 200)
	if err != nil {
		t.Fatalf("unexpected error exporting SVG: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Errorf("expected <svg tag, got: %s", svg)
	}

	// 6. Test SVG Export without dimensions
	svgNoDim, err := ExportSVG(header, 0, 0)
	if err != nil || !strings.Contains(svgNoDim, "<svg") {
		t.Errorf("expected valid SVG without dimensions")
	}
}
