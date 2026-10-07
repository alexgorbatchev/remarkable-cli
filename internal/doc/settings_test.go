package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

func TestTransferSettingsPreservesDocuments(t *testing.T) {
	for _, sourceSchema := range []string{"pages", "cPages"} {
		for _, destinationSchema := range []string{"pages", "cPages"} {
			t.Run(sourceSchema+"/"+destinationSchema, func(t *testing.T) {
				f := settingsFixture(t, sourceSchema, destinationSchema, "")
				result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}})
				if err != nil {
					t.Fatal(err)
				}
				if result.State != cloud.UpdateVerified || len(result.Uploaded) != 1 || result.Uploaded[0] != settingsDestinationID+".content" {
					t.Fatalf("progress: %+v", result)
				}
				var got map[string]json.RawMessage
				if err := json.Unmarshal(f.file(t, settingsDestinationID, ".content"), &got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(got["pageTags"], []byte(`"pageId":"d1"`)) || !bytes.Contains(got["pageTags"], []byte(`"keep-unmapped"`)) || !bytes.Contains(got["pageTags"], []byte(`"extra":{"value":9007199254740993}`)) {
					t.Fatalf("tags not retained/remapped: %s", got["pageTags"])
				}
				for _, key := range []string{"tags", "zoomMode", "viewBackgroundFilter", "customZoomCenterX", "customZoomCenterY", "customZoomOrientation", "customZoomPageHeight", "customZoomPageWidth", "customZoomScale"} {
					var src map[string]json.RawMessage
					if err := json.Unmarshal(f.originalSource[settingsSourceID+".content"], &src); err != nil {
						t.Fatal(err)
					}
					var actual, expected any
					if err := json.Unmarshal(got[key], &actual); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(src[key], &expected); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(actual, expected) {
						t.Errorf("%s: %s != %s", key, got[key], src[key])
					}
				}
				var before map[string]json.RawMessage
				if err := json.Unmarshal(f.originalDestination[settingsDestinationID+".content"], &before); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"pages", "cPages", "pageCount", "redirectionPageMap", "unknown", "customZoomFuture"} {
					if !bytes.Equal(got[key], before[key]) {
						t.Errorf("unrelated %s changed: %s != %s", key, got[key], before[key])
					}
				}
				f.assertPreserved(t)
				puts := f.puts
				repeated, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}})
				if err != nil || repeated.State != cloud.UpdateVerified || f.puts != puts {
					t.Fatalf("idempotent transfer: %+v %v writes=%d/%d", repeated, err, f.puts, puts)
				}
			})
		}
	}
}

func TestTransferSettingsPreflight(t *testing.T) {
	for _, scenario := range []string{"missing-map", "duplicate-source", "duplicate-destination", "source-bounds", "destination-bounds", "identity", "uuid", "uninitialized", "native-id-duplicate", "page-count", "deleted", "disagree", "unknown-page-tag", "tag-conflict", "document-tag-conflict", "malformed-tags", "null-zoom", "unknown-zoom", "null-tags", "duplicate-key", "folder", "corrupt-source", "corrupt-destination", "source-change", "destination-change", "root-change"} {
		t.Run(scenario, func(t *testing.T) {
			f := settingsFixture(t, "cPages", "cPages", scenario)
			mapping := []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}
			src, dst := settingsSourceID, settingsDestinationID
			switch scenario {
			case "missing-map":
				mapping = []SettingsPageMap{{SourcePage: 1, DestinationPage: 0}}
			case "duplicate-source":
				mapping = append(mapping, SettingsPageMap{SourcePage: 0, DestinationPage: 0})
			case "duplicate-destination":
				mapping = append(mapping, SettingsPageMap{SourcePage: 1, DestinationPage: 1})
			case "source-bounds":
				mapping[0].SourcePage = 2
			case "destination-bounds":
				mapping[0].DestinationPage = 2
			case "identity":
				dst = src
			case "uuid":
				src = "source"
			}
			_, err := TransferSettings(context.Background(), f.client, src, dst, SettingsOptions{Mapping: mapping, ReplaceViewport: true})
			if err == nil || f.puts != 0 || f.commits != 0 {
				t.Fatalf("preflight %s: err=%v puts=%d commits=%d", scenario, err, f.puts, f.commits)
			}
		})
	}
}

