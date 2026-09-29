package planner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

func ResolvePlannerDocument(ctx context.Context, client *cloud.Client, name string) (*cloud.Item, int, error) {
	year := time.Now().Year()
	if name == "" {
		name = fmt.Sprintf("%d - Daily", year)
	}

	item, err := client.Resolve(ctx, name)
	if err != nil {
		return nil, year, fmt.Errorf("resolving planner document %q: %w", name, err)
	}

	if match := regexp.MustCompile(`(\d{4})`).FindString(item.Metadata.VisibleName); match != "" {
		if y, err := strconv.Atoi(match); err == nil {
			year = y
		}
	}

	return item, year, nil
}

func EnsureStationeryPDF(ctx context.Context, client *cloud.Client, item *cloud.Item, cacheDir string) (string, error) {
	cachedPDF := filepath.Join(cacheDir, item.ID+".pdf")
	if info, err := os.Stat(cachedPDF); err == nil && info.Size() > 0 {
		return cachedPDF, nil
	}

	manifest, err := client.GetManifest(ctx, item.Hash, item.ID)
	if err != nil {
		return "", fmt.Errorf("fetching planner manifest: %w", err)
	}

	entry := manifest.Find(item.ID + ".pdf")
	if entry == nil {
		entry = manifest.FindSuffix(".pdf")
	}
	if entry == nil {
		return "", fmt.Errorf("planner document %q has no PDF stationery template", item.Metadata.VisibleName)
	}

	data, err := client.GetBlob(ctx, entry.Hash, item.ID+".pdf")
	if err != nil {
		return "", fmt.Errorf("downloading template PDF: %w", err)
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}
	if err := os.WriteFile(cachedPDF, data, 0644); err != nil {
		return "", fmt.Errorf("writing cached template PDF: %w", err)
	}

	return cachedPDF, nil
}

func SyncPlanner(ctx context.Context, client *cloud.Client, opts SyncOptions) ([]SyncResult, error) {
	if opts.DPI <= 0 {
		opts.DPI = render.DefaultDPI
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "modules/remarkable/data"
	}

	targetDates := opts.Dates
	if len(targetDates) == 0 {
		targetDates = DefaultTargets(opts.OutputDir, time.Now())
	}

	docItem, year, err := ResolvePlannerDocument(ctx, client, opts.DocName)
	if err != nil {
		return nil, err
	}

	templatePDF, err := EnsureStationeryPDF(ctx, client, docItem, opts.CacheDir)
	if err != nil {
		return nil, err
	}

	dateIndex, err := render.IndexPlannerDates(templatePDF, year)
	if err != nil {
		return nil, fmt.Errorf("indexing planner dates: %w", err)
	}

	docContent, err := client.GetDocumentContent(ctx, docItem.Hash, docItem.ID)
	if err != nil {
		return nil, fmt.Errorf("fetching planner content schema: %w", err)
	}

	manifest, err := client.GetManifest(ctx, docItem.Hash, docItem.ID)
	if err != nil {
		return nil, fmt.Errorf("fetching planner manifest: %w", err)
	}

	var results []SyncResult

	for _, date := range targetDates {
		pageMap, ok := dateIndex[date]
		if !ok {
			results = append(results, SyncResult{
				Date:  date,
				State: "absent",
			})
			continue
		}

		for _, kind := range []string{"day", "notes"} {
			pageIdx, pageFound := pageMap[kind]
			if !pageFound {
				continue
			}

			outPath := filepath.Join(opts.OutputDir, fmt.Sprintf("%s-%s.png", date, kind))

			if !opts.Force {
				if _, err := os.Stat(outPath); err == nil {
					results = append(results, SyncResult{
						Date:  date,
						Kind:  kind,
						Path:  outPath,
						State: "skipped",
					})
					continue
				}
			}

			if pageIdx >= len(docContent.Pages) {
				continue
			}
			pageID := docContent.Pages[pageIdx]
			rmFilename := fmt.Sprintf("%s/%s.rm", docItem.ID, pageID)

			var rmBytes []byte
			entry := manifest.Find(rmFilename)
			if entry == nil {
				entry = manifest.FindSuffix(pageID + ".rm")
			}
			if entry != nil {
				rmBytes, err = client.GetBlob(ctx, entry.Hash, rmFilename)
				if err != nil {
					return results, fmt.Errorf("downloading strokes for %s %s: %w", date, kind, err)
				}
			}

			pngBytes, err := render.RenderPlannerPage(templatePDF, pageIdx, rmBytes, opts.DPI)
			if err != nil {
				return results, fmt.Errorf("rendering %s %s: %w", date, kind, err)
			}

			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return results, fmt.Errorf("creating directory for %s: %w", outPath, err)
			}
			if err := os.WriteFile(outPath, pngBytes, 0644); err != nil {
				return results, fmt.Errorf("writing %s: %w", outPath, err)
			}

			results = append(results, SyncResult{
				Date:  date,
				Kind:  kind,
				Path:  outPath,
				State: "written",
			})
		}
	}

	return results, nil
}
