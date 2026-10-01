package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

func TestTransferSettingsPageTagOrder(t *testing.T) {
	for _, sourceSchema := range []string{"pages", "cPages"} {
		for _, destinationSchema := range []string{"pages", "cPages"} {
			for _, appendTags := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/append=%t", sourceSchema, destinationSchema, appendTags), func(t *testing.T) {
					f := settingsFixture(t, sourceSchema, destinationSchema, "", func(src, dst []byte) ([]byte, []byte) {
						return settingsOrderContent(t, src, dst, appendTags)
					})
					before := bytes.Clone(f.originalDestination[settingsDestinationID+".content"])
					mapping := []SettingsPageMap{{SourcePage: 2, DestinationPage: 3}, {SourcePage: 0, DestinationPage: 1}, {SourcePage: 1, DestinationPage: 2}}
					result, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: mapping})
					if err != nil || result.State != cloud.UpdateVerified {
						t.Fatalf("ordered tags: result=%+v err=%v", result, err)
					}
					after := f.file(t, settingsDestinationID, ".content")
					if !appendTags {
						if f.puts != 0 || f.commits != 0 || !bytes.Equal(before, after) {
							t.Fatalf("identical interleaved tags changed destination: uploads=%d commits=%d before=%s after=%s", f.puts, f.commits, before, after)
						}
					} else {
						if f.puts != 3 || f.commits != 1 {
							t.Fatalf("appended tags: uploads=%d commits=%d", f.puts, f.commits)
						}
						settingsAssertTagAppend(t, f, before, after)
					}
					f.assertPreserved(t)
					puts, commits := f.puts, f.commits
					repeated, err := TransferSettings(context.Background(), f.client, settingsSourceID, settingsDestinationID, SettingsOptions{Mapping: mapping})
					if err != nil || repeated.State != cloud.UpdateVerified || f.puts != puts || f.commits != commits || !bytes.Equal(after, f.file(t, settingsDestinationID, ".content")) {
						t.Fatalf("repeat changed ordered tags: result=%+v err=%v uploads=%d/%d commits=%d/%d", repeated, err, f.puts, puts, f.commits, commits)
					}
				})
			}
		}
	}
}

func settingsOrderContent(t *testing.T, src, dst []byte, appendTags bool) ([]byte, []byte) {
	t.Helper()
	source, destination := settingsOrderFields(t, src), settingsOrderFields(t, dst)
	for _, row := range []struct {
		fields map[string]json.RawMessage
		prefix string
		count  int
	}{{source, "s", 3}, {destination, "d", 4}} {
		row.fields["pageCount"] = settingsOrderJSON(t, row.count)
		var ids []string
		var native []map[string]any
		for i := range row.count {
			id := fmt.Sprintf("%s%d", row.prefix, i)
			ids = append(ids, id)
			native = append(native, map[string]any{"id": id, "idx": map[string]any{"timestamp": fmt.Sprintf("1:%d", i+2), "value": fmt.Sprintf("b%d", i)}, "redir": map[string]any{"timestamp": "1:2", "value": i}})
		}
		if row.fields["pages"] != nil {
			row.fields["pages"] = settingsOrderJSON(t, ids)
			row.fields["redirectionPageMap"] = settingsOrderJSON(t, []int{0, 1, 2, 3}[:row.count])
		} else {
			pages := settingsOrderFields(t, row.fields["cPages"])
			pages["pages"] = settingsOrderJSON(t, native)
			pages["original"] = settingsOrderJSON(t, map[string]any{"timestamp": "1:1", "value": row.count})
			row.fields["cPages"] = settingsOrderJSON(t, pages)
		}
	}
	tag := func(name, id, number string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"name":%q,"pageId":%q,"timestamp":%s,"extra":{"number":%s}}`, name, id, number, number))
	}
	source["pageTags"] = settingsOrderJSON(t, []json.RawMessage{tag("second", "s1", "9007199254740993.000e+0"), tag("first-a", "s0", "9"), tag("third-a", "s2", "-0.000e+2"), tag("first-b", "s0", "9"), tag("third-b", "s2", "1e999999999999999999999999")})
	existing := []json.RawMessage{tag("first-a", "d1", "9.00"), tag("unmapped", "d0", "9007199254740993"), tag("first-b", "d1", "9e0")}
	if !appendTags {
		existing = []json.RawMessage{tag("third-a", "d3", "0"), existing[0], existing[1], tag("second", "d2", "9007199254740993"), existing[2], tag("third-b", "d3", "10e999999999999999999999998")}
	}
	destination["pageTags"] = settingsOrderJSON(t, existing)
	// Canonical object keys keep document tags byte-identical during an append;
	// these cases isolate page-tag row order and numeric-token preservation.
	var documentTags []map[string]json.RawMessage
	if err := json.Unmarshal(source["tags"], &documentTags); err != nil {
		t.Fatal(err)
	}
	source["tags"] = settingsOrderJSON(t, documentTags)
	destination["tags"] = bytes.Clone(source["tags"])
	return settingsOrderJSON(t, source), settingsOrderJSON(t, destination)
}

func settingsAssertTagAppend(t *testing.T, f *settingsCloud, before, after []byte) {
	t.Helper()
	old, got := settingsOrderFields(t, before), settingsOrderFields(t, after)
	var oldTags, gotTags, sourceTags []map[string]json.RawMessage
	for _, row := range []struct {
		raw  []byte
		tags *[]map[string]json.RawMessage
	}{{old["pageTags"], &oldTags}, {got["pageTags"], &gotTags}, {settingsOrderFields(t, f.originalSource[settingsSourceID+".content"])["pageTags"], &sourceTags}} {
		if err := json.Unmarshal(row.raw, row.tags); err != nil {
			t.Fatal(err)
		}
	}
	want := slices.Clone(oldTags)
	// New groups follow explicit mapping order: s2 -> d3, then s1 -> d2.
	for _, index := range []int{2, 4, 0} {
		tag := sourceTags[index]
		id := "d3"
		if index == 0 {
			id = "d2"
		}
		tag["pageId"] = settingsOrderJSON(t, id)
		want = append(want, tag)
	}
	if len(gotTags) != len(want) {
		t.Fatalf("tag count %d != %d", len(gotTags), len(want))
	}
	for i, expected := range want {
		if len(gotTags[i]) != len(expected) {
			t.Fatalf("tag %d properties changed: %s", i, got["pageTags"])
		}
		for key, value := range expected {
			if !bytes.Equal(value, gotTags[i][key]) {
				t.Errorf("tag %d %s order/raw token changed: %s != %s", i, key, gotTags[i][key], value)
			}
		}
	}
	for key, value := range old {
		if key != "pageTags" && !bytes.Equal(value, got[key]) {
			t.Errorf("unrelated content %s changed", key)
		}
	}
}

func settingsOrderFields(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func settingsOrderJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
