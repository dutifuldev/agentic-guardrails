package rules

import (
	"context"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestGoDependencyBoundariesReportLocalImportViolation(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod": {
			Path:    "go/go.mod",
			Content: "module example.com/project\n",
		},
		"go/internal/repo/repo.go": {
			Path: "go/internal/repo/repo.go",
			Content: `package repo

import "example.com/project/internal/rules"

func Name() string { return rules.Name() }
`,
		},
		"go/internal/rules/rules.go": {
			Path:    "go/internal/rules/rules.go",
			Content: "package rules\n\nfunc Name() string { return \"rules\" }\n",
		},
	})
	cfg := config.Config{Go: config.GoConfig{DependencyBoundaries: []config.DependencyBoundary{{
		From:  "internal/repo",
		Allow: nil,
	}}}}

	report := RunWithPolicy(context.Background(), snapshot, []Rule{dependencyBoundaryTestRule()}, NewPolicy(cfg, nil))

	assertRuleIDs(t, report.Findings, []string{GoDependencyBoundariesRuleID})
	if report.Findings[0].Path != "go/internal/repo/repo.go" {
		t.Fatalf("Path = %q, want go/internal/repo/repo.go", report.Findings[0].Path)
	}
}

func TestGoDependencyBoundariesAllowDeclaredImports(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod": {
			Path:    "go/go.mod",
			Content: "module example.com/project\n",
		},
		"go/internal/rules/rules.go": {
			Path: "go/internal/rules/rules.go",
			Content: `package rules

import "example.com/project/internal/repo"

func Name() string { return repo.Name() }
`,
		},
		"go/internal/repo/repo.go": {
			Path:    "go/internal/repo/repo.go",
			Content: "package repo\n\nfunc Name() string { return \"repo\" }\n",
		},
	})
	cfg := config.Config{Go: config.GoConfig{DependencyBoundaries: []config.DependencyBoundary{{
		From:  "internal/rules",
		Allow: []string{"internal/repo"},
	}}}}

	report := RunWithPolicy(context.Background(), snapshot, []Rule{dependencyBoundaryTestRule()}, NewPolicy(cfg, nil))

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func TestGoDependencyBoundariesIgnoreExternalImports(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod": {
			Path:    "go.mod",
			Content: "module example.com/project\n",
		},
		"internal/repo/repo.go": {
			Path: "internal/repo/repo.go",
			Content: `package repo

import "strings"

func Name() string { return strings.TrimSpace("repo") }
`,
		},
	})
	cfg := config.Config{Go: config.GoConfig{DependencyBoundaries: []config.DependencyBoundary{{
		From:  "internal/repo",
		Allow: nil,
	}}}}

	report := RunWithPolicy(context.Background(), snapshot, []Rule{dependencyBoundaryTestRule()}, NewPolicy(cfg, nil))

	if !report.OK {
		t.Fatalf("report.OK = false, findings = %#v", report.Findings)
	}
}

func dependencyBoundaryTestRule() Rule {
	return newGoDependencyBoundariesRule(Definition{
		ID:          GoDependencyBoundariesRuleID,
		Severity:    SeverityError,
		Path:        "slophammer.yml",
		Message:     "Go projects must respect configured dependency boundaries",
		Description: "Go projects should keep imports inside configured dependency boundaries.",
	})
}
