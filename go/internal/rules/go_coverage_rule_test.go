package rules

import (
	"context"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoCoverageRuleRequiresCoverageOutputAndCoverTool(t *testing.T) {
	tests := []struct {
		name          string
		coverageCheck string
	}{
		{name: "missing cover tool", coverageCheck: "go test -coverprofile=coverage.out ./...\n"},
		{name: "missing cover profile", coverageCheck: "go tool cover -func=coverage.out\n"},
		{name: "missing threshold", coverageCheck: "go test -coverprofile=coverage.out ./...\ngo tool cover -func=coverage.out\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := cleanGoGuardrailFiles(map[string]repo.File{
				"go/scripts/check-go-coverage.sh": {
					Path:    "go/scripts/check-go-coverage.sh",
					Content: tt.coverageCheck,
				},
			})

			report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

			assertRuleIDs(t, report.Findings, []string{GoCoverageRequiredRuleID})
		})
	}
}

func TestGoCoverageRuleAcceptsGoGlobalFlags(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: `minimum_coverage="80"
go -C . test -coverprofile=coverage.out ./...
total="$(go -C . tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
awk -v total="$total" -v minimum="$minimum_coverage" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCoverageRuleAcceptsConfigBackedSlophammerGoCheckExecute(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  coverage:
    threshold: 85
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
      - run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.7 check .. --execute
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
	cfg := config.Config{Go: config.GoConfig{CoverageThreshold: 85}}

	report := RunWithPolicy(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules(), NewPolicy(cfg, nil))

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCoverageRuleRejectsReportRedirectionWithoutThreshold(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: `go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out > coverage.html
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoCoverageRequiredRuleID})
}

func TestGoCoverageRuleRequiresThresholdInSameCheck(t *testing.T) {
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
      - run: go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.0 run
      - run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -html=coverage.out > coverage.html
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
		".github/workflows/node.yml": {
			Path: ".github/workflows/node.yml",
			Content: `name: Node
on: [push]
jobs:
  node:
    steps:
      - run: echo "minimum node version >= 20"
        working-directory: go
`,
		},
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: `go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out > coverage.html
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoCoverageRequiredRuleID})
}

func TestGoCoverageRuleRequiresEvidenceInSameCheck(t *testing.T) {
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
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
      - run: echo "minimum node version >= 20"
`,
		},
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: `go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out > coverage.html
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoCoverageRequiredRuleID})
}

func TestGoCoverageRuleAcceptsWorkflowGateSplitAcrossSteps(t *testing.T) {
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
      - run: golangci-lint run
      - run: go test -coverprofile=coverage.out ./...
      - run: total="$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
      - run: awk -v total="$total" -v minimum_coverage="80" 'BEGIN { exit !(total + 0 >= minimum_coverage + 0) }'
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
		"go/scripts/check-go-coverage.sh": {
			Path:    "go/scripts/check-go-coverage.sh",
			Content: "echo coverage lives in the workflow\n",
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoCoverageRuleRequiresCoverProfileOnRunnableGoTest(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: `echo "go test will add -coverprofile later"
go tool cover -func=coverage.out
awk -v total="100" -v minimum="80" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoCoverageRequiredRuleID})
}

func TestGoCoverageRuleAcceptsSpaceSeparatedCoverProfileFlag(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/scripts/check-go-coverage.sh": {
			Path: "go/scripts/check-go-coverage.sh",
			Content: `go test -coverprofile coverage.out ./...
total="$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
awk -v total="$total" -v minimum="80" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoThresholdPatternsAcceptStrictComparisons(t *testing.T) {
	if !hasCoverageThreshold(`awk -v total="$total" -v minimum="80" 'BEGIN { exit (total + 0 < minimum + 0) }'`) {
		t.Fatal("coverage threshold pattern rejected strict minimum comparison")
	}

	if !hasCRAPThreshold(`awk -v score="0" -v maximum="30" 'BEGIN { exit (score + 0 > maximum + 0) }'`) {
		t.Fatal("CRAP threshold pattern rejected strict maximum comparison")
	}
}

func TestGoCoverageRuleAcceptsCommonThresholdNames(t *testing.T) {
	tests := []struct {
		name          string
		coverageCheck string
	}{
		{
			name: "threshold variable",
			coverageCheck: `go test -coverprofile=coverage.out ./...
total="$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
awk -v total="$total" -v threshold="80" 'BEGIN { exit !(total + 0 >= threshold + 0) }'
`,
		},
		{
			name: "literal threshold",
			coverageCheck: `go test -coverprofile=coverage.out ./...
total="$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
awk -v total="$total" 'BEGIN { exit !(total + 0 >= 80) }'
`,
		},
		{
			name: "strict threshold",
			coverageCheck: `go test -coverprofile=coverage.out ./...
total="$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
awk -v total="$total" -v minimum="80" 'BEGIN { exit (total + 0 < minimum + 0) }'
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := cleanGoGuardrailFiles(map[string]repo.File{
				"go/scripts/check-go-coverage.sh": {
					Path:    "go/scripts/check-go-coverage.sh",
					Content: tt.coverageCheck,
				},
			})

			report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

			if !report.OK {
				t.Fatalf("report.OK = false, findings = %#v", report.Findings)
			}
		})
	}
}
