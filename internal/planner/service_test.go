package planner_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexgorbatchev/remarkable-sync/internal/planner"
)

func TestLastBusinessDay(t *testing.T) {
	// Monday -> Friday
	monday := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	expectedFriday := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if got := planner.LastBusinessDay(monday); got.Day() != expectedFriday.Day() {
		t.Errorf("expected %v, got %v", expectedFriday, got)
	}

	// Sunday -> Friday
	sunday := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if got := planner.LastBusinessDay(sunday); got.Day() != expectedFriday.Day() {
		t.Errorf("expected %v, got %v", expectedFriday, got)
	}

	// Tuesday -> Monday
	tuesday := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if got := planner.LastBusinessDay(tuesday); got.Day() != monday.Day() {
		t.Errorf("expected %v, got %v", monday, got)
	}
}

func TestCapturedDates(t *testing.T) {
	tmpDir := t.TempDir()

	dates, err := planner.CapturedDates(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dates) != 0 {
		t.Errorf("expected 0 dates, got %d", len(dates))
	}

	// Create dummy capture files
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-25-day.png"), []byte("png"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-25-notes.png"), []byte("png"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-28-day.png"), []byte("png"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("txt"), 0o644)

	dates, err = planner.CapturedDates(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dates) != 2 || dates[0] != "2026-09-25" || dates[1] != "2026-09-28" {
		t.Errorf("expected [2026-09-25, 2026-09-28], got %v", dates)
	}
}

func TestDefaultTargets(t *testing.T) {
	tmpDir := t.TempDir()
	today := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	// When empty, targets should include today and previous business day (Friday)
	targets := planner.DefaultTargets(tmpDir, today)
	if len(targets) != 2 || targets[0] != "2026-09-25" || targets[1] != "2026-09-28" {
		t.Errorf("expected [2026-09-25, 2026-09-28], got %v", targets)
	}

	// When Friday is already captured, only today should be targeted
	_ = os.WriteFile(filepath.Join(tmpDir, "2026-09-25-day.png"), []byte("png"), 0o644)
	targets = planner.DefaultTargets(tmpDir, today)
	if len(targets) != 1 || targets[0] != "2026-09-28" {
		t.Errorf("expected [2026-09-28], got %v", targets)
	}
}
