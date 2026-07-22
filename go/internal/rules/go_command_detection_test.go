package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/gotools"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoTestsRuleAcceptsFlagsBeforePackagePattern(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		".github/workflows/ci.yml": {
			Path:    ".github/workflows/ci.yml",
			Content: strings.ReplaceAll(goCleanWorkflow, "go test ./...", "go test -race -count=1 ./..."),
		},
		"go/scripts/check-go-coverage.sh": {
			Path:    "go/scripts/check-go-coverage.sh",
			Content: strings.ReplaceAll(cleanCoverageScript, "go test -coverprofile=coverage.out ./...", "go test -coverprofile=coverage.out ./internal/..."),
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCommandRulesAcceptShellContinuations(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
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
      - run: |
          go test \
            -race \
            ./...
      - run: |
          go vet \
            ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
		"go/scripts/check-go-coverage.sh": {
			Path:    "go/scripts/check-go-coverage.sh",
			Content: strings.ReplaceAll(cleanCoverageScript, "go test -coverprofile=coverage.out ./...", "go test -coverprofile=coverage.out ./internal/..."),
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCommandRulesIgnoreCommentedCommands(t *testing.T) {
	tests := []struct {
		name      string
		workflow  string
		overrides map[string]repo.File
		want      string
	}{
		{
			name: "commented go test",
			workflow: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: "# go test ./..."
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
			overrides: map[string]repo.File{
				"go/scripts/check-go-coverage.sh": {
					Path:    "go/scripts/check-go-coverage.sh",
					Content: strings.ReplaceAll(cleanCoverageScript, "go test -coverprofile=coverage.out ./...", "go test -coverprofile=coverage.out ./internal/..."),
				},
			},
			want: GoTestsRequiredRuleID,
		},
		{
			name: "commented go vet",
			workflow: `name: CI
on: [push]
defaults:
  run:
    working-directory: go
jobs:
  test:
    steps:
      - run: go test ./...
      - run: "# go vet ./..."
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
			want: GoVetRequiredRuleID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]repo.File{
				".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: tt.workflow},
			}
			for path, file := range tt.overrides {
				overrides[path] = file
			}
			files := cleanGoGuardrailFiles(overrides)

			report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

			assertRuleIDs(t, report.Findings, []string{tt.want})
		})
	}
}

func TestGoRulesIgnoreNonGoCommandSubstrings(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md": {Path: "README.md"},
		"AGENTS.md": {Path: "AGENTS.md"},
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
      - run: cargo test
      - run: python manage.py django test
      - run: echo "go test ./..."
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCommandRulesAcceptEnvPrefixedCommands(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
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
      - run: CGO_ENABLED=0 go test ./...
      - run: GOFLAGS=-mod=readonly go vet ./...
      - run: env GOLANGCI_LINT_CACHE=/tmp golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCommandRulesAcceptGoGlobalFlags(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
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
      - run: go -C . test ./...
      - run: go -C=. vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: strings.NewReplacer(
				"go test -coverprofile=coverage.out ./...",
				"go -C . test -coverprofile=coverage.out ./...",
				"go tool cover -func=coverage.out",
				"go -C . tool cover -func=coverage.out",
			).Replace(cleanCoverageScript),
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCommandRulesIgnoreWorkflowStepMetadata(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - name: go test ./...
        run: echo no tests
      - name: go vet ./...
        run: echo no vet
      - name: golangci-lint run
        run: echo no lint
`,
		},
	})

	checks := []struct {
		name  string
		check func(repo.Snapshot) bool
	}{
		{name: "go test", check: hasGoTestCommand},
		{name: "go vet", check: hasGoVetCommand},
		{name: "golangci-lint", check: hasGolangCICommand},
	}
	for _, check := range checks {
		if check.check(snapshot) {
			t.Fatalf("%s command accepted workflow step metadata", check.name)
		}
	}
}

func TestGoCommandRulesIgnoreEchoedCommands(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
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
      - run: echo "go test ./..."
      - run: echo "go vet ./..."
      - run: echo "golangci-lint run"
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
		"go/scripts/check-go-coverage.sh": {
			Path:    "go/scripts/check-go-coverage.sh",
			Content: strings.ReplaceAll(cleanCoverageScript, "go test -coverprofile=coverage.out ./...", "go test -coverprofile=coverage.out ./internal/..."),
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoLintRequiredRuleID,
		GoTestsRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestGoCommandRulesIgnoreRunnerArguments(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
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
      - run: npm run go test ./...
      - run: npm run go vet ./...
      - run: npm run golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
`,
		},
		"go/scripts/check-go-coverage.sh": {
			Path:    "go/scripts/check-go-coverage.sh",
			Content: strings.ReplaceAll(cleanCoverageScript, "go test -coverprofile=coverage.out ./...", "go test -coverprofile=coverage.out ./internal/..."),
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoLintRequiredRuleID,
		GoTestsRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestGoToolCommandDetectionRequiresRunnableCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		tool    gotools.Tool
		want    bool
	}{
		{
			name:    "dry package run",
			command: "go run github.com/unclebob/dry4go/cmd/dry4go@latest --format json .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry package run with go global flag",
			command: "go -C tools run github.com/unclebob/dry4go/cmd/dry4go@latest --format json .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry package run with flag",
			command: "go run -mod=readonly github.com/unclebob/dry4go/cmd/dry4go@latest --format json .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry package run with flag value",
			command: "go run -mod readonly github.com/unclebob/dry4go/cmd/dry4go@latest --format json .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry install only",
			command: "go install github.com/unclebob/dry4go/cmd/dry4go@latest",
			tool:    gotools.Dry4Go,
		},
		{
			name:    "dry install then run",
			command: "go install github.com/unclebob/dry4go/cmd/dry4go@latest && dry4go .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry after semicolon",
			command: "cd go; dry4go .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry after env assignment",
			command: "DRY_CACHE=/tmp dry4go .",
			tool:    gotools.Dry4Go,
			want:    true,
		},
		{
			name:    "dry echo only",
			command: "echo go run github.com/unclebob/dry4go/cmd/dry4go@latest --format json .",
			tool:    gotools.Dry4Go,
		},
		{
			name:    "crap binary run",
			command: "crap4go .",
			tool:    gotools.CRAP4Go,
			want:    true,
		},
		{
			name:    "crap package run with flag",
			command: "go run -mod=readonly github.com/unclebob/crap4go/cmd/crap4go@latest",
			tool:    gotools.CRAP4Go,
			want:    true,
		},
		{
			name:    "crap echo only",
			command: "echo crap4go",
			tool:    gotools.CRAP4Go,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentHasGoToolCommand(tt.command, tt.tool); got != tt.want {
				t.Fatalf("contentHasGoToolCommand(%q) = %t, want %t", tt.command, got, tt.want)
			}
		})
	}
}

func TestSlophammerGoCommandRequiredFlagsNeedValues(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		subcommand  string
		required    string
		expectMatch bool
	}{
		{name: "public dry command", command: "slophammer-go dry .", subcommand: "dry", expectMatch: true},
		{name: "public crap command", command: "slophammer-go crap . --max-score 8", subcommand: "crap", required: "--max-score", expectMatch: true},
		{name: "crap value", command: "slophammer go crap . --max-score 30", subcommand: "crap", required: "--max-score", expectMatch: true},
		{name: "crap after semicolon", command: "cd go; slophammer go crap . --max-score 30", subcommand: "crap", required: "--max-score", expectMatch: true},
		{name: "crap missing value", command: "slophammer go crap . --max-score", subcommand: "crap", required: "--max-score"},
		{name: "crap flag after separator", command: "slophammer go crap . && echo --max-score 30", subcommand: "crap", required: "--max-score"},
		{name: "public mutate command", command: "slophammer-go mutate . --target main.go", subcommand: "mutate", required: "--target", expectMatch: true},
		{name: "mutate value", command: "slophammer go mutate . --target main.go", subcommand: "mutate", required: "--target", expectMatch: true},
		{name: "mutate after semicolon", command: "cd go; slophammer go mutate . --target main.go", subcommand: "mutate", required: "--target", expectMatch: true},
		{name: "mutate missing value", command: "slophammer go mutate . --target", subcommand: "mutate", required: "--target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentHasSlophammerGoCommand(tt.command, tt.subcommand, tt.required)
			if got != tt.expectMatch {
				t.Fatalf("contentHasSlophammerGoCommand(%q) = %t, want %t", tt.command, got, tt.expectMatch)
			}
		})
	}
}
