package stroke

import (
	"fmt"
	"os"

	"github.com/alexgorbatchev/go-rmscene"
)

// StrokeStats holds parsed statistics of a .rm file.
type StrokeStats struct {
	TotalBlocks int
	Lines       int
	Points      int
	Tools       map[string]int
	Colors      map[string]int
}

// Inspect parses v6 .rm bytes and returns structural metrics.
func Inspect(rmBytes []byte) (*StrokeStats, error) {
	scene, err := rmscene.Parse(rmBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing stroke file: %w", err)
	}

	stats := &StrokeStats{
		TotalBlocks: len(scene.Blocks),
		Tools:       make(map[string]int),
		Colors:      make(map[string]int),
	}

	for _, b := range scene.Blocks {
		lineBlock, ok := b.(*rmscene.SceneLineItemBlock)
		if !ok || lineBlock.Item.Value == nil {
			continue
		}
		line := lineBlock.Item.Value
		stats.Lines++
		stats.Points += len(line.Points)

		toolName := line.Tool.String()
		stats.Tools[toolName]++

		colorName := line.Color.String()
		stats.Colors[colorName]++
	}

	return stats, nil
}

// ExportSVG converts .rm bytes directly into layered SVG.
func ExportSVG(rmBytes []byte, widthPt, heightPt float64) (string, error) {
	var opts []rmscene.SVGOption
	if widthPt > 0 && heightPt > 0 {
		opts = append(opts, rmscene.WithDimensions(widthPt, heightPt))
	}
	return rmscene.RenderRMToSVG(rmBytes, opts...)
}

// ReadFile reads a stroke file from disk.
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
