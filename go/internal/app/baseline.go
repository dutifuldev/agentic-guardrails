package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/rules"
)

const baselineFileName = "slophammer-baseline.json"

// BaselineMode selects how check treats the checked-in baseline file.
type BaselineMode int

const (
	BaselineOff BaselineMode = iota
	BaselineCheck
	BaselineWrite
)

type baselineEntry struct {
	RuleID string `json:"rule_id"`
	Path   string `json:"path"`
}

type baselineFile struct {
	Version  int             `json:"version"`
	Findings []baselineEntry `json:"findings"`
}

// applyBaselineCheck applies a checked-in baseline to a report: matched
// findings are marked baselined and stop affecting OK; stale entries are an
// error so the ratchet can only shrink. Matching is on rule_id plus path,
// never message.
func applyBaselineCheck(root string, report *rules.Report) error {
	baseline, err := readBaselineFile(root)
	if err != nil {
		return err
	}
	unmatched := entryDifference(baseline, nil)
	ok := true
	for i := range report.Findings {
		entry := entryOf(report.Findings[i])
		if _, matched := baseline[entry]; matched {
			report.Findings[i].Baselined = true
			delete(unmatched, entry)
		} else {
			ok = false
		}
	}
	if len(unmatched) > 0 {
		return fmt.Errorf("baseline contains resolved findings; rewrite it: %s", joinedEntries(unmatched))
	}
	report.OK = ok
	return nil
}

// writeBaselineFile records current findings as the baseline. It refuses to
// write a superset of an existing baseline and reports the added and removed
// entries, so debt is recorded once, reviewed, and only ever reduced.
func writeBaselineFile(root string, report rules.Report) (string, error) {
	current := map[baselineEntry]struct{}{}
	for _, finding := range report.Findings {
		current[entryOf(finding)] = struct{}{}
	}
	previous, err := readBaselineFile(root)
	if err != nil && !errors.Is(err, errBaselineMissing) {
		return "", err
	}
	added := entryDifference(current, previous)
	removed := entryDifference(previous, current)
	if len(previous) > 0 && len(added) > 0 && len(removed) == 0 {
		return "", fmt.Errorf("baseline write would grow the baseline; fix the new findings instead: %s", joinedEntries(added))
	}
	if err := writeBaselineEntries(root, current); err != nil {
		return "", err
	}
	return baselineWriteSummary(len(current), added, removed), nil
}

func writeBaselineEntries(root string, entries map[baselineEntry]struct{}) error {
	serialized, err := json.MarshalIndent(baselineFile{Version: 1, Findings: sortedBaselineEntries(entries)}, "", "  ")
	if err != nil {
		return fmt.Errorf("baseline write failed: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, baselineFileName), append(serialized, '\n'), 0o600); err != nil {
		return fmt.Errorf("baseline write failed: %w", err)
	}
	return nil
}

func baselineDebtLine(report rules.Report) string {
	baselined := 0
	for _, finding := range report.Findings {
		if finding.Baselined {
			baselined++
		}
	}
	return fmt.Sprintf("%d findings baselined; %d new\n", baselined, len(report.Findings)-baselined)
}

// errBaselineMissing marks the only read failure that counts as the
// initial-write case; a present but unreadable or malformed baseline must
// never be silently replaced.
var errBaselineMissing = errors.New("baseline file " + baselineFileName + " is missing")

func readBaselineFile(root string) (map[baselineEntry]struct{}, error) {
	// #nosec G304 -- the baseline lives at a fixed name under the check root.
	content, err := os.ReadFile(filepath.Join(root, baselineFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errBaselineMissing
	}
	if err != nil {
		return nil, fmt.Errorf("baseline read failed: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var file baselineFile
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("baseline parse failed: %w", err)
	}
	if file.Version != 1 {
		return nil, errors.New("baseline version must be 1")
	}
	entries := map[baselineEntry]struct{}{}
	for _, entry := range file.Findings {
		entries[entry] = struct{}{}
	}
	return entries, nil
}

func entryOf(finding rules.Finding) baselineEntry {
	return baselineEntry{RuleID: finding.RuleID, Path: finding.Path}
}

func entryDifference(left map[baselineEntry]struct{}, right map[baselineEntry]struct{}) map[baselineEntry]struct{} {
	diff := map[baselineEntry]struct{}{}
	for entry := range left {
		if _, ok := right[entry]; !ok {
			diff[entry] = struct{}{}
		}
	}
	return diff
}

func sortedBaselineEntries(entries map[baselineEntry]struct{}) []baselineEntry {
	sorted := make([]baselineEntry, 0, len(entries))
	for entry := range entries {
		sorted = append(sorted, entry)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RuleID == sorted[j].RuleID {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].RuleID < sorted[j].RuleID
	})
	return sorted
}

func joinedEntries(entries map[baselineEntry]struct{}) string {
	joined := make([]string, 0, len(entries))
	for _, entry := range sortedBaselineEntries(entries) {
		joined = append(joined, entry.RuleID+" at "+entry.Path)
	}
	return strings.Join(joined, ", ")
}

func baselineWriteSummary(total int, added map[baselineEntry]struct{}, removed map[baselineEntry]struct{}) string {
	var summary strings.Builder
	_, _ = fmt.Fprintf(&summary, "baseline written: %d finding(s)\n", total)
	for _, entry := range sortedBaselineEntries(added) {
		_, _ = fmt.Fprintf(&summary, "added: %s at %s\n", entry.RuleID, entry.Path)
	}
	for _, entry := range sortedBaselineEntries(removed) {
		_, _ = fmt.Fprintf(&summary, "removed: %s at %s\n", entry.RuleID, entry.Path)
	}
	return summary.String()
}
