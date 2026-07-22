package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoRulesScopeGoCFlagCommandsToNestedModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                       {Path: "README.md"},
		"AGENTS.md":                       {Path: "AGENTS.md"},
		"go/go.mod":                       {Path: "go/go.mod"},
		"go/main.go":                      {Path: "go/main.go"},
		"go/.golangci.yml":                {Path: "go/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"go/scripts/check-go-coverage.sh": {Path: "go/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"go/scripts/check-dry.sh":         {Path: "go/scripts/check-dry.sh", Content: cleanDryScript()},
		"go/scripts/check-crap.sh":        {Path: "go/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"go/scripts/check-mutation.sh":    {Path: "go/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: go -C go test ./...
      - run: go -C=go vet ./...
      - run: go -C go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.0 run
      - run: go/scripts/check-go-coverage.sh
      - run: go/scripts/check-dry.sh
      - run: go/scripts/check-crap.sh
      - run: go/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesScopeWorkflowEvidenceToModuleRoots(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                                    {Path: "README.md"},
		"AGENTS.md":                                    {Path: "AGENTS.md"},
		"services/api/go.mod":                          {Path: "services/api/go.mod"},
		"services/api/main.go":                         {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                   {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh":    {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":            {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":           {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":       {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                       {Path: "services/worker/go.mod"},
		"services/worker/main.go":                      {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":                {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-go-coverage.sh": {Path: "services/worker/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/worker/scripts/check-dry.sh":         {Path: "services/worker/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/worker/scripts/check-crap.sh":        {Path: "services/worker/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/worker/scripts/check-mutation.sh":    {Path: "services/worker/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    steps:
      - run: cd services/api && go test ./...
      - run: cd services/api && go vet ./...
      - run: cd services/api && golangci-lint run
      - run: services/api/scripts/check-go-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
  worker:
    steps:
      - run: services/worker/scripts/check-go-coverage.sh
      - run: services/worker/scripts/check-dry.sh
      - run: services/worker/scripts/check-crap.sh
      - run: services/worker/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoLintRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestGoRulesAcceptDotSlashWorkflowModuleRoots(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                                    {Path: "README.md"},
		"AGENTS.md":                                    {Path: "AGENTS.md"},
		"services/api/go.mod":                          {Path: "services/api/go.mod"},
		"services/api/main.go":                         {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                   {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh":    {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":            {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":           {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":       {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                       {Path: "services/worker/go.mod"},
		"services/worker/main.go":                      {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":                {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-go-coverage.sh": {Path: "services/worker/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/worker/scripts/check-dry.sh":         {Path: "services/worker/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/worker/scripts/check-crap.sh":        {Path: "services/worker/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/worker/scripts/check-mutation.sh":    {Path: "services/worker/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    steps:
      - run: cd ./services/api && go test ./...
      - run: cd ./services/api && go vet ./...
      - run: cd ./services/api && golangci-lint run
      - run: ./services/api/scripts/check-go-coverage.sh
      - run: ./services/api/scripts/check-dry.sh
      - run: ./services/api/scripts/check-crap.sh
      - run: ./services/api/scripts/check-mutation.sh
  worker:
    steps:
      - run: cd ./services/worker && go test ./...
      - run: cd ./services/worker && go vet ./...
      - run: cd ./services/worker && golangci-lint run
      - run: ./services/worker/scripts/check-go-coverage.sh
      - run: ./services/worker/scripts/check-dry.sh
      - run: ./services/worker/scripts/check-crap.sh
      - run: ./services/worker/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesFilterWorkflowCommandsPerModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                                    {Path: "README.md"},
		"AGENTS.md":                                    {Path: "AGENTS.md"},
		"services/api/go.mod":                          {Path: "services/api/go.mod"},
		"services/api/main.go":                         {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                   {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh":    {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":            {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":           {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":       {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                       {Path: "services/worker/go.mod"},
		"services/worker/main.go":                      {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":                {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-go-coverage.sh": {Path: "services/worker/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/worker/scripts/check-dry.sh":         {Path: "services/worker/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/worker/scripts/check-crap.sh":        {Path: "services/worker/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/worker/scripts/check-mutation.sh":    {Path: "services/worker/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    steps:
      - run: cd services/api && go test ./...
      - run: cd services/api && go vet ./...
      - run: cd services/api && golangci-lint run
  worker:
    steps:
      - run: cd services/worker && golangci-lint run
      - run: echo services/worker
      - run: services/api/scripts/check-go-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
      - run: services/worker/scripts/check-go-coverage.sh
      - run: services/worker/scripts/check-dry.sh
      - run: services/worker/scripts/check-crap.sh
      - run: services/worker/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoVetRequiredRuleID})
}

func TestScopedWorkflowStepBlockFiltersMixedModuleCommands(t *testing.T) {
	block := `      - run: |-
          cd services/worker && go test ./...
          cd services/api
          go vet ./...
          golangci-lint run
          cd ../worker
          go vet ./...
`

	scoped, ok := scopedWorkflowStepBlock(block, "services/api", []string{"services/api", "services/worker"})

	if !ok {
		t.Fatal("scopedWorkflowStepBlock returned ok=false")
	}
	if strings.Contains(scoped, "go test ./...") {
		t.Fatalf("scoped block leaked worker command: %q", scoped)
	}
	if strings.Contains(scoped, "cd ../worker") {
		t.Fatalf("scoped block kept command after leaving api: %q", scoped)
	}
	if !strings.Contains(scoped, "- run: |-") ||
		!strings.Contains(scoped, "cd services/api") ||
		!strings.Contains(scoped, "go vet ./...") ||
		!strings.Contains(scoped, "golangci-lint run") {
		t.Fatalf("scoped block lost api run content: %q", scoped)
	}
}

func TestGoRulesAcceptChompedMixedModuleWorkflowRunBlocks(t *testing.T) {
	workflow := `name: CI
on: [push]
jobs:
  all:
    steps:
      - run: |-
          cd services/api
          go test ./...
          go vet ./...
          golangci-lint run
          ./scripts/check-go-coverage.sh
          ./scripts/check-dry.sh
          ./scripts/check-crap.sh
          ./scripts/check-mutation.sh
          cd services/worker
          go test ./...
          go vet ./...
          golangci-lint run
          ./scripts/check-go-coverage.sh
          ./scripts/check-dry.sh
          ./scripts/check-crap.sh
          ./scripts/check-mutation.sh
`
	snapshot := repo.NewSnapshot("/repo", cleanTwoModuleGoGuardrailFiles(workflow))

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesKeepRootWorkflowStepsWithNestedModules(t *testing.T) {
	files := map[string]repo.File{
		"README.md":                                 {Path: "README.md"},
		"AGENTS.md":                                 {Path: "AGENTS.md"},
		"go.mod":                                    {Path: "go.mod"},
		"main.go":                                   {Path: "main.go"},
		".golangci.yml":                             {Path: ".golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"scripts/check-go-coverage.sh":              {Path: "scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"scripts/check-dry.sh":                      {Path: "scripts/check-dry.sh", Content: cleanDryScript()},
		"scripts/check-crap.sh":                     {Path: "scripts/check-crap.sh", Content: cleanCRAPScript()},
		"scripts/check-mutation.sh":                 {Path: "scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/api/go.mod":                       {Path: "services/api/go.mod"},
		"services/api/main.go":                      {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh": {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":         {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":        {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":    {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  root:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
      - run: ./scripts/check-dry.sh
      - run: ./scripts/check-crap.sh
      - run: ./scripts/check-mutation.sh
  api:
    steps:
      - run: cd services/api && go test ./...
      - run: cd services/api && go vet ./...
      - run: cd services/api && golangci-lint run
      - run: services/api/scripts/check-go-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
`,
		},
	}

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesKeepWholeWorkflowStepForModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                                    {Path: "README.md"},
		"AGENTS.md":                                    {Path: "AGENTS.md"},
		"services/api/go.mod":                          {Path: "services/api/go.mod"},
		"services/api/main.go":                         {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                   {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh":    {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":            {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":           {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":       {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                       {Path: "services/worker/go.mod"},
		"services/worker/main.go":                      {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":                {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-go-coverage.sh": {Path: "services/worker/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/worker/scripts/check-dry.sh":         {Path: "services/worker/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/worker/scripts/check-crap.sh":        {Path: "services/worker/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/worker/scripts/check-mutation.sh":    {Path: "services/worker/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  go:
    steps:
      - run: go test ./...
        working-directory: services/api
      - run: go vet ./...
        working-directory: services/api
      - uses: golangci/golangci-lint-action@v8
        with:
          working-directory: services/api
      - run: go test ./...
        working-directory: services/worker
      - run: go vet ./...
        working-directory: services/worker
      - uses: golangci/golangci-lint-action@v8
        with:
          working-directory: services/worker
      - run: ./scripts/check-go-coverage.sh
        working-directory: services/api
      - run: ./scripts/check-dry.sh
        working-directory: services/api
      - run: ./scripts/check-crap.sh
        working-directory: services/api
      - run: ./scripts/check-mutation.sh
        working-directory: services/api
      - run: ./scripts/check-go-coverage.sh
        working-directory: services/worker
      - run: ./scripts/check-dry.sh
        working-directory: services/worker
      - run: ./scripts/check-crap.sh
        working-directory: services/worker
      - run: ./scripts/check-mutation.sh
        working-directory: services/worker
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesMatchNestedModuleWorkflowRootsOnBoundaries(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                              {Path: "README.md"},
		"AGENTS.md":                              {Path: "AGENTS.md"},
		"services/go.mod":                        {Path: "services/go.mod"},
		"services/main.go":                       {Path: "services/main.go"},
		"services/.golangci.yml":                 {Path: "services/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/scripts/check-go-coverage.sh":  {Path: "services/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/scripts/check-dry.sh":          {Path: "services/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/scripts/check-crap.sh":         {Path: "services/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/scripts/check-mutation.sh":     {Path: "services/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/api/go.mod":                    {Path: "services/api/go.mod"},
		"services/api/main.go":                   {Path: "services/api/main.go"},
		"services/api/.golangci.yml":             {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-coverage.sh": {Path: "services/api/scripts/check-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":      {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":     {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh": {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    steps:
      - run: cd services/api && go test ./...
      - run: cd services/api && go vet ./...
      - run: cd services/api && golangci-lint run
      - run: services/api/scripts/check-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
  services:
    steps:
      - run: services/scripts/check-go-coverage.sh
      - run: services/scripts/check-dry.sh
      - run: services/scripts/check-crap.sh
      - run: services/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoLintRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestGoRulesKeepJobDefaultWorkingDirectoryWithWorkflowSteps(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                                    {Path: "README.md"},
		"AGENTS.md":                                    {Path: "AGENTS.md"},
		"services/api/go.mod":                          {Path: "services/api/go.mod"},
		"services/api/main.go":                         {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                   {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh":    {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":            {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":           {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":       {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                       {Path: "services/worker/go.mod"},
		"services/worker/main.go":                      {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":                {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-go-coverage.sh": {Path: "services/worker/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/worker/scripts/check-dry.sh":         {Path: "services/worker/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/worker/scripts/check-crap.sh":        {Path: "services/worker/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/worker/scripts/check-mutation.sh":    {Path: "services/worker/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    defaults:
      run:
        working-directory: services/api
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
        working-directory: services/api
      - run: ./scripts/check-dry.sh
        working-directory: services/api
      - run: ./scripts/check-crap.sh
        working-directory: services/api
      - run: ./scripts/check-mutation.sh
        working-directory: services/api
  worker:
    defaults:
      run:
        working-directory: services/worker
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: ./scripts/check-go-coverage.sh
        working-directory: services/worker
      - run: ./scripts/check-dry.sh
        working-directory: services/worker
      - run: ./scripts/check-crap.sh
        working-directory: services/worker
      - run: ./scripts/check-mutation.sh
        working-directory: services/worker
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesIgnoreWorkflowListsBeforeSteps(t *testing.T) {
	blocks := workflowStepBlocks(`name: CI
on: [push]
jobs:
  go:
    strategy:
      matrix:
        include:
          - go: "1.23"
    defaults:
      run:
        working-directory: go
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
`)

	if len(blocks) != 3 {
		t.Fatalf("len(blocks) = %d, want 3: %#v", len(blocks), blocks)
	}
	for _, block := range blocks {
		if !strings.Contains(block, "working-directory: go") {
			t.Fatalf("block missing working directory context: %q", block)
		}
		if strings.Contains(block, `- go: "1.23"`) {
			t.Fatalf("block included matrix list item before steps: %q", block)
		}
	}
}

func TestGoRulesKeepTopLevelDefaultWorkingDirectoryWithWorkflowSteps(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                       {Path: "README.md"},
		"AGENTS.md":                       {Path: "AGENTS.md"},
		"go/go.mod":                       {Path: "go/go.mod"},
		"go/main.go":                      {Path: "go/main.go"},
		"go/.golangci.yml":                {Path: "go/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"go/scripts/check-go-coverage.sh": {Path: "go/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"go/scripts/check-dry.sh":         {Path: "go/scripts/check-dry.sh", Content: cleanDryScript()},
		"go/scripts/check-crap.sh":        {Path: "go/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"go/scripts/check-mutation.sh":    {Path: "go/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
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

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesDoNotApplyRunDefaultsToActionSteps(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    defaults:
      run:
        working-directory: go
    steps:
      - run: go test ./...
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@v8
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

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoLintRequiredRuleID})
}

func TestGoRulesIgnoreWorkflowListsBeforeJobs(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                       {Path: "README.md"},
		"AGENTS.md":                       {Path: "AGENTS.md"},
		"go/go.mod":                       {Path: "go/go.mod"},
		"go/main.go":                      {Path: "go/main.go"},
		"go/.golangci.yml":                {Path: "go/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"go/scripts/check-go-coverage.sh": {Path: "go/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"go/scripts/check-dry.sh":         {Path: "go/scripts/check-dry.sh", Content: cleanDryScript()},
		"go/scripts/check-crap.sh":        {Path: "go/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"go/scripts/check-mutation.sh":    {Path: "go/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on:
  push:
    branches:
      - main
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

	report := Run(context.Background(), snapshot, DefaultRules())

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoRulesScopeSingleNestedModuleWorkflowEvidence(t *testing.T) {
	files := cleanGoGuardrailFiles(map[string]repo.File{
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: golangci-lint run
      - run: go/scripts/check-go-coverage.sh
      - run: go/scripts/check-dry.sh
      - run: go/scripts/check-crap.sh
      - run: go/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), repo.NewSnapshot("/repo", files), DefaultRules())

	assertRuleIDs(t, report.Findings, []string{
		GoLintRequiredRuleID,
		GoVetRequiredRuleID,
	})
}

func TestGoRulesKeepNamedWorkflowStepsForModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"README.md":                                    {Path: "README.md"},
		"AGENTS.md":                                    {Path: "AGENTS.md"},
		"services/api/go.mod":                          {Path: "services/api/go.mod"},
		"services/api/main.go":                         {Path: "services/api/main.go"},
		"services/api/.golangci.yml":                   {Path: "services/api/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/api/scripts/check-go-coverage.sh":    {Path: "services/api/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/api/scripts/check-dry.sh":            {Path: "services/api/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/api/scripts/check-crap.sh":           {Path: "services/api/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/api/scripts/check-mutation.sh":       {Path: "services/api/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		"services/worker/go.mod":                       {Path: "services/worker/go.mod"},
		"services/worker/main.go":                      {Path: "services/worker/main.go"},
		"services/worker/.golangci.yml":                {Path: "services/worker/.golangci.yml", Content: "linters:\n  enable:\n    - cyclop\n"},
		"services/worker/scripts/check-go-coverage.sh": {Path: "services/worker/scripts/check-go-coverage.sh", Content: cleanCoverageScript},
		"services/worker/scripts/check-dry.sh":         {Path: "services/worker/scripts/check-dry.sh", Content: cleanDryScript()},
		"services/worker/scripts/check-crap.sh":        {Path: "services/worker/scripts/check-crap.sh", Content: cleanCRAPScript()},
		"services/worker/scripts/check-mutation.sh":    {Path: "services/worker/scripts/check-mutation.sh", Content: cleanMutationScript("main.go")},
		".github/workflows/ci.yml": {
			Path: ".github/workflows/ci.yml",
			Content: `name: CI
on: [push]
jobs:
  api:
    steps:
      - name: api tests
        run: cd services/api && go test ./...
      - name: api vet
        run: cd services/api && go vet ./...
      - name: api lint
        run: cd services/api && golangci-lint run
  worker:
    steps:
      - name: worker lint
        run: cd services/worker && golangci-lint run
      - name: marker
        run: echo services/worker
      - run: services/api/scripts/check-go-coverage.sh
      - run: services/api/scripts/check-dry.sh
      - run: services/api/scripts/check-crap.sh
      - run: services/api/scripts/check-mutation.sh
      - run: services/worker/scripts/check-go-coverage.sh
      - run: services/worker/scripts/check-dry.sh
      - run: services/worker/scripts/check-crap.sh
      - run: services/worker/scripts/check-mutation.sh
`,
		},
	})

	report := Run(context.Background(), snapshot, DefaultRules())

	assertRuleIDs(t, report.Findings, []string{GoVetRequiredRuleID})
}

func TestBindingWorkflowTriggersGateCommandText(t *testing.T) {
	tests := []struct {
		name    string
		on      string
		binding bool
	}{
		{name: "push without filter", on: "on:\n  push:\n", binding: true},
		{name: "push to main", on: "on:\n  push:\n    branches:\n      - main\n", binding: true},
		{name: "push wildcard branch", on: "on:\n  push:\n    branches:\n      - \"release/*\"\n", binding: true},
		{name: "push scalar branch", on: "on:\n  push:\n    branches: trunk\n", binding: true},
		{name: "push to feature only", on: "on:\n  push:\n    branches:\n      - feature\n", binding: false},
		{name: "push with tag filter only", on: "on:\n  push:\n    tags:\n      - \"v*\"\n", binding: false},
		{name: "push with tags and main", on: "on:\n  push:\n    branches:\n      - main\n    tags:\n      - \"v*\"\n", binding: true},
		{name: "pull request entry", on: "on:\n  pull_request:\n", binding: true},
		{name: "workflow dispatch only", on: "on:\n  workflow_dispatch:\n", binding: false},
		{name: "scalar trigger", on: "on: push\n", binding: true},
		{name: "non-binding scalar trigger", on: "on: workflow_call\n", binding: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bindingWorkflowCommandText("name: CI\n" + tt.on + "jobs:\n  test:\n    steps:\n      - run: go test ./...\n")
			if binding := strings.Contains(got, "go test ./..."); binding != tt.binding {
				t.Fatalf("binding = %v, want %v; text = %q", binding, tt.binding, got)
			}
		})
	}
}

func TestBindingWorkflowCommandTextIgnoresStepMetadata(t *testing.T) {
	got := bindingWorkflowCommandText(`name: CI
on: [push]
jobs:
  test:
    steps:
      - name: go test ./...
        run: echo ok
      - name: go vet ./...
        run: |
          echo still ok
`)

	if strings.Contains(got, "go test ./...") || strings.Contains(got, "go vet ./...") {
		t.Fatalf("workflow metadata leaked into command text: %q", got)
	}
	if !strings.Contains(got, "echo ok") || !strings.Contains(got, "echo still ok") {
		t.Fatalf("workflow run content missing from command text: %q", got)
	}
}

func TestBindingWorkflowCommandTextFoldsRunBlocks(t *testing.T) {
	got := bindingWorkflowCommandText(`name: CI
on: [push]
jobs:
  test:
    steps:
      - run: >
          go vet
          ./...
`)

	if strings.TrimSpace(got) != "go vet ./..." {
		t.Fatalf("workflow folded run command text = %q", got)
	}
}

func TestBindingWorkflowCommandTextAcceptsChompedRunBlocks(t *testing.T) {
	got := bindingWorkflowCommandText(`name: CI
on: [push]
jobs:
  test:
    steps:
      - run: |-
          go test ./...
      - run: >-
          go vet
          ./...
`)

	want := "go test ./..." + workflowStepBoundary + "go vet ./..."
	if got != want {
		t.Fatalf("workflow chomped run command text = %q, want %q", got, want)
	}
}

func TestBindingWorkflowCommandTextIgnoresPostRunYamlBlocks(t *testing.T) {
	got := bindingWorkflowCommandText(`name: CI
on: [push]
jobs:
  test:
    steps:
      - run: |
          echo noop
        env:
          SCRIPT: |
            go test ./...
            go vet ./...
`)

	if strings.Contains(got, "go test ./...") || strings.Contains(got, "go vet ./...") {
		t.Fatalf("post-run yaml leaked into command text: %q", got)
	}
	if strings.TrimSpace(got) != "echo noop" {
		t.Fatalf("workflow run command text = %q", got)
	}
}
