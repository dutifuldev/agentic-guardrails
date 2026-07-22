package rules

import (
	"context"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoCRAPRuleRequiresMetricThreshold(t *testing.T) {
	tests := []struct {
		name      string
		workflow  string
		crapCheck string
	}{
		{
			name:      "missing threshold",
			crapCheck: "go run github.com/unclebob/crap4go/cmd/crap4go@latest\n",
		},
		{
			name:      "report redirection",
			crapCheck: "go run github.com/unclebob/crap4go/cmd/crap4go@latest > crap-report.txt\n",
		},
		{
			name: "threshold in different workflow step",
			workflow: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.0 run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
      - run: crap4go .
      - run: echo "minimum coverage >= 80"
`,
			crapCheck: "go run github.com/unclebob/crap4go/cmd/crap4go@latest\n",
		},
		{
			name: "unrelated coverage threshold",
			crapCheck: `go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
minimum_coverage="80"
awk -v total="90" -v minimum="$minimum_coverage" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
go run github.com/unclebob/crap4go/cmd/crap4go@latest
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]repo.File{
				"go/scripts/check-crap.sh": {Path: "go/scripts/check-crap.sh", Content: tt.crapCheck},
			}
			if tt.workflow != "" {
				overrides[".github/workflows/ci.yml"] = repo.File{Path: ".github/workflows/ci.yml", Content: tt.workflow}
			}
			files := cleanGoGuardrailFiles(overrides)
			report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

			assertRuleIDs(t, report.Findings, []string{GoCRAPRequiredRuleID})
		})
	}
}

func TestGoToolRulesAcceptSlophammerGoCommands(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  coverage:
    threshold: 85
  crap:
    max_score: 8
  targets:
    - .
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
      - run: go run ./cmd/slophammer go dry ..
      - run: go run ./cmd/slophammer go crap ..
      - run: go run ./cmd/slophammer go mutate ..
      - run: go run ./cmd/slophammer-go check ..
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoToolRulesAcceptReleasedSlophammerGoCommands(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
  dry:
    max_findings: 0
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
      - run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.1 dry .. --max-candidates 0
      - run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.1 crap ..
      - run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.1 mutate ..
      - run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.1 check ..
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoToolRulesAcceptConfigBackedSlophammerGoCheckExecute(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
  dry:
    max_findings: 0
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.5 check .. --execute
      - run: ./scripts/check-go-coverage.sh
        working-directory: go
      - run: ./scripts/check-dry.sh
        working-directory: go
      - run: ./scripts/check-crap.sh
        working-directory: go
      - run: ./scripts/check-mutation.sh
        working-directory: go
`,
		},
		"go/scripts/check-dry.sh":      {Path: "go/scripts/check-dry.sh"},
		"go/scripts/check-crap.sh":     {Path: "go/scripts/check-crap.sh"},
		"go/scripts/check-mutation.sh": {Path: "go/scripts/check-mutation.sh"},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCRAPRuleAcceptsConfigBackedCommandWithCoverageProfileBeforePath(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
`,
		},
		"go/scripts/check-crap.sh": {
			Path:    "go/scripts/check-crap.sh",
			Content: "slophammer-go crap --coverage-profile coverage.out .\n",
		},
	})
	snapshot := repo.NewSnapshot("/repo", files)

	if !hasCRAP4GoGate(snapshot) {
		t.Fatal("hasCRAP4GoGate = false, want true with coverage profile before root path")
	}
}

