package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

// SettingsPageMap associates zero-based source and destination native pages.
type SettingsPageMap struct {
	SourcePage      int `json:"source_page"`
	DestinationPage int `json:"destination_page"`
}

// LoadSettingsMapping accepts one exact-key JSON array, including an empty array
// for a source without page tags. Page bounds are checked against fresh content.
func LoadSettingsMapping(path string) ([]SettingsPageMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading settings mapping: %w", err)
	}
	if err := validateSettingsJSON(data); err != nil {
		return nil, fmt.Errorf("decoding settings mapping: %w", err)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil || rows == nil {
		return nil, fmt.Errorf("settings mapping must be a JSON array")
	}
	mapping := make([]SettingsPageMap, 0, len(rows))
	for i, row := range rows {
		if len(row) != 2 || row["source_page"] == nil || row["destination_page"] == nil {
			return nil, fmt.Errorf("mapping row %d requires exactly source_page and destination_page", i)
		}
		var item SettingsPageMap
		if err := settingsInteger(row["source_page"], &item.SourcePage); err != nil {
			return nil, fmt.Errorf("mapping row %d source_page: %w", i, err)
		}
		if err := settingsInteger(row["destination_page"], &item.DestinationPage); err != nil {
			return nil, fmt.Errorf("mapping row %d destination_page: %w", i, err)
		}
		mapping = append(mapping, item)
	}
	if err := validateSettingsMap(mapping); err != nil {
		return nil, err
	}
	return mapping, nil
}

func settingsInteger(raw json.RawMessage, v *int) error {
	if bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("integer required")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("integer required: %w", err)
	}
	return nil
}

func validateSettingsMap(mapping []SettingsPageMap) error {
	source, destination := make(map[int]bool), make(map[int]bool)
	for i, row := range mapping {
		if row.SourcePage < 0 || row.DestinationPage < 0 || source[row.SourcePage] || destination[row.DestinationPage] {
			return fmt.Errorf("mapping row %d has negative or duplicate source/destination page", i)
		}
		source[row.SourcePage], destination[row.DestinationPage] = true, true
	}
	return nil
}

func validateSettingsJSON(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON contains invalid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := settingsJSONValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("one JSON value required")
	}
	return nil
}

func settingsJSONValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := make(map[string]bool)
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return fmt.Errorf("duplicate or invalid JSON object key %q", key)
			}
			keys[name] = true
		}
		if err := settingsJSONValue(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func settingsObject(data []byte) (map[string]json.RawMessage, error) {
	if err := validateSettingsJSON(data); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("JSON object required")
	}
	return fields, nil
}

func sameSettingsJSON(a, b json.RawMessage) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	decode := func(raw json.RawMessage) (any, error) {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var value any
		err := d.Decode(&value)
		return value, err
	}
	x, err := decode(a)
	if err != nil {
		return false
	}
	y, err := decode(b)
	return err == nil && sameSettingsValue(x, y)
}

func settingsPageIDs(fields map[string]json.RawMessage) ([]string, error) {
	var legacy []string
	if raw := fields["pages"]; raw != nil {
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return nil, fmt.Errorf("invalid pages: %w", err)
		}
	}
	ids := legacy
	if raw := fields["cPages"]; raw != nil {
		pages, err := settingsObject(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid cPages: %w", err)
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(pages["pages"], &rows); err != nil {
			return nil, fmt.Errorf("invalid cPages.pages: %w", err)
		}
		if len(rows) > 0 {
			ids = make([]string, 0, len(rows))
			for i, row := range rows {
				var id string
				if err := json.Unmarshal(row["id"], &id); err != nil {
					return nil, fmt.Errorf("page %d has invalid native ID", i)
				}
				if deleted := row["deleted"]; deleted != nil {
					field, err := settingsObject(deleted)
					if err != nil {
						return nil, err
					}
					var value int
					if err := settingsInteger(field["value"], &value); err != nil || value != 0 {
						return nil, fmt.Errorf("page %d is deleted or incompatible", i)
					}
				}
				ids = append(ids, id)
			}
			if len(legacy) > 0 && !equalPageIDs(ids, legacy) {
				return nil, fmt.Errorf("native page schemas disagree")
			}
		}
	}
	var count int
	if raw := fields["pageCount"]; raw != nil {
		if err := settingsInteger(raw, &count); err != nil || count < 0 {
			return nil, fmt.Errorf("invalid pageCount")
		}
	}
	// Reuse the import's established complete-page/ID validation, with only
	// exact native IDs decoded above; no typed content is marshaled for writes.
	return importPageIDs(&cloud.DocumentContent{Pages: ids, PageCount: count})
}
