package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoRulesPreferModuleLocalConfigOverRepoRootConfig(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		".golangci.yml": {
			Path:    ".golangci.yml",
			Content: "linters:\n  enable:\n    - errcheck\n",
		},
		"go/.golangci.yml": {
			Path:    "go/.golangci.yml",
			Content: "linters:\n  enable:\n    - cyclop\n",
		},
	})

	for range 100 {
		report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())
		if !report.OK {
			t.Fatalf("report.OK = false, findings = %#v", report.Findings)
		}
	}
}

func TestGoRulesDoNotUseRepoRootConfigWhenModuleLocalConfigExists(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		".golangci.yml": {
			Path:    ".golangci.yml",
			Content: "linters:\n  enable:\n    - cyclop\n",
		},
		"go/.golangci.yaml": {
			Path:    "go/.golangci.yaml",
			Content: "linters:\n  enable:\n    - errcheck\n",
		},
	})
	delete(files, "go/.golangci.yml")

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoComplexityRequiredRuleID})
}

func TestGoLintRuleRequiresRunCommand(t *testing.T) {
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
      - run: go test ./...
      - run: go vet ./...
      - run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.0
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

	assertRuleIDs(t, report.Findings, []string{GoLintRequiredRuleID})
}

func TestGoComplexityRuleRequiresEnabledLinter(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name: "disabled",
			config: `linters:
  disable:
    - cyclop
`,
		},
		{
			name: "settings only",
			config: `linters:
  settings:
    cyclop:
      max-complexity: 10
`,
		},
		{
			name: "comment only",
			config: `linters:
  enable:
    - errcheck
# cyclop belongs in enable, not comments.
`,
		},
		{
			name: "enabled and disabled",
			config: `linters:
  enable:
    - cyclop
  disable:
    - cyclop
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := cleanGoGuardrailFiles(map[string]repo.File{
				"go/.golangci.yml": {Path: "go/.golangci.yml", Content: tt.config},
			})

			report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

			assertRuleIDs(t, report.Findings, []string{GoComplexityRequiredRuleID})
		})
	}
}

func TestGoComplexityRuleAcceptsDefaultAll(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/.golangci.yml": {
			Path:    "go/.golangci.yml",
			Content: "linters:\n  default: all\n",
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoComplexityRuleRejectsDefaultAllWhenAllComplexityLintersDisabled(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/.golangci.yml": {
			Path: "go/.golangci.yml",
			Content: `linters:
  default: all
  disable:
    - cyclop
    - gocognit
    - gocyclo
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoComplexityRequiredRuleID})
}