func TestGoMutationRuleRequiresTargetForSlophammerCommand(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"main.go": {Path: "main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go run ./cmd/slophammer go mutate .
      - run: ./scripts/check-go-coverage.sh
        working-directory: go
      - run: ./scripts/check-dry.sh
        working-directory: go
      - run: ./scripts/check-crap.sh
        working-directory: go
      - run: ./scripts/check-mutation.sh
        working-directory: go
`,
		},
	})

	if hasMutate4GoCommand(snapshot) {
		t.Fatal("hasMutate4GoCommand = true, want false without --target or config-root path")
	}
}

func TestGoMutationRuleRejectsRepoRootConfigPathFromNestedWorkingDirectory(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":  {Path: "go/go.mod"},
		"go/main.go": {Path: "go/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - go
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: slophammer-go mutate .
      - run: ./scripts/check-go-coverage.sh
        working-directory: go
      - run: ./scripts/check-dry.sh
        working-directory: go
      - run: ./scripts/check-crap.sh
        working-directory: go
      - run: ./scripts/check-mutation.sh
        working-directory: go
`,
		},
	})

	if hasMutate4GoCommand(snapshot) {
		t.Fatal("hasMutate4GoCommand = true, want false when . is relative to workflow working directory")
	}
}

func TestGoToolRulesAcceptConfigBackedSlophammerCommands(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/main.go": {Path: "go/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
`,
		},
		"scripts/check-crap.sh": {
			Path:    "scripts/check-crap.sh",
			Content: "cd go && go run ./cmd/slophammer go crap ..\n",
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "cd go && go run ./cmd/slophammer go mutate ..\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
	})

	if !hasCRAP4GoGate(snapshot) {
		t.Fatal("hasCRAP4GoGate = false, want true with configured threshold")
	}
	if !hasMutate4GoCommand(snapshot) {
		t.Fatal("hasMutate4GoCommand = false, want true with configured target")
	}
}

func TestGoMutationRuleResolvesRepoRootTargetsForNestedModule(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - go
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: go run ./cmd/slophammer go mutate ..
      - run: go run ./cmd/slophammer-go check ..
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoMutationRuleAcceptsRootConfigCommandForMultipleModules(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"a/go.mod":  {Path: "a/go.mod"},
		"a/main.go": {Path: "a/main.go"},
		"b/go.mod":  {Path: "b/go.mod"},
		"b/main.go": {Path: "b/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "slophammer-go mutate .\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)

	for _, root := range roots {
		scoped := goProjectSnapshot(snapshot, root, roots)
		if !hasMutate4GoCommandForRoot(snapshot, scoped, root, roots) {
			t.Fatalf("hasMutate4GoCommandForRoot(%q) = false, want true", root)
		}
	}
}

func TestGoMutationRuleSkipsModulesOutsideConfiguredTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"a/go.mod":  {Path: "a/go.mod"},
		"a/main.go": {Path: "a/main.go"},
		"b/go.mod":  {Path: "b/go.mod"},
		"b/main.go": {Path: "b/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - a
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "slophammer-go mutate .\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)

	for _, root := range roots {
		scoped := goProjectSnapshot(snapshot, root, roots)
		if !hasMutate4GoCommandForRoot(snapshot, scoped, root, roots) {
			t.Fatalf("hasMutate4GoCommandForRoot(%q) = false, want true", root)
		}
	}
}

func TestGoMutationRuleRejectsUnrelatedModuleScriptForRootConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"a/go.mod":  {Path: "a/go.mod"},
		"a/main.go": {Path: "a/main.go"},
		"b/go.mod":  {Path: "b/go.mod"},
		"b/main.go": {Path: "b/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - a
`,
		},
		"b/scripts/check-mutation.sh": {
			Path:    "b/scripts/check-mutation.sh",
			Content: "slophammer-go mutate\n",
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "a", roots)

	if hasMutate4GoCommandForRoot(snapshot, scoped, "a", roots) {
		t.Fatal("hasMutate4GoCommandForRoot(a) = true, want false")
	}
}

