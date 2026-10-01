package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

var viewportFields = []string{"zoomMode", "viewBackgroundFilter", "customZoomCenterX", "customZoomCenterY", "customZoomOrientation", "customZoomPageHeight", "customZoomPageWidth", "customZoomScale"}

// ViewportDifference retains presence separately from JSON values: removal has
// different firmware behavior from assigning zero or an empty string.
type ViewportDifference struct {
	Field       string
	Destination json.RawMessage
	Source      json.RawMessage
}

func validateViewport(fields map[string]json.RawMessage) error {
	for _, key := range viewportFields {
		raw, present := fields[key]
		if !present {
			continue
		}
		if bytes.Equal(raw, []byte("null")) {
			return fmt.Errorf("%s requires a value, not null", key)
		}
		switch key {
		case "zoomMode", "viewBackgroundFilter", "customZoomOrientation":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("%s requires a string", key)
			}
			valid := false
			switch key {
			case "zoomMode":
				valid = value == "bestFit" || value == "customFit" || value == "fitToHeight" || value == "fitToWidth"
			case "viewBackgroundFilter":
				valid = value == "off" || value == "fullpage"
			case "customZoomOrientation":
				valid = value == "portrait" || value == "landscape"
			}
			if !valid {
				return fmt.Errorf("unsupported %s value %q", key, value)
			}
		default:
			var number json.Number
			if err := json.Unmarshal(raw, &number); err != nil || len(raw) == 0 || raw[0] == '"' {
				return fmt.Errorf("%s requires a number", key)
			}
			if _, err := strconv.ParseFloat(string(number), 64); err != nil {
				return fmt.Errorf("%s has an unsupported numeric value", key)
			}
		}
	}
	return nil
}

func settingsTags(fields map[string]json.RawMessage, key string, ids []string) ([]map[string]json.RawMessage, error) {
	raw, present := fields[key]
	if !present {
		return nil, nil
	}
	var tags []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &tags); err != nil || tags == nil {
		return nil, fmt.Errorf("%s requires a structured tag array", key)
	}
	validIDs := make(map[string]bool)
	for _, id := range ids {
		validIDs[id] = true
	}
	for i, tag := range tags {
		var name string
		if err := json.Unmarshal(tag["name"], &name); err != nil || name == "" {
			return nil, fmt.Errorf("%s tag %d requires a name", key, i)
		}
		var timestamp json.Number
		raw := tag["timestamp"]
		if err := json.Unmarshal(raw, &timestamp); err != nil || len(raw) == 0 || raw[0] == '"' || bytes.Equal(raw, []byte("null")) {
			return nil, fmt.Errorf("%s tag %d requires a numeric timestamp", key, i)
		}
		if key == "pageTags" {
			var id string
			if err := json.Unmarshal(tag["pageId"], &id); err != nil || !validIDs[id] {
				return nil, fmt.Errorf("pageTags tag %d references an unknown native page", i)
			}
		}
	}
	return tags, nil
}

func mergeSettings(source, destination map[string]json.RawMessage, opts SettingsOptions, result *SettingsResult) ([]byte, error) {
	sourceIDs, err := settingsPageIDs(source)
	if err != nil {
		return nil, fmt.Errorf("source page structure: %w", err)
	}
	destinationIDs, err := settingsPageIDs(destination)
	if err != nil {
		return nil, fmt.Errorf("destination page structure: %w", err)
	}
	if err := validateViewport(source); err != nil {
		return nil, fmt.Errorf("source viewport: %w", err)
	}
	if err := validateViewport(destination); err != nil {
		return nil, fmt.Errorf("destination viewport: %w", err)
	}
	pages, err := settingsMappedPages(sourceIDs, destinationIDs, opts.Mapping)
	if err != nil {
		return nil, err
	}
	var conflicts []error
	if err := transferTagFields(source, destination, sourceIDs, destinationIDs, pages); err != nil {
		conflicts = append(conflicts, err)
	}
	for _, key := range viewportFields {
		if sameSettingsJSON(source[key], destination[key]) {
			continue
		}
		result.ViewportDifferences = append(result.ViewportDifferences, ViewportDifference{Field: key, Destination: bytes.Clone(destination[key]), Source: bytes.Clone(source[key])})
		if !opts.ReplaceViewport {
			conflicts = append(conflicts, fmt.Errorf("destination viewport %s differs; review differences and use --replace-viewport to replace it", key))
		}
	}
	if len(conflicts) > 0 {
		return nil, errors.Join(conflicts...)
	}
	for _, key := range viewportFields {
		if raw, ok := source[key]; ok {
			destination[key] = raw
		} else {
			delete(destination, key)
		}
	}
	return json.Marshal(destination)
}

