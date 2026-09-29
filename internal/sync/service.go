package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
)

var (
	ErrCloudUnavailable = errors.New("reMarkable Cloud unavailable")
	ErrDateNotFound     = errors.New("date not found in planner")
)

type SyncResult struct {
	Date  string
	Kind  string
	Path  string
	State string // "written", "skipped", "absent"
}

type SyncOptions struct {
	Dates      []string
	OutputDir  string
	DPI        int
	DocName    string
	ConfigFile string
	CacheDir   string
	Force      bool
}

func LastBusinessDay(from time.Time) time.Time {
	cursor := from.AddDate(0, 0, -1)
	for cursor.Weekday() == time.Saturday || cursor.Weekday() == time.Sunday {
		cursor = cursor.AddDate(0, 0, -1)
	}
	return cursor
}

func CapturedDates(dataDir string) ([]string, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	re := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-day\.png$`)
	dateSet := make(map[string]bool)

	for _, entry := range entries {
		if match := re.FindStringSubmatch(entry.Name()); match != nil {
			dateSet[match[1]] = true
		}
	}

	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	return dates, nil
}

func DefaultTargets(dataDir string, today time.Time) []string {
	captured, _ := CapturedDates(dataDir)
	capturedMap := make(map[string]bool, len(captured))
	for _, c := range captured {
		capturedMap[c] = true
	}

	todayStr := today.Format("2006-01-02")
	targets := []string{todayStr}

	prev := LastBusinessDay(today).Format("2006-01-02")
	if !capturedMap[prev] {
		targets = append(targets, prev)
	}

	sort.Strings(targets)
	return targets
}

func ResolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	home, err := os.UserHomeDir()
	if err == nil {
		homeRmapi := filepath.Join(home, ".rmapi")
		if _, err := os.Stat(homeRmapi); err == nil {
			return homeRmapi
		}
	}
	p, _ := cloud.ResolveConfigPath("")
	return p
}

func ResolveCacheDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		return filepath.Join(cacheDir, "remarkable-sync")
	}
	return filepath.Join(".tmp", "remarkable_cache")
}

func ResolveOutputDir(dir string) string {
	if dir == "" {
		dir = filepath.Join("modules", "remarkable", "data")
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	candidates := []string{
		filepath.Join("..", dir),
		filepath.Join("..", "..", dir),
		filepath.Join("..", "..", "..", dir),
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return dir
}

func SyncPlanner(ctx context.Context, opts SyncOptions) ([]SyncResult, error) {
	if opts.DPI <= 0 {
		opts.DPI = 200
	}
	opts.OutputDir = ResolveOutputDir(opts.OutputDir)

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	var neededDates []string
	var results []SyncResult

	// Upfront check: skip dates that already exist on disk
	for _, d := range opts.Dates {
		dayPath := filepath.Join(opts.OutputDir, fmt.Sprintf("%s-day.png", d))
		notesPath := filepath.Join(opts.OutputDir, fmt.Sprintf("%s-notes.png", d))

		_, dayErr := os.Stat(dayPath)
		_, notesErr := os.Stat(notesPath)

		if dayErr == nil && notesErr == nil && !opts.Force {
			results = append(results,
				SyncResult{Date: d, Kind: "day", Path: dayPath, State: "skipped"},
				SyncResult{Date: d, Kind: "notes", Path: notesPath, State: "skipped"},
			)
		} else {
			neededDates = append(neededDates, d)
		}
	}

	if len(neededDates) == 0 {
		return results, nil
	}

	configPath := ResolveConfigPath(opts.ConfigFile)
	client, err := cloud.NewClient(
		cloud.WithConfigFile(configPath),
		cloud.WithAutoRenew(true),
		cloud.WithSaveOnRenew(true),
	)
	if err != nil {
		return results, fmt.Errorf("%w: %v", ErrCloudUnavailable, err)
	}

	year := time.Now().Year()
	docName := opts.DocName
	if docName == "" {
		docName = fmt.Sprintf("%d - Daily", year)
	}

	item, err := client.ResolveByName(ctx, docName)
	if err != nil {
		// try exact ID if name didn't match directly
		item, err = client.ResolveByID(ctx, docName)
		if err != nil {
			return results, fmt.Errorf("resolve planner document '%s': %w", docName, err)
		}
	}

	manifest, err := item.GetManifest(ctx)
	if err != nil {
		return results, fmt.Errorf("get document manifest: %w", err)
	}

	content, err := item.GetContent(ctx)
	if err != nil {
		return results, fmt.Errorf("get document content: %w", err)
	}

	redirToPageID := make(map[int]string)
	for _, p := range content.CPages.Pages {
		if p.Redir != nil {
			redirToPageID[p.Redir.Value] = p.ID
		}
	}

	// Cache stationery background template PDF
	pdfEntry := manifest.FindSuffix(".pdf")
	if pdfEntry == nil {
		return results, fmt.Errorf("no background PDF found in planner document")
	}

	cacheDir := ResolveCacheDir(opts.CacheDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return results, fmt.Errorf("create cache dir: %w", err)
	}

	cachedPDFPath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.pdf", item.ID, pdfEntry.Hash[:min(16, len(pdfEntry.Hash))]))
	if _, err := os.Stat(cachedPDFPath); os.IsNotExist(err) {
		pdfBytes, err := client.GetBlob(ctx, pdfEntry.Hash, pdfEntry.ID)
		if err != nil {
			return results, fmt.Errorf("download template PDF: %w", err)
		}
		if err := os.WriteFile(cachedPDFPath, pdfBytes, 0o644); err != nil {
			return results, fmt.Errorf("write cached PDF: %w", err)
		}
	}

	plannerDates, err := render.IndexPlannerDates(cachedPDFPath, year)
	if err != nil {
		return results, fmt.Errorf("index planner dates: %w", err)
	}

	manifestRMMap := make(map[string]cloud.SchemaEntry)
	for _, e := range manifest.Entries {
		if strings.HasSuffix(e.ID, ".rm") {
			manifestRMMap[e.ID] = e
		}
	}

	for _, dateStr := range neededDates {
		dateViews, ok := plannerDates[dateStr]
		if !ok {
			results = append(results,
				SyncResult{Date: dateStr, Kind: "day", Path: "", State: "absent"},
				SyncResult{Date: dateStr, Kind: "notes", Path: "", State: "absent"},
			)
			continue
		}

		for _, kind := range []string{"day", "notes"} {
			outPath := filepath.Join(opts.OutputDir, fmt.Sprintf("%s-%s.png", dateStr, kind))

			if _, err := os.Stat(outPath); err == nil && !opts.Force {
				results = append(results, SyncResult{Date: dateStr, Kind: kind, Path: outPath, State: "skipped"})
				continue
			}

			pdfIdx, hasView := dateViews[kind]
			if !hasView {
				results = append(results, SyncResult{Date: dateStr, Kind: kind, Path: outPath, State: "absent"})
				continue
			}

			pageUUID := redirToPageID[pdfIdx]
			var rmBytes []byte

			if pageUUID != "" {
				rmID := fmt.Sprintf("%s/%s.rm", item.ID, pageUUID)
				if rmRecord, ok := manifestRMMap[rmID]; ok {
					rmBytes, err = client.GetBlob(ctx, rmRecord.Hash, rmRecord.ID)
					if err != nil {
						// non-fatal stroke fetch error, fall back to empty strokes
						rmBytes = nil
					}
				}
			}

			pngBytes, err := render.RenderPlannerPage(cachedPDFPath, pdfIdx, rmBytes, opts.DPI)
			if err != nil {
				return results, fmt.Errorf("render %s %s: %w", dateStr, kind, err)
			}

			if err := os.WriteFile(outPath, pngBytes, 0o644); err != nil {
				return results, fmt.Errorf("write %s: %w", outPath, err)
			}

			results = append(results, SyncResult{Date: dateStr, Kind: kind, Path: outPath, State: "written"})
		}
	}

	return results, nil
}