func TestTransferSettingsReportsMissingDocument(t *testing.T) {
	const absent = "55555555-5555-4555-8555-555555555555"
	for _, ids := range [][2]string{{absent, settingsDestinationID}, {settingsSourceID, absent}} {
		t.Run(ids[0]+"/"+ids[1], func(t *testing.T) {
			f := settingsFixture(t, "cPages", "cPages", "")
			_, err := TransferSettings(context.Background(), f.client, ids[0], ids[1], SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}})
			if !errors.Is(err, cloud.ErrItemNotFound) || !strings.Contains(err.Error(), absent+" is missing from root") || f.puts != 0 {
				t.Fatalf("transfer with missing document: err=%v puts=%d, want ErrItemNotFound and no uploads", err, f.puts)
			}
		})
	}
}

func TestTransferSettingsViewportPolicy(t *testing.T) {
	for _, scenario := range []string{"viewport-conflict", "viewport-delete", "viewport-add", "no-page-tags", "cached"} {
		for _, replace := range []bool{false, true} {
			t.Run(scenario+"/"+map[bool]string{false: "reject", true: "replace"}[replace], func(t *testing.T) {
				f := settingsFixture(t, "pages", "cPages", scenario)
				mapping := []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}
				if scenario == "no-page-tags" {
					mapping = nil
				}
				result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: mapping, ReplaceViewport: replace})
				conflict := strings.HasPrefix(scenario, "viewport-")
				if conflict && !replace {
					if err == nil || result == nil || len(result.ViewportDifferences) == 0 || f.puts != 0 {
						t.Fatalf("missing explicit conflict: %+v %v puts=%d", result, err, f.puts)
					}
					return
				}
				if err != nil || result.State != cloud.UpdateVerified {
					t.Fatalf("transfer: %+v %v", result, err)
				}
				if scenario == "viewport-delete" && bytes.Contains(f.file(t, settingsDestinationID, ".content"), []byte(`"viewBackgroundFilter"`)) {
					t.Fatal("source omission did not transfer")
				}
				f.assertPreserved(t)
			})
		}
	}
}

func TestTransferSettingsPartialStates(t *testing.T) {
	for _, row := range []struct {
		scenario string
		state    cloud.UpdateState
	}{
		{"stage-failure", cloud.UpdateStaged}, {"stage-corrupt", cloud.UpdateStaged}, {"generation-conflict", cloud.UpdateStaged}, {"commit-unknown", cloud.UpdateCommitUnknown}, {"content-corrupt", cloud.UpdateCommitted}, {"pdf-corrupt", cloud.UpdateCommitted}, {"stroke-corrupt", cloud.UpdateCommitted}, {"source-corrupt-after", cloud.UpdateCommitted}, {"source-change-after", cloud.UpdateCommitted}, {"destination-change-after", cloud.UpdateCommitted}, {"verify-root-change", cloud.UpdateCommitted},
	} {
		t.Run(row.scenario, func(t *testing.T) {
			f := settingsFixture(t, "pages", "cPages", row.scenario)
			result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}})
			if err == nil || result == nil || result.State != row.state {
				t.Fatalf("state: %+v err=%v, want %s", result, err, row.state)
			}
			if row.state != cloud.UpdateStaged && (len(result.Uploaded) != 1 || result.RootHash == "") {
				t.Fatalf("lost progress: %+v", result)
			}
		})
	}
}

func TestLoadSettingsMapping(t *testing.T) {
	for _, row := range []struct {
		data  string
		valid bool
	}{
		{`[{"source_page":0,"destination_page":1}]`, true}, {`[]`, true}, {`null`, false}, {`{}`, false}, {`[{"source_page":0}]`, false}, {`[{"source_page":null,"destination_page":1}]`, false}, {`[{"source_page":-1,"destination_page":1}]`, false}, {`[{"source_page":0.5,"destination_page":1}]`, false}, {`[{"Source_Page":0,"destination_page":1}]`, false}, {`[{"source_page":0,"destination_page":1,"extra":1}]`, false}, {`[{"source_page":0,"source_page":1,"destination_page":1}]`, false}, {`[] null`, false},
	} {
		t.Run(row.data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mapping.json")
			if err := os.WriteFile(path, []byte(row.data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSettingsMapping(path)
			if (err == nil) != row.valid {
				t.Fatalf("mapping valid=%t: %v", row.valid, err)
			}
		})
	}
}
