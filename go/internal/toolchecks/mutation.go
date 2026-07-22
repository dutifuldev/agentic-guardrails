// Mutation gating around mutate4go: run the tool per target, fail on
// survivors and uncovered changed sites, and only let the in-source
// manifests move forward when the run earned it.
package toolchecks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/gotools"
)

func CheckMutation(ctx context.Context, options MutationOptions, out io.Writer, errOut io.Writer, runner Runner) int {
	root := defaultRoot(options.Root)
	targets := mutationTargets(options)
	if len(targets) == 0 {
		_, _ = fmt.Fprintln(errOut, "--target is required")
		return 2
	}
	totals := mutationTotals{}
	for _, target := range targets {
		targetTotals, code := runMutationTarget(ctx, root, target, options.Scan, out, errOut, runner)
		if code != 0 {
			return code
		}
		totals.merge(targetTotals)
	}
	if options.Scan {
		return 0
	}
	return totals.gate(errOut)
}

func runMutationTarget(
	ctx context.Context,
	root string,
	target string,
	scan bool,
	out io.Writer,
	errOut io.Writer,
	runner Runner,
) (mutationTotals, int) {
	sources, err := snapshotGoSources(filepath.Join(root, target))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "reading %s before mutation failed: %v\n", target, err)
		return mutationTotals{}, 2
	}
	args := gotools.Mutate4Go.GoRunArgs(gotools.Latest, target)
	if scan {
		args = append(args, "--scan")
	}
	result, runErr := runner.Run(ctx, root, "go", args...)
	writeBytes(out, result.Stdout)
	writeBytes(errOut, result.Stderr)
	totals := mutationTotals{}
	totals.add(result.Stdout)
	if !totals.earnedManifestUpdate(scan, runErr) {
		if restoreErr := restoreSources(sources); restoreErr != nil {
			_, _ = fmt.Fprintf(errOut, "restoring %s after mutation failed: %v\n", target, restoreErr)
			return totals, 2
		}
	}
	if runErr != nil {
		_, _ = fmt.Fprintf(errOut, "mutate4go failed for %s: %v\n", target, runErr)
		return totals, 2
	}
	return totals, 0
}

type sourceFile struct {
	content []byte
	mode    fs.FileMode
}

// Mutation runs rewrite the in-source manifests as they go, even when
// mutants survive. The snapshot lets the gate put files back when the run
// did not earn the update. A missing path is not an error here: mutate4go
// reports missing targets itself.
func snapshotGoSources(path string) (map[string]sourceFile, error) {
	files := map[string]sourceFile{}
	err := filepath.WalkDir(path, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(filePath, ".go") {
			return nil
		}
		return recordSourceFile(files, filePath, entry)
	})
	if errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	return files, err
}

func recordSourceFile(files map[string]sourceFile, filePath string, entry fs.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(filePath) // #nosec G304 -- paths come from filepath.WalkDir rooted at the mutation target.
	if err != nil {
		return err
	}
	files[filePath] = sourceFile{content: content, mode: info.Mode()}
	return nil
}

func restoreSources(files map[string]sourceFile) error {
	for filePath, file := range files {
		current, err := os.ReadFile(filePath) // #nosec G304 -- restores only files recorded by the pre-run snapshot.
		if err == nil && bytes.Equal(current, file.content) {
			continue
		}
		if writeErr := os.WriteFile(filePath, file.content, file.mode); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

type mutationTotals struct {
	changed          int
	survived         int
	uncoveredChanged int
}

func (t *mutationTotals) merge(other mutationTotals) {
	t.changed += other.changed
	t.survived += other.survived
	t.uncoveredChanged += other.uncoveredChanged
}

// Manifests only move forward on a run that proved something: a scan never
// executes mutants, a failing or erroring run must not baseline its
// survivors, and a clean run with no changed sites has nothing new to
// record.
func (t mutationTotals) earnedManifestUpdate(scan bool, runErr error) bool {
	return !scan && runErr == nil &&
		t.changed > 0 && t.survived == 0 && t.uncoveredChanged == 0
}

func (t *mutationTotals) add(output []byte) {
	t.survived += reportedCount(output, "Survived: ")
	changed := reportedCount(output, "Changed mutation sites: ")
	selected := reportedCount(output, "Selected mutation sites: ")
	t.changed += changed
	if changed > selected {
		t.uncoveredChanged += changed - selected
	}
}

// Survivors are new untested behavior in changed functions; changed sites
// without test coverage never get mutated at all, so they would pass
// silently. Both fail the gate.
func (t mutationTotals) gate(errOut io.Writer) int {
	if t.survived > 0 {
		_, _ = fmt.Fprintf(errOut, "%d mutant(s) survived; strengthen the tests or refresh the manifest deliberately\n", t.survived)
		return 1
	}
	if t.uncoveredChanged > 0 {
		_, _ = fmt.Fprintf(errOut, "%d changed mutation site(s) have no test coverage; cover them before relying on the gate\n", t.uncoveredChanged)
		return 1
	}
	return 0
}

func reportedCount(output []byte, prefix string) int {
	total := 0
	for _, line := range strings.Split(string(output), "\n") {
		value, found := strings.CutPrefix(strings.TrimSpace(line), prefix)
		if !found {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			total += count
		}
	}
	return total
}

func mutationTargets(options MutationOptions) []string {
	if options.Target != "" {
		return []string{options.Target}
	}
	targets := make([]string, 0, len(options.Targets))
	for _, target := range options.Targets {
		if target != "" {
			targets = append(targets, target)
		}
	}
	return targets
}