type mappedSettingsPage struct {
	source      string
	destination string
	index       int
}

func settingsMappedPages(sourceIDs, destinationIDs []string, mapping []SettingsPageMap) ([]mappedSettingsPage, error) {
	pages := make([]mappedSettingsPage, 0, len(mapping))
	for _, row := range mapping {
		if row.SourcePage >= len(sourceIDs) || row.DestinationPage >= len(destinationIDs) {
			return nil, fmt.Errorf("mapping %d -> %d is out of range", row.SourcePage, row.DestinationPage)
		}
		pages = append(pages, mappedSettingsPage{source: sourceIDs[row.SourcePage], destination: destinationIDs[row.DestinationPage], index: row.DestinationPage})
	}
	return pages, nil
}

func transferTagFields(source, destination map[string]json.RawMessage, sourceIDs, destinationIDs []string, pages []mappedSettingsPage) error {
	tags, err := settingsTags(source, "tags", sourceIDs)
	if err != nil {
		return err
	}
	dstTags, err := settingsTags(destination, "tags", destinationIDs)
	if err != nil {
		return err
	}
	srcPageTags, err := settingsTags(source, "pageTags", sourceIDs)
	if err != nil {
		return err
	}
	dstPageTags, err := settingsTags(destination, "pageTags", destinationIDs)
	if err != nil {
		return err
	}
	var conflicts []error
	if len(dstTags) > 0 && !sameTagRows(tags, dstTags) {
		conflicts = append(conflicts, fmt.Errorf("destination document tags conflict with source tags"))
	}
	retained, err := remapPageTags(srcPageTags, dstPageTags, pages)
	if err != nil {
		conflicts = append(conflicts, err)
	}
	if len(conflicts) > 0 {
		return errors.Join(conflicts...)
	}
	if len(tags) > 0 || source["tags"] != nil || destination["tags"] != nil {
		if tags == nil {
			tags = make([]map[string]json.RawMessage, 0)
		}
		destination["tags"], err = json.Marshal(tags)
		if err != nil {
			return err
		}
	}
	if len(retained) > 0 || source["pageTags"] != nil || destination["pageTags"] != nil {
		if retained == nil {
			retained = make([]map[string]json.RawMessage, 0)
		}
		destination["pageTags"], err = json.Marshal(retained)
		if err != nil {
			return err
		}
	}
	return nil
}

func remapPageTags(source, destination []map[string]json.RawMessage, pages []mappedSettingsPage) ([]map[string]json.RawMessage, error) {
	pageMap := make(map[string]string)
	for _, page := range pages {
		pageMap[page.source] = page.destination
	}
	remapped := make(map[string][]map[string]json.RawMessage)
	for _, tag := range source {
		var id string
		if err := json.Unmarshal(tag["pageId"], &id); err != nil {
			return nil, err
		}
		dst, ok := pageMap[id]
		if !ok {
			return nil, fmt.Errorf("source tagged native page %s has no mapping", id)
		}
		raw, err := json.Marshal(dst)
		if err != nil {
			return nil, err
		}
		tag["pageId"] = raw
		remapped[dst] = append(remapped[dst], tag)
	}
	mappedIDs := make(map[string]bool)
	for _, dst := range pageMap {
		mappedIDs[dst] = true
	}
	var retained []map[string]json.RawMessage
	existing := make(map[string][]map[string]json.RawMessage)
	for _, tag := range destination {
		var id string
		if err := json.Unmarshal(tag["pageId"], &id); err != nil {
			return nil, err
		}
		if mappedIDs[id] {
			existing[id] = append(existing[id], tag)
		} else {
			retained = append(retained, tag)
		}
	}
	// Use the explicit row order, never map iteration, for deterministic remapping.
	var conflicts []error
	for _, page := range pages {
		id := page.destination
		if len(existing[id]) > 0 && !sameTagRows(existing[id], remapped[id]) {
			conflicts = append(conflicts, fmt.Errorf("destination page %d (%s) tags conflict", page.index, id))
		}
		retained = append(retained, remapped[id]...)
	}
	if len(conflicts) > 0 {
		return nil, errors.Join(conflicts...)
	}
	return retained, nil
}

func sameTagRows(a, b []map[string]json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i, tag := range a {
		if len(tag) != len(b[i]) {
			return false
		}
		for key, value := range tag {
			if !sameSettingsJSON(value, b[i][key]) {
				return false
			}
		}
	}
	return true
}
