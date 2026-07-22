package gotargets

import (
	"errors"
	"reflect"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestResolveFileAndDirectoryTargets(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"cmd/server/main.go":              "package main\n",
		"internal/app/app.go":             "package app\n",
		"internal/app/app_test.go":        "package app\n",
		"internal/app/testdata/sample.go": "package testdata\n",
		"internal/fixtures/example.go":    "package fixtures\n",
		"internal/vendor/dependency.go":   "package dependency\n",
	})

	got, err := Resolve(snapshot, Options{
		Targets: []string{"internal", "cmd/server/main.go"},
	})

	assertResolved(t, got, err, []string{
		"cmd/server/main.go",
		"internal/app/app.go",
	})
}

func TestResolveConfiguredExcludes(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"internal/app/app.go":        "package app\n",
		"internal/generated/code.go": "package generated\n",
		"internal/keep/keep.go":      "package keep\n",
	})

	got, err := Resolve(snapshot, Options{
		Targets: []string{"internal"},
		Exclude: []string{"internal/generated/**", "keep.go"},
	})

	assertResolved(t, got, err, []string{"internal/app/app.go"})
}

func TestResolveConfiguredExcludesRelativeToTarget(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"go/main.go":             "package main\n",
		"go/templates/helper.go": "package templates\n",
	})

	got, err := Resolve(snapshot, Options{
		Targets: []string{"go"},
		Exclude: []string{"templates/**"},
	})

	assertResolved(t, got, err, []string{"go/main.go"})
}

func TestResolveSortsAndDeduplicatesTargets(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"internal/b/b.go": "package b\n",
		"internal/a/a.go": "package a\n",
	})

	got, err := Resolve(snapshot, Options{
		Targets: []string{"internal", "internal/a/a.go"},
	})

	assertResolved(t, got, err, []string{
		"internal/a/a.go",
		"internal/b/b.go",
	})
}

func TestResolveWithSingleModuleFallbackUsesDirectMatch(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"cmd/main.go": "package main\n",
	})

	got, err := ResolveWithSingleModuleFallback(snapshot, Options{
		Targets: []string{"cmd"},
	}, []string{"go"}, ".")

	assertResolved(t, got, err, []string{"cmd/main.go"})
}

func TestResolveWithSingleModuleFallbackPrefixesTargetsAndExcludes(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"go/internal/example.go":         "package internal\n",
		"go/internal/generated/model.go": "package generated\n",
	})

	got, err := ResolveWithSingleModuleFallback(snapshot, Options{
		Targets: []string{"internal"},
		Exclude: []string{"internal/generated/**"},
	}, []string{"go"}, ".")

	assertResolved(t, got, err, []string{"go/internal/example.go"})
}

func TestResolveWithSingleModuleFallbackKeepsOriginalErrorWithoutSingleModule(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"go/internal/example.go": "package internal\n",
	})

	_, err := ResolveWithSingleModuleFallback(snapshot, Options{
		Targets: []string{"internal"},
	}, []string{"."}, ".")

	if !errors.Is(err, ErrNoFiles) {
		t.Fatalf("err = %v, want ErrNoFiles", err)
	}
}

func TestResolveRejectsMissingTargets(t *testing.T) {
	_, err := Resolve(targetSnapshot(nil), Options{})

	if !errors.Is(err, ErrNoTargets) {
		t.Fatalf("err = %v, want ErrNoTargets", err)
	}
}

func TestResolveRejectsZeroProductionFiles(t *testing.T) {
	snapshot := targetSnapshot(map[string]string{
		"internal/app/app_test.go": "package app\n",
	})

	_, err := Resolve(snapshot, Options{Targets: []string{"internal"}})

	if !errors.Is(err, ErrNoFiles) {
		t.Fatalf("err = %v, want ErrNoFiles", err)
	}
}

func targetSnapshot(files map[string]string) repo.Snapshot {
	repoFiles := map[string]repo.File{}
	for path, content := range files {
		repoFiles[path] = repo.File{Path: path, Content: content}
	}
	return repo.NewSnapshot("/repo", repoFiles)
}

func assertResolved(t *testing.T, got []string, err error, want []string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve = %#v, want %#v", got, want)
	}
}
