package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

func TestTransferSettingsNumericEquality(t *testing.T) {
	for _, field := range []string{"viewport", "document-timestamp", "page-timestamp", "document-nested", "page-nested"} {
		for _, row := range []struct {
			source, destination string
			equal               bool
		}{
			{"9", "9.0", true}, {"9.00", "9e0", true}, {"-9", "-90e-1", true}, {"0", "-0.000e+2", true}, {"0.09", "9e-2", true}, {"90.0", "9E+1", true},
			{"9007199254740993", "9007199254740993.0", true}, {"9007199254740993", "9007199254740992", false}, {"9", "0.9", false}, {"9e-2", "9e+2", false},
		} {
			t.Run(field+"/"+row.source+"/"+row.destination, func(t *testing.T) {
				f := settingsFixture(t, "pages", "cPages", "", func(src, dst []byte) ([]byte, []byte) {
					return settingsNumberContent(t, src, dst, field, row.source, row.destination)
				})
				before := bytes.Clone(f.originalDestination[settingsDestinationID+".content"])
				result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}})
				if !row.equal {
					if err == nil || f.puts != 0 {
						t.Fatalf("distinct numbers treated as equal: result=%+v err=%v writes=%d", result, err, f.puts)
					}
					return
				}
				if err != nil || result.State != cloud.UpdateVerified || f.puts != 0 || len(result.ViewportDifferences) != 0 {
					t.Fatalf("equivalent numbers conflicted or wrote: result=%+v err=%v writes=%d", result, err, f.puts)
				}
				if !bytes.Equal(f.file(t, settingsDestinationID, ".content"), before) {
					t.Fatal("no-op changed raw numeric tokens")
				}
				f.assertPreserved(t)
			})
		}
	}
}

func TestTransferSettingsExtremeNestedNumbers(t *testing.T) {
	for _, row := range []struct {
		source, destination string
		equal               bool
	}{
		{"1e1000001", "10e1000000", true}, {"1e-1000001", "10e-1000002", true}, {"1e999999999999999999999999", "10e999999999999999999999998", true}, {"1e999999999999999999999999", "1e999999999999999999999998", false},
	} {
		t.Run(row.source+"/"+row.destination, func(t *testing.T) {
			f := settingsFixture(t, "cPages", "pages", "", func(src, dst []byte) ([]byte, []byte) {
				return settingsNumberContent(t, src, dst, "page-nested", row.source, row.destination)
			})
			result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}})
			if row.equal && (err != nil || result.State != cloud.UpdateVerified) {
				t.Fatalf("extreme equivalent JSON numbers: %+v %v", result, err)
			}
			if !row.equal && err == nil {
				t.Fatal("distinct extreme exponent accepted")
			}
			if f.puts != 0 {
				t.Fatalf("numeric comparison unexpectedly wrote %d blobs", f.puts)
			}
		})
	}
}

func TestTransferSettingsNumericTokensSurviveWrite(t *testing.T) {
	f := settingsFixture(t, "cPages", "pages", "", func(src, dst []byte) ([]byte, []byte) {
		src, dst = settingsNumberContent(t, src, dst, "page-nested", "1e999999999999999999999999", "0")
		var source, destination map[string]json.RawMessage
		if err := json.Unmarshal(src, &source); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(dst, &destination); err != nil {
			t.Fatal(err)
		}
		source["customZoomScale"] = json.RawMessage("9.00e-1")
		source["tags"] = json.RawMessage(`[{"name":"exact","timestamp":9007199254740993.000e+0}]`)
		destination["tags"], destination["pageTags"] = json.RawMessage("[]"), json.RawMessage("[]")
		src, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		dst, err = json.Marshal(destination)
		if err != nil {
			t.Fatal(err)
		}
		return src, dst
	})
	result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: []SettingsPageMap{{SourcePage: 0, DestinationPage: 1}}, ReplaceViewport: true})
	if err != nil || result.State != cloud.UpdateVerified || f.puts != 3 {
		t.Fatalf("numeric write: result=%+v err=%v writes=%d", result, err, f.puts)
	}
	var source, destination map[string]json.RawMessage
	if err := json.Unmarshal(f.originalSource[settingsSourceID+".content"], &source); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(f.file(t, settingsDestinationID, ".content"), &destination); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tags", "customZoomScale"} {
		if !bytes.Equal(source[key], destination[key]) {
			t.Errorf("%s raw numeric tokens changed: %s != %s", key, destination[key], source[key])
		}
	}
	var sourceTags, destinationTags []map[string]json.RawMessage
	if err := json.Unmarshal(source["pageTags"], &sourceTags); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(destination["pageTags"], &destinationTags); err != nil {
		t.Fatal(err)
	}
	if len(sourceTags) != 1 || len(destinationTags) != 1 || !bytes.Equal(destinationTags[0]["pageId"], []byte(`"d1"`)) {
		t.Fatalf("page-tag mapping changed: %s", destination["pageTags"])
	}
	for key, value := range sourceTags[0] {
		if key != "pageId" && !bytes.Equal(value, destinationTags[0][key]) {
			t.Errorf("page-tag %s raw numeric tokens changed: %s != %s", key, destinationTags[0][key], value)
		}
	}
	f.assertPreserved(t)
}

func settingsNumberContent(t *testing.T, src, dst []byte, field, sourceNumber, destinationNumber string) ([]byte, []byte) {
	t.Helper()
	var source, destination map[string]json.RawMessage
	if err := json.Unmarshal(src, &source); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(dst, &destination); err != nil {
		t.Fatal(err)
	}
	// All settings begin semantically equal so a numeric equivalence must
	// remain a no-op rather than merely surviving another field's update.
	destination["tags"] = bytes.Clone(source["tags"])
	destination["pageTags"] = bytes.Replace(source["pageTags"], []byte(`"s0"`), []byte(`"d1"`), 1)
	switch field {
	case "viewport":
		source["customZoomScale"], destination["customZoomScale"] = json.RawMessage(sourceNumber), json.RawMessage(destinationNumber)
	default:
		key := "tags"
		page := false
		if field == "page-timestamp" || field == "page-nested" {
			key = "pageTags"
			page = true
		}
		tag := func(number, id string) json.RawMessage {
			timestamp, payload := number, `{"keep":true}`
			if field == "page-nested" || field == "document-nested" {
				timestamp = "123"
				payload = fmt.Sprintf(`{"nested":[{"number":%s},[null,true,"number",%s]]}`, number, number)
			}
			pageID := ""
			if page {
				pageID = fmt.Sprintf(`,"pageId":%q`, id)
			}
			return json.RawMessage(fmt.Sprintf(`[{"name":"numeric-tag","timestamp":%s,"extra":%s%s}]`, timestamp, payload, pageID))
		}
		source[key], destination[key] = tag(sourceNumber, "s0"), tag(destinationNumber, "d1")
	}
	src, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	dst, err = json.Marshal(destination)
	if err != nil {
		t.Fatal(err)
	}
	return src, dst
}