func TestGoMutationRuleDoesNotAcceptUnrelatedModuleLocalConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"a/go.mod":  {Path: "a/go.mod"},
		"a/main.go": {Path: "a/main.go"},
		"a/slophammer.yml": {
			Path: "a/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"b/go.mod":  {Path: "b/go.mod"},
		"b/main.go": {Path: "b/main.go"},
		"b/slophammer.yml": {
			Path: "b/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "cd a && slophammer-go mutate\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)

	aScoped := goProjectSnapshot(snapshot, "a", roots)
	if !hasMutate4GoCommandForRoot(snapshot, aScoped, "a", roots) {
		t.Fatal("hasMutate4GoCommandForRoot(a) = false, want true")
	}
	bScoped := goProjectSnapshot(snapshot, "b", roots)
	if hasMutate4GoCommandForRoot(snapshot, bScoped, "b", roots) {
		t.Fatal("hasMutate4GoCommandForRoot(b) = true, want false")
	}
}

func TestGoMutationRuleDoesNotUseRootScopeToBypassLocalConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - b
`,
		},
		"a/go.mod":  {Path: "a/go.mod"},
		"a/main.go": {Path: "a/main.go"},
		"a/slophammer.yml": {
			Path: "a/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"b/go.mod":  {Path: "b/go.mod"},
		"b/main.go": {Path: "b/main.go"},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: slophammer-go mutate .
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "a", roots)

	if hasMutate4GoCommandForRoot(snapshot, scoped, "a", roots) {
		t.Fatal("hasMutate4GoCommandForRoot(a) = true, want false")
	}
}

func TestGoMutationRuleRejectsRepoRootCommandForLocalConfigWithoutModuleRoot(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":  {Path: "go/go.mod"},
		"go/main.go": {Path: "go/main.go"},
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: go test ./go/... && slophammer-go mutate
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot(go) = true, want false")
	}
}

func TestGoMutationRuleUsesSingleModuleFallbackForRootConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":              {Path: "go/go.mod"},
		"go/internal/example.go": {Path: "go/internal/example.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - internal
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "slophammer-go mutate .\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if !hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot = false, want true")
	}
}

func TestGoMutationRuleAllowsRootCoverageWithUnrelatedLocalConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":  {Path: "go/go.mod"},
		"go/main.go": {Path: "go/main.go"},
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `rules:
  repo.readme-required:
    severity: warn
`,
		},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "slophammer-go mutate .\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if !hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot = false, want true")
	}
}

func TestGoMutationRuleResolvesModuleLocalConfigTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
`,
		},
		"go/go.mod":              {Path: "go/go.mod"},
		"go/internal/example.go": {Path: "go/internal/example.go"},
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - internal
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "cd go && slophammer-go mutate\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if !hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot = false, want true")
	}
}

func TestGoMutationRuleAcceptsRepoRootCommandForModuleLocalConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":  {Path: "go/go.mod"},
		"go/main.go": {Path: "go/main.go"},
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "slophammer-go mutate go\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if !hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot = false, want true")
	}
}

func TestGoMutationRuleRejectsWorkflowWorkingDirDoubleModulePath(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":  {Path: "go/go.mod"},
		"go/main.go": {Path: "go/main.go"},
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: slophammer-go mutate go
      - run: ./scripts/check-go-coverage.sh
        working-directory: go
      - run: ./scripts/check-dry.sh
        working-directory: go
      - run: ./scripts/check-crap.sh
        working-directory: go
      - run: ./scripts/check-mutation.sh
        working-directory: go
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot = true, want false")
	}
}

func TestGoMutationRuleAcceptsCdCommandForModuleLocalConfig(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":  {Path: "go/go.mod"},
		"go/main.go": {Path: "go/main.go"},
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - .
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "cd go && slophammer-go mutate\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-mutation.sh
`,
		},
	})
	roots := goProjectRoots(snapshot)
	scoped := goProjectSnapshot(snapshot, "go", roots)

	if !hasMutate4GoCommandForRoot(snapshot, scoped, "go", roots) {
		t.Fatal("hasMutate4GoCommandForRoot = false, want true")
	}
}

func TestGoToolRulesRejectConfigBackedNonRootParentPath(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/main.go": {Path: "go/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go run ./cmd/slophammer go crap ../tmp
      - run: go run ./cmd/slophammer go mutate ../tmp
      - run: ./scripts/check-go-coverage.sh
        working-directory: go
      - run: ./scripts/check-dry.sh
        working-directory: go
      - run: ./scripts/check-crap.sh
        working-directory: go
      - run: ./scripts/check-mutation.sh
        working-directory: go
`,
		},
	})

	if hasCRAP4GoGate(snapshot) {
		t.Fatal("hasCRAP4GoGate = true, want false for non-root parent path")
	}
	if hasMutate4GoCommand(snapshot) {
		t.Fatal("hasMutate4GoCommand = true, want false for non-root parent path")
	}
}

