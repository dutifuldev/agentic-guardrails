package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func slophammerCIFindings(t *testing.T, files map[string]repo.File) []Finding {
	t.Helper()
	rule := newSlophammerCIRule(definitionByID(t, SlophammerCIRequiredRuleID))
	return rule.Check(context.Background(), repo.NewSnapshot("/repo", files))
}

func definitionByID(t *testing.T, id string) Definition {
	t.Helper()
	for _, definition := range DefaultDefinitions() {
		if definition.ID == id {
			return definition
		}
	}
	t.Fatalf("missing definition %s", id)
	return Definition{}
}

func ciWorkflow(commands ...string) repo.File {
	lines := []string{"name: CI", "on: [push]", "jobs:", "  ci:", "    steps:"}
	for _, command := range commands {
		lines = append(lines, "      - run: "+command)
	}
	return repo.File{Path: ".github/workflows/ci.yml", Content: strings.Join(lines, "\n") + "\n"}
}

func TestSlophammerCIRuleFiresForConfigWithoutInvocation(t *testing.T) {
	findings := slophammerCIFindings(t, map[string]repo.File{
		"slophammer.yml":           {Path: "slophammer.yml", Content: "go: {}\n"},
		".github/workflows/ci.yml": ciWorkflow("go test ./..."),
	})

	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one", findings)
	}
	if findings[0].RuleID != SlophammerCIRequiredRuleID || findings[0].Path != ".github/workflows" {
		t.Fatalf("finding = %#v", findings[0])
	}
}

func TestSlophammerCIRulePassesWithoutConfig(t *testing.T) {
	findings := slophammerCIFindings(t, map[string]repo.File{
		".github/workflows/ci.yml": ciWorkflow("go test ./..."),
	})

	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestSlophammerCIRuleAcceptsCheckerInvocations(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{name: "go checker", command: "go run ./cmd/slophammer-go check .."},
		{name: "ts checker", command: "npx slophammer-ts check ."},
		{name: "rs checker", command: "slophammer-rs check ."},
		{name: "py checker", command: "uvx slophammer-py check ."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := slophammerCIFindings(t, map[string]repo.File{
				"slophammer.yml":           {Path: "slophammer.yml", Content: "go: {}\n"},
				".github/workflows/ci.yml": ciWorkflow(tt.command),
			})

			if len(findings) != 0 {
				t.Fatalf("findings = %#v, want none", findings)
			}
		})
	}
}

func TestSlophammerCIRuleAcceptsActionReference(t *testing.T) {
	workflow := repo.File{
		Path: ".github/workflows/ci.yml",
		Content: "name: CI\non: [push]\njobs:\n  ci:\n    steps:\n" +
			"      - uses: osolmaz/slophammer@v0.2.0\n        with:\n          checker: go\n",
	}
	findings := slophammerCIFindings(t, map[string]repo.File{
		"slophammer.yaml":          {Path: "slophammer.yaml", Content: "go: {}\n"},
		".github/workflows/ci.yml": workflow,
	})

	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestSlophammerCIRuleAcceptsInvocationThroughReferencedScript(t *testing.T) {
	findings := slophammerCIFindings(t, map[string]repo.File{
		"slophammer.yml":           {Path: "slophammer.yml", Content: "go: {}\n"},
		".github/workflows/ci.yml": ciWorkflow("./scripts/gate.sh"),
		"scripts/gate.sh":          {Path: "scripts/gate.sh", Content: "slophammer-go check .\n"},
	})

	if len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestSlophammerInvocationRequiresCheckSubcommand(t *testing.T) {
	if slophammerInvocation("go run ./cmd/slophammer-go dry ..") {
		t.Fatal("slophammerInvocation = true for dry-only evidence")
	}
	if slophammerInvocation("slophammer-go") {
		t.Fatal("slophammerInvocation = true without subcommand")
	}
	if !slophammerInvocation("slophammer-go check .") {
		t.Fatal("slophammerInvocation = false for check invocation")
	}
}

func TestSlophammerInvocationWindowIsBounded(t *testing.T) {
	farApart := "slophammer-go " + strings.Repeat("x", slophammerInvocationWindow) + " check"
	if invocationWithCheck(farApart, "slophammer-go") {
		t.Fatal("invocationWithCheck = true beyond the window")
	}
	repeated := "slophammer-go dry . && slophammer-go check ."
	if !invocationWithCheck(repeated, "slophammer-go") {
		t.Fatal("invocationWithCheck = false for a later matching occurrence")
	}
}
