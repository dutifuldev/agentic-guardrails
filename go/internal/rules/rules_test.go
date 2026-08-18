package rules

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/gotools"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestDefaultRulesPassForMinimalRepo(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                 {Path: "README.md"},
		"AGENTS.md":                 {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		".github/workflows/ci.yaml": {Path: ".github/workflows/ci.yaml"},
	})

	report := Run(context.Background(), snapshot, DefaultRules())
	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestDefaultRulesReportMissingFiles(t *testing.T) {
	report := Run(context.Background(), repo.NewSnapshot("/repo", nil), DefaultRules())

	if report.OK {
		t.Fatal("report.OK = true, want false")
	}
	wantRuleIDs := []string{
		AgentsRequiredRuleID,
		CIRequiredRuleID,
		ReadmeRequiredRuleID,
	}
	if len(report.Findings) != len(wantRuleIDs) {
		t.Fatalf("len(findings) = %d, want %d", len(report.Findings), len(wantRuleIDs))
	}
	for i, want := range wantRuleIDs {
		if report.Findings[i].RuleID != want {
			t.Fatalf("finding[%d].RuleID = %q, want %q", i, report.Findings[i].RuleID, want)
		}
	}
}

func TestDefaultRulesReportMissingGoGuardrails(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md": {Path: "README.md"},
		"AGENTS.md": {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"main.go":   {Path: "main.go"},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	wantRuleIDs := []string{
		GoComplexityRequiredRuleID,
		GoCoverageRequiredRuleID,
		GoCRAPRequiredRuleID,
		GoDryRequiredRuleID,
		GoLintRequiredRuleID,
		GoModuleRequiredRuleID,
		GoMutationRequiredRuleID,
		GoTestsRequiredRuleID,
		GoVetRequiredRuleID,
		CIRequiredRuleID,
	}
	assertRuleIDs(t, report.Findings, wantRuleIDs)
}

func TestDefaultRulesPassForGoRepoWithDeclaredGuardrails(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", cleanGoGuardrailFiles(nil))

	report := Run(context.Background(), snapshot, DefaultRules())
	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestExplainKnownRule(t *testing.T) {
	got, ok := Explain(DefaultRules(), AgentsRequiredRuleID)
	if !ok {
		t.Fatal("Explain returned ok=false")
	}
	if got == "" {
		t.Fatal("Explain returned empty output")
	}
}

func TestExplainUnknownRule(t *testing.T) {
	_, ok := Explain(DefaultRules(), "missing")
	if ok {
		t.Fatal("Explain returned ok=true for missing rule")
	}
}

func TestDefaultDefinitionsAreStable(t *testing.T) {
	definitions := DefaultDefinitions()
	wantIDs := []string{
		ReadmeRequiredRuleID,
		AgentsRequiredRuleID,
		AgentsEmptyRuleID,
		AgentsCommandsRequiredRuleID,
		AgentsCommandInvalidRuleID,
		AgentsScopeRequiredRuleID,
		AgentsStaleRuleID,
		CIRequiredRuleID,
		SlophammerCIRequiredRuleID,
		GoModuleRequiredRuleID,
		GoTestsRequiredRuleID,
		GoVetRequiredRuleID,
		GoLintRequiredRuleID,
		GoCoverageRequiredRuleID,
		GoComplexityRequiredRuleID,
		GoDryRequiredRuleID,
		GoCRAPRequiredRuleID,
		GoMutationRequiredRuleID,
		GoDependencyBoundariesRuleID,
		GoScopeIncompleteRuleID,
		GoSuppressionsJustifiedRuleID,
	}
	if len(definitions) != len(wantIDs) {
		t.Fatalf("len(definitions) = %d, want %d", len(definitions), len(wantIDs))
	}
	for i, want := range wantIDs {
		if definitions[i].ID != want {
			t.Fatalf("definition[%d].ID = %q, want %q", i, definitions[i].ID, want)
		}
		if definitions[i].Severity == "" {
			t.Fatalf("definition[%d].Severity is empty", i)
		}
		if definitions[i].Path == "" {
			t.Fatalf("definition[%d].Path is empty", i)
		}
		if definitions[i].Message == "" {
			t.Fatalf("definition[%d].Message is empty", i)
		}
		if definitions[i].Description == "" {
			t.Fatalf("definition[%d].Description is empty", i)
		}
	}
}

func TestDefaultDefinitionsMatchRulesSpec(t *testing.T) {
	specContent, err := os.ReadFile(filepath.Join(repoRoot(t), "specs", "rules.json"))
	if err != nil {
		t.Fatalf("read rules spec: %v", err)
	}

	var spec ruleSpecFile
	if err := json.Unmarshal(specContent, &spec); err != nil {
		t.Fatalf("unmarshal rules spec: %v", err)
	}

	specRules := goImplementationSpecRules(spec.Rules)
	definitions := DefaultDefinitions()
	if len(specRules) != len(definitions) {
		t.Fatalf("len(specRules) = %d, want %d", len(specRules), len(definitions))
	}
	for i, definition := range definitions {
		got := specRules[i]
		want := ruleSpec(definition)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("spec rule[%d] mismatch\ngot:  %#v\nwant: %#v", i, got, want)
		}
	}
}

func goImplementationSpecRules(rules []ruleSpec) []ruleSpec {
	filtered := make([]ruleSpec, 0, len(rules))
	for _, rule := range rules {
		if rule.Category == "repo" || rule.Category == "go" {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

func assertRuleIDs(t *testing.T, findings []Finding, want []string) {
	t.Helper()
	if len(findings) != len(want) {
		t.Fatalf("len(findings) = %d, want %d; findings = %#v", len(findings), len(want), findings)
	}
	for i, wantID := range want {
		if findings[i].RuleID != wantID {
			t.Fatalf("finding[%d].RuleID = %q, want %q", i, findings[i].RuleID, wantID)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller returned ok=false")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

type ruleSpecFile struct {
	Rules []ruleSpec `json:"rules"`
}

type ruleSpec struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Severity    Severity `json:"severity"`
	Path        string   `json:"path"`
	Message     string   `json:"message"`
	Description string   `json:"description"`
	Tool        string   `json:"tool,omitempty"`
	Status      string   `json:"status"`
}

func cleanGoGuardrailFiles(overrides map[string]repo.File) map[string]repo.File {
	files := map[string]repo.File{
		"README.md":                       {Path: "README.md"},
		"AGENTS.md":                       {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		"go/go.mod":                       {Path: "go/go.mod"},
		"go/main.go":                      {Path: "go/main.go"},
		"go/.golangci.yml":                {Path: "go/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		".github/workflows/ci.yml":        {Path: ".github/workflows/ci.yml", Content: goCleanWorkflow},
		"go/scripts/check-go-coverage.sh": {Path: "go/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"go/scripts/check-dry.sh":         {Path: "go/scripts/check-dry.sh", Content: cleanDryScript()},
		"go/scripts/check-crap.sh":        {Path: "go/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"go/scripts/check-mutation.sh":    {Path: "go/scripts/check-mutation.sh", Content: cleanMutationScript("internal/rules/rules.go")},
	}
	for path, file := range overrides {
		files[path] = file
	}
	return files
}

func cleanTwoModuleGoGuardrailFiles(workflow string) map[string]repo.File {
	files := map[string]repo.File{
		"README.md":                {Path: "README.md"},
		"AGENTS.md":                {Path: "AGENTS.md", Content: "Run `go test ./...` before finishing."},
		".github/workflows/ci.yml": {Path: ".github/workflows/ci.yml", Content: workflow},
	}
	for _, root := range []string{"services/api", "services/worker"} {
		files[root+"/go.mod"] = repo.File{Path: root + "/go.mod"}
		files[root+"/main.go"] = repo.File{Path: root + "/main.go"}
		files[root+"/.golangci.yml"] = repo.File{
			Path:    root + "/.golangci.yml",
			Content: "linters:\n  enable:\n    - cyclop\n",
		}
		files[root+"/scripts/check-go-coverage.sh"] = repo.File{
			Path:    root + "/scripts/check-go-coverage.sh",
			Content: cleanCoverageScript,
		}
		files[root+"/scripts/check-dry.sh"] = repo.File{
			Path:    root + "/scripts/check-dry.sh",
			Content: cleanDryScript(),
		}
		files[root+"/scripts/check-crap.sh"] = repo.File{
			Path:    root + "/scripts/check-crap.sh",
			Content: cleanCRAPScript(),
		}
		files[root+"/scripts/check-mutation.sh"] = repo.File{
			Path:    root + "/scripts/check-mutation.sh",
			Content: cleanMutationScript("main.go"),
		}
	}
	return files
}

func cleanDryScript() string {
	return gotools.Dry4Go.GoRunLine(gotools.Latest, "--format", "json", ".") + "\n"
}

func cleanCRAPScript() string {
	return `maximum_crap_score="30"
` + gotools.CRAP4Go.GoRunLine(gotools.Latest) + `
awk -v score="0" -v maximum="$maximum_crap_score" 'BEGIN { exit !(score + 0 <= maximum + 0) }'
`
}

func cleanMutationScript(target string) string {
	return "slophammer-go mutate . --target " + target + "\n"
}

const cleanCoverageScript = `minimum_coverage="80"
go test -coverprofile=coverage.out ./...
total="$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')"
awk -v total="$total" -v minimum="$minimum_coverage" 'BEGIN { exit !(total + 0 >= minimum + 0) }'
`

const goCleanWorkflow = `name: CI
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
`

const nestedGoWorkflow = `name: CI
on: [push]

jobs:
  test:
    steps:
      - run: cd services/api && go test ./...
      - run: cd services/api && go vet ./...
      - run: cd services/api && golangci-lint run
      - run: services/api/scripts/check-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
`