func TestPackagePathTokenAloneIsNotMutationEvidence(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":                   {Path: "go.mod", Content: "module example.com/demo\n"},
		".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: "name: CI\non: [push]\njobs:\n  t:\n    steps:\n      - run: ./scripts/gate.sh\n"},
		"scripts/gate.sh": {
			Path:    "scripts/gate.sh",
			Content: "echo github.com/unclebob/mutate4go/cmd/mutate4go@latest main.go\n",
		},
	})
	if hasMutate4GoCommand(snapshot) {
		t.Fatal("a package path mentioned outside go run must not be mutation evidence")
	}
}

func TestArgumentPositionSlophammerGoIsNotMutationEvidence(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":                   {Path: "go.mod", Content: "module example.com/demo\n"},
		".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: "name: CI\non: [push]\njobs:\n  t:\n    steps:\n      - run: ./scripts/gate.sh\n"},
		"scripts/gate.sh": {
			Path:    "scripts/gate.sh",
			Content: "echo slophammer-go mutate . --target main.go\n",
		},
	})
	if hasMutate4GoCommand(snapshot) {
		t.Fatal("slophammer-go quoted as an argument must not be mutation evidence")
	}
}

// Every rejection path returns argsStart 0 alongside ok false; callers
// rely on ok, but the zero is the documented contract and pins the
// constants against mutation.
func TestSlophammerGoCommandArgsStartRejectionsReturnZero(t *testing.T) {
	cases := []struct {
		name         string
		tokens       []string
		commandIndex int
	}{
		{"not a slophammer token", []string{"echo", "hi"}, 0},
		{"argument position", []string{"echo", "slophammer-go", "mutate", "."}, 1},
		{"wrong subcommand", []string{"slophammer-go", "crap", "."}, 0},
		{"scan flag", []string{"slophammer-go", "mutate", ".", "--target", "main.go", "--scan"}, 0},
	}
	for _, testCase := range cases {
		argsStart, ok := slophammerGoCommandArgsStart(testCase.tokens, testCase.commandIndex, "mutate")
		if ok || argsStart != 0 {
			t.Fatalf("%s: argsStart, ok = %d, %v; want 0, false", testCase.name, argsStart, ok)
		}
	}
}

func TestScanRejectionScopesToTheMatchedCommand(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":                   {Path: "go.mod", Content: "module example.com/demo\n"},
		".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: "name: CI\non: [push]\njobs:\n  t:\n    steps:\n      - run: ./scripts/gate.sh\n"},
		"scripts/gate.sh": {
			Path:    "scripts/gate.sh",
			Content: "slophammer-go mutate . --target main.go && echo --scan\n",
		},
	})
	if !hasMutate4GoCommand(snapshot) {
		t.Fatal("a later command's --scan token must not discredit the executing gate")
	}
}

func TestWorkingDirectoryScanOnlyMutationIsNotEvidence(t *testing.T) {
	content := "cd go && go run ./cmd/slophammer go mutate . --scan"
	if contentHasConfigBackedSlophammerGoCommandInWorkingDir(content, "mutate", "go") {
		t.Fatal("scan-only mutation must not satisfy the working-directory matcher")
	}
	executing := "cd go && go run ./cmd/slophammer go mutate ."
	if !contentHasConfigBackedSlophammerGoCommandInWorkingDir(executing, "mutate", "go") {
		t.Fatal("executing mutation must satisfy the working-directory matcher")
	}
}

