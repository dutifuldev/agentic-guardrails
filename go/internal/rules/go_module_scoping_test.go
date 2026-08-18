package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoRulesInspectNestedModuleScripts(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                              {Path: "README.md"},
		"AGENTS.md":                              {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"services/api/go.mod":                    {Path: "services/api/go.mod"},
		"services/api/main.go":                   {Path: "services/api/main.go"},
		"services/api/.golangci.yml":             {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		".github/workflows/ci.yml":               {Path: ".github/workflows/ci.yml", Content: nestedGoWorkflow},
		"services/api/scripts/check-coverage.sh": {Path: "services/api/scripts/check-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":      {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":     {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh": {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("internal/rules/rules.go")},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesIgnoreEmbeddedFixtureEvidence(t *testing.T) {
	files := cleanGoGuardrailFiles(nil)
	delete(files, "go/.golangci.yml")
	delete(files, "go/scripts/check-mutation.sh")
	files["fixtures/repos/go-clean/go/.golangci.yml"] = repo.File{
		Path:    "fixtures/repos/go-clean/go/.golangci.yml",
		Content: "linters:\n  enable:\n    - cyclop\n",
	}
	files["fixtures/repos/go-clean/go/scripts/check-mutation.sh"] = repo.File{
		Path:    "fixtures/repos/go-clean/go/scripts/check-mutation.sh",
		Content: cleanMutationScript("internal/rules/rules.go"),
	}

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoComplexityRequiredRuleID,
		GoLintRequiredRuleID,
		GoMutationRequiredRuleID,
	})
}

func TestGoRulesDoNotTreatEmbeddedFixturesAsTargetProjects(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                         {Path: "README.md"},
		"AGENTS.md":                         {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		".github/workflows/ci.yml":          {Path: ".github/workflows/ci.yml"},
		"fixtures/repos/go-missing/go.mod":  {Path: "fixtures/repos/go-missing/go.mod"},
		"fixtures/repos/go-missing/main.go": {Path: "fixtures/repos/go-missing/main.go"},
		"templates/go/go.mod":               {Path: "templates/go/go.mod"},
		"templates/go/main.go":              {Path: "templates/go/main.go"},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesIgnoreVendoredModules(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		"go/vendor/example.com/dep/go.mod": {Path: "go/vendor/example.com/dep/go.mod"},
		"go/vendor/example.com/dep/dep.go": {Path: "go/vendor/example.com/dep/dep.go"},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesAcceptRootGuardrailsForNestedModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":     {Path: "README.md"},
		"AGENTS.md":     {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"go/go.mod":     {Path: "go/go.mod"},
		"go/main.go":    {Path: "go/main.go"},
		".golangci.yml": {Path: ".golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"Makefile": {
			Path: "Makefile",
			Content: `test:
	cd go && go test ./...
vet:
	cd go && go vet ./...
lint:
	cd go && golangci-lint run
`,
		},
		"scripts/check-go-coverage.sh": {
			Path: "scripts/check-go-coverage.sh",
			Content: `cd go
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
awk -v total="80" -v minimum="80" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
`,
		},
		"scripts/check-dry.sh": {
			Path:    "scripts/check-dry.sh",
			Content: "cd go && go run github.com/unclebob/dry4go/cmd/dry4go@latest --format json .\n",
		},
		"scripts/check-crap.sh": {
			Path: "scripts/check-crap.sh",
			Content: `cd go
maximum_crap_score="30"
go run github.com/unclebob/crap4go/cmd/crap4go@latest
awk -v score="0" -v maximum="$maximum_crap_score" 'BEGIN { exit !(score + 0 <= maximum + 0) }'
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "cd go && slophammer-go mutate . --target main.go\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: make test
        working-directory: go
      - run: make vet
        working-directory: go
      - run: make lint
        working-directory: go
      - run: scripts/check-go-coverage.sh
        working-directory: go
      - run: scripts/check-dry.sh
        working-directory: go
      - run: scripts/check-crap.sh
        working-directory: go
      - run: scripts/check-mutation.sh
        working-directory: go
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesDoNotCarryMakefileScopeAcrossTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":     {Path: "README.md"},
		"AGENTS.md":     {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"go/go.mod":     {Path: "go/go.mod"},
		"go/main.go":    {Path: "go/main.go"},
		".golangci.yml": {Path: ".golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"Makefile": {
			Path: "Makefile",
			Content: `test:
	cd go && go test ./...
vet:
	go vet ./...
lint:
	cd go && golangci-lint run
`,
		},
		"scripts/check-go-coverage.sh": {
			Path: "scripts/check-go-coverage.sh",
			Content: `cd go
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
awk -v total="80" -v minimum="80" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
`,
		},
		"scripts/check-dry.sh": {
			Path:    "scripts/check-dry.sh",
			Content: "cd go && go run github.com/unclebob/dry4go/cmd/dry4go@latest --format json .\n",
		},
		"scripts/check-crap.sh": {
			Path: "scripts/check-crap.sh",
			Content: `cd go
maximum_crap_score="30"
go run github.com/unclebob/crap4go/cmd/crap4go@latest
awk -v score="0" -v maximum="$maximum_crap_score" 'BEGIN { exit !(score + 0 <= maximum + 0) }'
`,
		},
		"scripts/check-mutation.sh": {
			Path:    "scripts/check-mutation.sh",
			Content: "cd go && slophammer-go mutate . --target main.go\n",
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: make test
        working-directory: go
      - run: make vet
        working-directory: go
      - run: make lint
        working-directory: go
      - run: scripts/check-go-coverage.sh
        working-directory: go
      - run: scripts/check-dry.sh
        working-directory: go
      - run: scripts/check-crap.sh
        working-directory: go
      - run: scripts/check-mutation.sh
        working-directory: go
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoVetRequiredRuleID})
}

func TestGoRulesScopeRootCommandFilesPerModule(t *testing.T) {
	coverageScript := strings.ReplaceAll(cleanCoverageScript, "go test -coverprofile=coverage.out ./...", "go test -coverprofile=coverage.out ./internal/...")
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                              {Path: "README.md"},
		"AGENTS.md":                              {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"services/api/go.mod":                    {Path: "services/api/go.mod"},
		"services/api/main.go":                   {Path: "services/api/main.go"},
		"services/api/.golangci.yml":             {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-coverage.sh": {Path: "services/api/scripts/check-coverage.sh", Content: coverageScript},
		"services/api/scripts/check-dry.sh":      {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":     {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh": {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                 {Path: "services/worker/go.mod"},
		"services/worker/main.go":                {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":          {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-coverage.sh": {
			Path:    "services/worker/scripts/check-coverage.sh",
			Content: coverageScript,
		},
		"services/worker/scripts/check-dry.sh": {
			Path:    "services/worker/scripts/check-dry.sh",
			Content: cleanDryScript(),
		},
		"services/worker/scripts/check-crap.sh": {
			Path:    "services/worker/scripts/check-crap.sh",
			Content: cleanCRAPScript(),
		},
		"services/worker/scripts/check-mutation.sh": {
			Path:    "services/worker/scripts/check-mutation.sh",
			Content: cleanMutationScript("main.go"),
		},
		"Makefile": {
			Path: "Makefile",
			Content: `api-test:
	cd services/api && go test ./...
api-lint:
	cd services/api && golangci-lint run
worker-vet:
	cd services/worker && go vet ./...
worker-lint:
	cd services/worker && golangci-lint run
`,
		},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: make api-test
        working-directory: services/api
      - run: make api-lint
        working-directory: services/api
      - run: make worker-vet
        working-directory: services/worker
      - run: make worker-lint
        working-directory: services/worker
      - run: services/api/scripts/check-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
      - run: services/worker/scripts/check-coverage.sh
      - run: services/worker/scripts/check-dry.sh
      - run: services/worker/scripts/check-crap.sh
      - run: services/worker/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoTestsRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestScopedRootCommandContentStopsAfterLeavingModule(t *testing.T) {
	content := `cd go
go test ./...
cd ..
go vet ./...
dry4go .
`

	scoped, ok := scopedRootCommandContent("scripts/check.sh", content, "go", []string{"go"})

	if !ok {
		t.Fatal("scopedRootCommandContent returned ok=false")
	}
	if strings.Contains(scoped, "go vet ./...") || strings.Contains(scoped, "dry4go .") {
		t.Fatalf("scoped content kept commands after leaving module: %q", scoped)
	}
	if !strings.Contains(scoped, "go test ./...") {
		t.Fatalf("scoped content lost in-module command: %q", scoped)
	}
}

func TestGoRulesDoNotTreatGoCommandAsGoModuleRoot(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                        {Path: "README.md"},
		"AGENTS.md":                        {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"go/go.mod":                        {Path: "go/go.mod"},
		"go/main.go":                       {Path: "go/main.go"},
		"go/.golangci.yml":                 {Path: "go/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"go/scripts/check-go-coverage.sh":  {Path: "go/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"go/scripts/check-dry.sh":          {Path: "go/scripts/check-dry.sh", Content: cleanDryScript()},
		"go/scripts/check-crap.sh":         {Path: "go/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"go/scripts/check-mutation.sh":     {Path: "go/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"api/go.mod":                       {Path: "api/go.mod"},
		"api/main.go":                      {Path: "api/main.go"},
		"api/.golangci.yml":                {Path: "api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"api/scripts/check-go-coverage.sh": {Path: "api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"api/scripts/check-dry.sh":         {Path: "api/scripts/check-dry.sh", Content: cleanDryScript()},
		"api/scripts/check-crap.sh":        {Path: "api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"api/scripts/check-mutation.sh":    {Path: "api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    steps:
      - run: cd api && go test ./...
      - run: cd api && go vet ./...
      - run: cd api && golangci-lint run
      - run: api/scripts/check-go-coverage.sh
      - run: api/scripts/check-dry.sh
      - run: api/scripts/check-crap.sh
      - run: api/scripts/check-mutation.sh
  go:
    steps:
      - run: go/scripts/check-go-coverage.sh
      - run: go/scripts/check-dry.sh
      - run: go/scripts/check-crap.sh
      - run: go/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoLintRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestGoRulesDetectRootGoSourceWithNestedModule(t *testing.T) {
	files := map[string]repo.File{
		"README.md":                  {Path: "README.md"},
		"AGENTS.md":                  {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"main.go":                    {Path: "main.go"},
		"services/api/go.mod":        {Path: "services/api/go.mod"},
		"services/api/main.go":       {Path: "services/api/main.go"},
		"services/api/.golangci.yml": {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh": {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":         {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":        {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":    {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml":                  {Path: ".github/workflows/ci.yml", Content: nestedGoWorkflow},
	}

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoComplexityRequiredRuleID,
		GoCoverageRequiredRuleID,
		GoCRAPRequiredRuleID,
		GoDryRequiredRuleID,
		GoLintRequiredRuleID,
		GoModuleRequiredRuleID,
		GoMutationRequiredRuleID,
		GoTestsRequiredRuleID,
		GoVetRequiredRuleID,
	})
}
