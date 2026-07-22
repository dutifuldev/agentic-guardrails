package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

func scopeFindings(t *testing.T, files map[string]repo.File, cfg config.Config) []Finding {
	t.Helper()
	rule := newGoScopeRule(definitionByID(t, GoScopeIncompleteRuleID)).(goScopeRule)
	return rule.CheckWithConfig(context.Background(), repo.NewSnapshot("/repo", files), cfg)
}

func scopeSnapshotFiles(paths ...string) map[string]repo.File {
	files := map[string]repo.File{}
	for _, path := range paths {
		files[path] = repo.File{Path: path, Content: "package x\n"}
	}
	return files
}

func TestScopeRuleIsSilentWithoutConfiguredScope(t *testing.T) {
	files := scopeSnapshotFiles("internal/app.go")

	if findings := scopeFindings(t, files, config.Config{}); len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScopeRulePassesWhenScopeCoversAllProductionFiles(t *testing.T) {
	files := scopeSnapshotFiles("internal/app.go", "cmd/main.go", "internal/app_test.go", "fixtures/demo.go")
	cfg := config.Config{Go: config.GoConfig{Targets: []string{"internal", "cmd/"}}}

	if findings := scopeFindings(t, files, cfg); len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScopeRuleDotTargetCoversEverything(t *testing.T) {
	files := scopeSnapshotFiles("internal/app.go", "tools/extra.go")
	cfg := config.Config{Go: config.GoConfig{Targets: []string{"."}}}

	if findings := scopeFindings(t, files, cfg); len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScopeRuleReportsUncoveredProductionDirs(t *testing.T) {
	files := scopeSnapshotFiles("internal/app.go", "tools/extra.go", "cmd/main.go", "root.go")
	cfg := config.Config{Go: config.GoConfig{Targets: []string{"internal"}}}

	findings := scopeFindings(t, files, cfg)

	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one", findings)
	}
	if findings[0].Path != "slophammer.yml" {
		t.Fatalf("finding path = %q", findings[0].Path)
	}
	if !strings.HasSuffix(findings[0].Message, ": ., cmd, tools") {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestScopeRuleAcceptsExcludedProductionFiles(t *testing.T) {
	files := scopeSnapshotFiles("internal/app.go", "tools/extra.go")
	cfg := config.Config{Go: config.GoConfig{
		Targets: []string{"internal"},
		Exclude: []string{"tools/**"},
	}}

	if findings := scopeFindings(t, files, cfg); len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestScopeRuleUsesDryScopeAndExcludes(t *testing.T) {
	files := scopeSnapshotFiles("internal/app.go", "cmd/main.go", "tools/extra.go")
	cfg := config.Config{Go: config.GoConfig{
		DRYPaths:   []string{"internal", "cmd"},
		DRYExclude: []string{"tools/**"},
	}}

	if findings := scopeFindings(t, files, cfg); len(findings) != 0 {
		t.Fatalf("findings = %#v, want none", findings)
	}
}

func TestConventionalGoPathClassifier(t *testing.T) {
	conventional := []string{
		"internal/app_test.go",
		"tests/helper.go",
		"fixtures/demo.go",
		"templates/go/main.go",
		"internal/testdata/data.go",
		"dist/out.go",
		"build/out.go",
		"coverage/out.go",
		"target/out.go",
		"node_modules/dep.go",
		"vendor/dep.go",
		"scripts/tool.go",
		"benches/bench.go",
		"internal/zz_generated.go",
	}
	for _, path := range conventional {
		if !conventionalGoPath(path) {
			t.Fatalf("conventionalGoPath(%q) = false", path)
		}
	}
	if conventionalGoPath("internal/app.go") {
		t.Fatal("conventionalGoPath flagged a production file")
	}
}

func TestGoScopeCoverageCountsConfiguredScope(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", scopeSnapshotFiles(
		"internal/app.go",
		"cmd/main.go",
		"tools/extra.go",
		"internal/app_test.go",
	))
	cfg := config.Config{Go: config.GoConfig{Targets: []string{"internal", "cmd"}}}

	coverage := GoScopeCoverage(snapshot, cfg)

	if coverage == nil || coverage.Scanned != 2 || coverage.ProductionFiles != 3 {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestGoScopeCoverageIsNilWithoutConfiguredScope(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", scopeSnapshotFiles("internal/app.go"))

	if coverage := GoScopeCoverage(snapshot, config.Config{}); coverage != nil {
		t.Fatalf("coverage = %#v, want nil", coverage)
	}
}

func TestScopeRulePlainCheckReturnsNothing(t *testing.T) {
	rule := newGoScopeRule(definitionByID(t, GoScopeIncompleteRuleID))

	if findings := rule.Check(context.Background(), repo.NewSnapshot("/repo", nil)); findings != nil {
		t.Fatalf("findings = %#v, want nil", findings)
	}
}