func TestScanOnlyMutationCommandsAreNotEvidence(t *testing.T) {
	contents := []string{
		"slophammer-go mutate . --target main.go --scan\n",
		"slophammer-go mutate . --target main.go --scan=true\n",
		"go run ./cmd/slophammer go mutate . --target main.go --scan\n",
		"go run github.com/unclebob/mutate4go/cmd/mutate4go@latest main.go --scan\n",
		"go run github.com/unclebob/mutate4go/cmd/mutate4go@latest main.go --scan=true\n",
		"go run github.com/unclebob/mutate4go/cmd/mutate4go@latest main.go --update-manifest\n",
		"go run github.com/unclebob/mutate4go/cmd/mutate4go@latest main.go --update-manifest=true\n",
		"slophammer-go mutate . --target main.go --update-manifest\n",
		"slophammer-go mutate . --target main.go --scan=false\n",
		"slophammer-go mutate . --target main.go --update-manifest=false\n",
	}
	for _, content := range contents {
		snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
			"go.mod":                   {Path: "go.mod", Content: "module example.com/demo\n"},
			".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: "name: CI\non: [push]\njobs:\n  t:\n    steps:\n      - run: ./scripts/gate.sh\n"},
			"scripts/gate.sh":          {Path: "scripts/gate.sh", Content: content},
		})
		if hasMutate4GoCommand(snapshot) {
			t.Fatalf("scan-only command credited as mutation evidence: %q", content)
		}
	}
}

func TestGoToolRulesRequireConfigRootForDeepWorkingDirectory(t *testing.T) {
	files := map[string]repo.File{
		"services/api/main.go": {Path: "services/api/main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
`,
		},
	}
	for _, tt := range []struct {
		name string
		path string
		want bool
	}{
		{name: "one parent", path: ".."},
		{name: "repo root", path: "../..", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshotFiles := map[string]repo.File{}
			for path, file := range files {
				snapshotFiles[path] = file
			}
			snapshotFiles["scripts/check-tools.sh"] = repo.File{
				Path: "scripts/check-tools.sh",
				Content: "cd services/api && go run ./cmd/slophammer go crap " + tt.path + "\n" +
					"cd services/api && go run ./cmd/slophammer go mutate " + tt.path + "\n",
			}
			snapshotFiles[".github/workflows/ci.yml"] = repo.File{
				Path: ".github/workflows/ci.yml",
				Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-tools.sh
`,
			}
			snapshot := repo.NewSnapshot("/repo", snapshotFiles)

			if got := hasCRAP4GoGate(snapshot); got != tt.want {
				t.Fatalf("hasCRAP4GoGate = %t, want %t", got, tt.want)
			}
			if got := hasMutate4GoCommand(snapshot); got != tt.want {
				t.Fatalf("hasMutate4GoCommand = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestConfigBackedMutationCommandWorkflowWorkingDir(t *testing.T) {
	tests := []struct {
		name string
		root string
		run  string
		want bool
	}{
		{name: "default module command", root: "go", run: "slophammer-go mutate", want: true},
		{name: "dot module command", root: "go", run: "slophammer-go mutate .", want: true},
		{name: "double module path", root: "go", run: "slophammer-go mutate go"},
		{name: "different module", root: "services/api", run: "slophammer-go mutate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Scoped step fragments, as produced by per-module workflow
			// scoping, keep their raw step text including the
			// working-directory context lines.
			snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
				".github/workflows/ci.yml": {
					Path: ".github/workflows/ci.yml",
					Content: `      - run: ` + tt.run + `
        working-directory: go` + workflowStepBoundary + `      - run: go test ./...
        working-directory: go
`,
				},
			})

			got := hasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir(snapshot, tt.root)
			if got != tt.want {
				t.Fatalf("hasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRepoRootConfiguredGoMutationScopeExcludesRoot(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]repo.File
		root  string
		roots []string
		want  bool
	}{
		{
			name: "targeted root included",
			files: map[string]repo.File{
				"a/go.mod":  {Path: "a/go.mod"},
				"a/main.go": {Path: "a/main.go"},
				"slophammer.yml": {Path: "slophammer.yml", Content: `go:
  targets:
    - a
`},
			},
			root:  "a",
			roots: []string{"a", "b"},
		},
		{
			name: "other root excluded",
			files: map[string]repo.File{
				"a/go.mod":  {Path: "a/go.mod"},
				"a/main.go": {Path: "a/main.go"},
				"b/go.mod":  {Path: "b/go.mod"},
				"b/main.go": {Path: "b/main.go"},
				"slophammer.yml": {Path: "slophammer.yml", Content: `go:
  targets:
    - a
`},
			},
			root:  "b",
			roots: []string{"a", "b"},
			want:  true,
		},
		{
			name: "nested config cannot exclude repo root",
			files: map[string]repo.File{
				"go/slophammer.yml": {Path: "go/slophammer.yml", Content: `go:
  targets:
    - .
`},
				"go/main.go": {Path: "go/main.go"},
			},
			root:  "go",
			roots: []string{"go"},
		},
		{
			name: "empty scope",
			files: map[string]repo.File{
				"slophammer.yml": {Path: "slophammer.yml", Content: "go:\n  crap:\n    max_score: 8\n"},
			},
			root:  "go",
			roots: []string{"go"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoRootConfiguredGoMutationScopeExcludesRoot(repo.NewSnapshot("/repo", tt.files), tt.root, tt.roots)
			if got != tt.want {
				t.Fatalf("repoRootConfiguredGoMutationScopeExcludesRoot = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestGoToolRulesAcceptConfigBackedRootSlophammerCommands(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"main.go": {Path: "main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
`,
		},
		"scripts/check-crap.sh": {
			Path:    "scripts/check-crap.sh",
			Content: "go run ./cmd/slophammer go crap .\n",
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "go run ./cmd/slophammer go mutate .\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
	})

	if !hasCRAP4GoGate(snapshot) {
		t.Fatal("hasCRAP4GoGate = false, want true with root config path")
	}
	if !hasMutate4GoCommand(snapshot) {
		t.Fatal("hasMutate4GoCommand = false, want true with root config path")
	}
}

func TestGoToolRulesAcceptConfigBackedRootSlophammerCommandsWithDefaultPath(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"main.go": {Path: "main.go"},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  crap:
    max_score: 8
  targets:
    - .
`,
		},
		"scripts/check-crap.sh": {
			Path:    "scripts/check-crap.sh",
			Content: "go run ./cmd/slophammer go crap\n",
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "go run ./cmd/slophammer go mutate\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
	})

	if !hasCRAP4GoGate(snapshot) {
		t.Fatal("hasCRAP4GoGate = false, want true with default root path")
	}
	if !hasMutate4GoCommand(snapshot) {
		t.Fatal("hasMutate4GoCommand = false, want true with default root path")
	}
}

func TestDirectMutate4GoIsNotAGate(t *testing.T) {
	// mutate4go exits zero even when mutants survive, so even the
	// strongest direct forms are not mutation-testing evidence.
	commands := []string{
		"go run github.com/unclebob/mutate4go/cmd/mutate4go@latest main.go\n",
		"mutate4go main.go\n",
		"go install github.com/unclebob/mutate4go/cmd/mutate4go@latest && mutate4go main.go\n",
	}
	for _, command := range commands {
		snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
			"go.mod":                   {Path: "go.mod", Content: "module example.com/demo\n"},
			".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: "name: CI\non: [push]\njobs:\n  t:\n    steps:\n      - run: ./scripts/gate.sh\n"},
			"scripts/gate.sh":          {Path: "scripts/gate.sh", Content: command},
		})
		if hasMutate4GoCommand(snapshot) {
			t.Fatalf("direct mutate4go credited as a gate: %q", command)
		}
	}
}
