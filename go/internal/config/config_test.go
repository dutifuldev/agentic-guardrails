package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestLoadReturnsZeroConfigWhenMissing(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", nil))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Go.DependencyBoundaries) != 0 {
		t.Fatalf("DependencyBoundaries = %#v, want empty", cfg.Go.DependencyBoundaries)
	}
}

func TestLoadParsesPolicyConfig(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `rules:
  go.crap-required:
    severity: warn
go:
  coverage:
    threshold: 85
    profile: coverage.out
  targets:
    - go
  exclude:
    - "go/generated/**"
  dry:
    max_findings: 0
    paths:
      - go/cmd
      - go/internal
    exclude:
      - "**/*_test.go"
  crap:
    max_score: 8
  mutation:
    targets:
      - internal/rules
    exclude:
      - "internal/rules/generated/**"
  dependency_boundaries:
    - from: internal/repo
      allow: []
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := cfg.RuleSeverity("go.crap-required", "error"); got != "warn" {
		t.Fatalf("RuleSeverity = %q, want warn", got)
	}
	assertParsedGoPolicyConfig(t, cfg)
}

func assertParsedGoPolicyConfig(t *testing.T, cfg Config) {
	t.Helper()
	if cfg.Go.CoverageThreshold != 85 || cfg.Go.DRYMaxCandidates != 0 || !cfg.Go.DRYMaxCandidatesSet || cfg.Go.CRAPMaxScore != 8 {
		t.Fatalf("Go config = %#v", cfg.Go)
	}
	if cfg.Go.CoverageProfile != "coverage.out" {
		t.Fatalf("CoverageProfile = %q, want coverage.out", cfg.Go.CoverageProfile)
	}
	assertParsedDryPaths(t, cfg)
	assertParsedTargets(t, cfg)
	assertParsedMutation(t, cfg)
	assertParsedDependencyBoundaries(t, cfg)
}

func assertParsedTargets(t *testing.T, cfg Config) {
	t.Helper()
	if !reflect.DeepEqual(cfg.Go.Targets, []string{"go"}) {
		t.Fatalf("Targets = %#v", cfg.Go.Targets)
	}
	if !reflect.DeepEqual(cfg.Go.Exclude, []string{"go/generated/**"}) {
		t.Fatalf("Exclude = %#v", cfg.Go.Exclude)
	}
}

func assertParsedDryPaths(t *testing.T, cfg Config) {
	t.Helper()
	if !reflect.DeepEqual(cfg.Go.DRYPaths, []string{"go/cmd", "go/internal"}) {
		t.Fatalf("DRYPaths = %#v", cfg.Go.DRYPaths)
	}
	if !reflect.DeepEqual(cfg.Go.DRYExclude, []string{"**/*_test.go"}) {
		t.Fatalf("DRYExclude = %#v", cfg.Go.DRYExclude)
	}
}

func assertParsedMutation(t *testing.T, cfg Config) {
	t.Helper()
	if !reflect.DeepEqual(cfg.Go.Mutation.Targets, []string{"internal/rules"}) {
		t.Fatalf("Mutation.Targets = %#v", cfg.Go.Mutation.Targets)
	}
	if !reflect.DeepEqual(cfg.Go.Mutation.Exclude, []string{"internal/rules/generated/**"}) {
		t.Fatalf("Mutation.Exclude = %#v", cfg.Go.Mutation.Exclude)
	}
}

func assertParsedDependencyBoundaries(t *testing.T, cfg Config) {
	t.Helper()
	if len(cfg.Go.DependencyBoundaries) != 1 || cfg.Go.DependencyBoundaries[0].From != "internal/repo" {
		t.Fatalf("DependencyBoundaries = %#v", cfg.Go.DependencyBoundaries)
	}
}

func TestLoadPrefersRootConfig(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"fixtures/repos/go-bad-dependency/slophammer.yml": {
			Path: "fixtures/repos/go-bad-dependency/slophammer.yml",
			Content: `go:
  dry:
    max_findings: 1
`,
		},
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  dry:
    max_findings: 0
  targets:
    - go/internal/rules
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Go.DRYMaxCandidates != 0 || !cfg.Go.DRYMaxCandidatesSet {
		t.Fatalf("DRYMaxCandidates = %d, set=%v, want 0 true", cfg.Go.DRYMaxCandidates, cfg.Go.DRYMaxCandidatesSet)
	}
	if !reflect.DeepEqual(cfg.Go.Targets, []string{"go/internal/rules"}) {
		t.Fatalf("Targets = %#v", cfg.Go.Targets)
	}
}

func TestGoMutationScopeIsRelativeToConfigFile(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - internal
  exclude:
    - "generated/**"
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	targets, exclude := cfg.GoMutationScope()

	if !reflect.DeepEqual(targets, []string{"go/internal"}) {
		t.Fatalf("targets = %#v", targets)
	}
	if !reflect.DeepEqual(exclude, []string{"generated/**", "go/generated/**"}) {
		t.Fatalf("exclude = %#v", exclude)
	}
}

func TestGoCoverageProfileIsRelativeToConfigFile(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  coverage:
    profile: coverage.out
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := cfg.GoCoverageProfile(); got != "go/coverage.out" {
		t.Fatalf("GoCoverageProfile = %q, want go/coverage.out", got)
	}
}

func TestGoCoverageProfilePreservesAbsoluteWindowsPath(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  coverage:
    profile: 'C:\ci\coverage.out'
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if got := cfg.GoCoverageProfile(); got != `C:\ci\coverage.out` {
		t.Fatalf("GoCoverageProfile = %q, want Windows absolute path", got)
	}
}

func TestGoMutationScopePreservesBasenameExcludesForConfigFile(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - internal
  exclude:
    - pattern: "*.pb.go"
      reason: protobuf bindings are machine written
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	targets, exclude := cfg.GoMutationScope()

	if !reflect.DeepEqual(targets, []string{"go/internal"}) {
		t.Fatalf("targets = %#v", targets)
	}
	if !reflect.DeepEqual(exclude, []string{"*.pb.go"}) {
		t.Fatalf("exclude = %#v", exclude)
	}
}

func TestGoMutationScopeUsesMutationOverride(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"go/slophammer.yml": {
			Path: "go/slophammer.yml",
			Content: `go:
  targets:
    - internal
  mutation:
    targets:
      - cmd
    exclude:
      - "cmd/generated/**"
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	targets, exclude := cfg.GoMutationScope()

	if !reflect.DeepEqual(targets, []string{"go/cmd"}) {
		t.Fatalf("targets = %#v", targets)
	}
	if !reflect.DeepEqual(exclude, []string{"cmd/generated/**", "go/cmd/generated/**"}) {
		t.Fatalf("exclude = %#v", exclude)
	}
}

func TestGoMutationScopeDoesNotInheritSharedExcludeForMutationTargets(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - internal
  exclude:
    - "generated/**"
  mutation:
    targets:
      - internal/generated
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	targets, exclude := cfg.GoMutationScope()

	if !reflect.DeepEqual(targets, []string{"internal/generated"}) {
		t.Fatalf("targets = %#v", targets)
	}
	if len(exclude) != 0 {
		t.Fatalf("exclude = %#v, want empty", exclude)
	}
}

func TestGoMutationScopeUsesMutationExcludeWithSharedTargets(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  targets:
    - internal
  exclude:
    - "internal/generated/**"
  mutation:
    exclude:
      - "internal/mutation_generated/**"
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	targets, exclude := cfg.GoMutationScope()

	if !reflect.DeepEqual(targets, []string{"internal"}) {
		t.Fatalf("targets = %#v", targets)
	}
	if !reflect.DeepEqual(exclude, []string{"internal/mutation_generated/**"}) {
		t.Fatalf("exclude = %#v", exclude)
	}
}

func TestLoadParsesNestedDryConfig(t *testing.T) {
	cfg, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  dry:
    max_findings: 0
    paths:
      - go/internal
    exclude:
      - "**/*_test.go"
    structural:
      enabled: true
      threshold: 0.82
      min_lines: 4
      min_nodes: 20
    copied_blocks:
      enabled: true
      min_tokens: 100
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	assertNestedDryConfig(t, cfg)
}

func assertNestedDryConfig(t *testing.T, cfg Config) {
	t.Helper()
	if cfg.Go.DRYMaxCandidates != 0 || !cfg.Go.DRYMaxCandidatesSet || !cfg.Go.DRY.MaxFindingsSet {
		t.Fatalf("DRY budget = %#v", cfg.Go)
	}
	if !reflect.DeepEqual(cfg.Go.DRYPaths, []string{"go/internal"}) || !reflect.DeepEqual(cfg.Go.DRY.Paths, []string{"go/internal"}) {
		t.Fatalf("DRY paths = %#v nested=%#v", cfg.Go.DRYPaths, cfg.Go.DRY.Paths)
	}
	assertNestedDryEngines(t, cfg.Go.DRY)
}

func assertNestedDryEngines(t *testing.T, cfg DryConfig) {
	t.Helper()
	if !cfg.Structural.EnabledSet || !cfg.Structural.Enabled || cfg.Structural.Threshold != 0.82 {
		t.Fatalf("DRY structural = %#v", cfg.Structural)
	}
	if !cfg.CopiedBlocks.EnabledSet || !cfg.CopiedBlocks.Enabled || cfg.CopiedBlocks.MinTokens != 100 {
		t.Fatalf("DRY copied blocks = %#v", cfg.CopiedBlocks)
	}
}

func TestLoadRejectsNegativeDryBudget(t *testing.T) {
	_, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path:    "slophammer.yml",
			Content: "go:\n  dry:\n    max_findings: -1\n",
		},
	}))
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), "max_findings") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsInvalidNestedDryTargets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "threshold", content: "go:\n  dry:\n    structural:\n      threshold: 1.2\n", want: "threshold"},
		{name: "min lines", content: "go:\n  dry:\n    structural:\n      min_lines: -1\n", want: "min_lines"},
		{name: "min nodes", content: "go:\n  dry:\n    structural:\n      min_nodes: -1\n", want: "min_nodes"},
		{name: "copied tokens", content: "go:\n  dry:\n    copied_blocks:\n      min_tokens: -1\n", want: "min_tokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
				"slophammer.yml": {Path: "slophammer.yml", Content: tc.content},
			}))
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadRejectsWeakerRecommendedGoTargets(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "coverage",
			content: "go:\n  coverage:\n    threshold: 84\n",
			want:    "coverage.threshold",
		},
		{
			name:    "crap",
			content: "go:\n  crap:\n    max_score: 9\n",
			want:    "crap.max_score",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
				"slophammer.yml": {Path: "slophammer.yml", Content: tt.content},
			}))
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	_, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path:    "slophammer.yml",
			Content: "rules:\n  go.crap-required:\n    severity: info\n",
		},
	}))
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), "severity") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsUnknownConfigKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "root", content: "made_up: true\n", want: "root.made_up"},
		{name: "rules", content: "rules:\n  repo.readme-required:\n    made_up: true\n", want: "rules.repo.readme-required.made_up"},
		{name: "go", content: "go:\n  made_up: true\n", want: "go.made_up"},
		{name: "go mutation", content: "go:\n  mutation:\n    made_up: true\n", want: "go.mutation.made_up"},
		{name: "removed go mutation targets", content: "go:\n  mutation_targets:\n    - main.go\n", want: "go.mutation_targets"},
		{name: "removed go coverage_threshold", content: "go:\n  coverage_threshold: 85\n", want: "go.coverage_threshold"},
		{name: "removed go crap_max_score", content: "go:\n  crap_max_score: 8\n", want: "go.crap_max_score"},
		{name: "removed typescript complexity_max", content: "typescript:\n  complexity_max: 8\n", want: "typescript.complexity_max"},
		{name: "removed typescript mutation_targets", content: "typescript:\n  mutation_targets:\n    - src/rules.ts\n", want: "typescript.mutation_targets"},
		{name: "removed rust coverage_threshold", content: "rust:\n  coverage_threshold: 85\n", want: "rust.coverage_threshold"},
		{name: "go dry", content: "go:\n  dry:\n    made_up: true\n", want: "go.dry.made_up"},
		{name: "go structural", content: "go:\n  dry:\n    structural:\n      made_up: true\n", want: "go.dry.structural.made_up"},
		{name: "go boundary", content: "go:\n  dependency_boundaries:\n    - from: internal/app\n      made_up: true\n", want: "go.dependency_boundaries[0].made_up"},
		{name: "typescript", content: "typescript:\n  made_up: true\n", want: "typescript.made_up"},
		{name: "typescript coverage", content: "typescript:\n  coverage:\n    made_up: true\n", want: "typescript.coverage.made_up"},
		{name: "typescript complexity", content: "typescript:\n  complexity:\n    made_up: true\n", want: "typescript.complexity.made_up"},
		{name: "typescript mutation", content: "typescript:\n  mutation:\n    made_up: true\n", want: "typescript.mutation.made_up"},
		{name: "typescript dry", content: "typescript:\n  dry:\n    made_up: true\n", want: "typescript.dry.made_up"},
		{name: "typescript copied blocks", content: "typescript:\n  dry:\n    copied_blocks:\n      made_up: true\n", want: "typescript.dry.copied_blocks.made_up"},
		{name: "typescript boundary", content: "typescript:\n  dependency_boundaries:\n    - from: src/app\n      made_up: true\n", want: "typescript.dependency_boundaries[0].made_up"},
		{name: "python", content: "python:\n  made_up: true\n", want: "python.made_up"},
		{name: "python coverage", content: "python:\n  coverage:\n    made_up: true\n", want: "python.coverage.made_up"},
		{name: "python typecheck", content: "python:\n  typecheck:\n    made_up: true\n", want: "python.typecheck.made_up"},
		{name: "python demotion", content: "python:\n  typecheck:\n    demotions:\n      - rule: deprecated\n        made_up: true\n", want: "python.typecheck.demotions[0].made_up"},
		{name: "python boundary", content: "python:\n  dependency_boundaries:\n    - from: src/app\n      made_up: true\n", want: "python.dependency_boundaries[0].made_up"},
		{name: "python dry exclude", content: "python:\n  dry:\n    exclude:\n      - pattern: src/**\n        made_up: true\n", want: "python.dry.exclude[0].made_up"},
		{name: "rust", content: "rust:\n  made_up: true\n", want: "rust.made_up"},
		{name: "rust dry", content: "rust:\n  dry:\n    made_up: true\n", want: "rust.dry.made_up"},
		{name: "rust unsafe allow", content: "rust:\n  unsafe:\n    allow:\n      - path: src/lib.rs\n        made_up: true\n", want: "rust.unsafe.allow[0].made_up"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
				"slophammer.yml": {Path: "slophammer.yml", Content: tt.content},
			}))
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func loadConfig(t *testing.T, content string) (Config, error) {
	t.Helper()
	return Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {Path: "slophammer.yml", Content: content},
	}))
}

func TestLoadAcceptsConventionalStringExcludes(t *testing.T) {
	cfg, err := loadConfig(t, `go:
  targets:
    - go
  exclude:
    - "fixtures/**"
    - "**/*_test.go"
    - "go/generated/**"
`)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(cfg.Go.Exclude, []string{"fixtures/**", "**/*_test.go", "go/generated/**"}) {
		t.Fatalf("Exclude = %#v", cfg.Go.Exclude)
	}
}

func TestLoadAcceptsReasonedExcludes(t *testing.T) {
	cfg, err := loadConfig(t, `go:
  targets:
    - go
  exclude:
    - pattern: "go/internal/vendored_parser/**"
      reason: vendored upstream code, synced verbatim
  dry:
    exclude:
      - pattern: "go/internal/legacy/**"
        reason: scheduled for deletion
  mutation:
    exclude:
      - pattern: "go/internal/slow/**"
        reason: mutation runtime is prohibitive
`)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(cfg.Go.Exclude, []string{"go/internal/vendored_parser/**"}) {
		t.Fatalf("Exclude = %#v", cfg.Go.Exclude)
	}
	if !reflect.DeepEqual(cfg.Go.DRYExclude, []string{"go/internal/legacy/**"}) {
		t.Fatalf("DRYExclude = %#v", cfg.Go.DRYExclude)
	}
	if !reflect.DeepEqual(cfg.Go.Mutation.Exclude, []string{"go/internal/slow/**"}) {
		t.Fatalf("Mutation.Exclude = %#v", cfg.Go.Mutation.Exclude)
	}
}

func TestLoadRejectsProductionStringExcludes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "go exclude",
			content: "go:\n  exclude:\n    - \"internal/secret/**\"\n",
			want:    "go.exclude requires a reason for production paths",
		},
		{
			name:    "dry exclude",
			content: "go:\n  dry:\n    exclude:\n      - \"internal/secret/**\"\n",
			want:    "go.dry.exclude requires a reason for production paths",
		},
		{
			name:    "mutation exclude",
			content: "go:\n  mutation:\n    exclude:\n      - \"internal/secret/**\"\n",
			want:    "go.mutation.exclude requires a reason for production paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(t, tt.content)
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsEmptyExcludeReasons(t *testing.T) {
	_, err := loadConfig(t, "go:\n  exclude:\n    - pattern: \"internal/secret/**\"\n      reason: \"  \"\n")
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), "go.exclude reasons must not be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsMalformedExcludeEntries(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown key",
			content: "go:\n  exclude:\n    - pattern: \"fixtures/**\"\n      surprise: true\n",
			want:    "go.exclude[0].surprise is not supported",
		},
		{
			name:    "dry unknown key",
			content: "go:\n  dry:\n    exclude:\n      - pattern: \"fixtures/**\"\n        surprise: true\n",
			want:    "go.dry.exclude[0].surprise is not supported",
		},
		{
			name:    "mutation unknown key",
			content: "go:\n  mutation:\n    exclude:\n      - pattern: \"fixtures/**\"\n        surprise: true\n",
			want:    "go.mutation.exclude[0].surprise is not supported",
		},
		{
			name:    "not a sequence",
			content: "go:\n  exclude: true\n",
			want:    "go.exclude must be a sequence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(t, tt.content)
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestConventionalExcludePattern(t *testing.T) {
	conventional := []string{
		"**/*_test.go",
		"**/*.test.ts",
		"**/*.spec.ts",
		"tests/**",
		"fixtures/**",
		"templates/**",
		"testdata/**",
		"dist/**",
		"build/**",
		"coverage/**",
		"target/**",
		"node_modules/**",
		"vendor/**",
		"go/generated/**",
		"scripts/**",
	}
	for _, pattern := range conventional {
		if !conventionalExcludePattern(pattern) {
			t.Fatalf("conventionalExcludePattern(%q) = false", pattern)
		}
	}
	if conventionalExcludePattern("internal/secret/**") {
		t.Fatal("conventionalExcludePattern accepted a production pattern")
	}
}

func TestGoScopeConfiguredAndUnion(t *testing.T) {
	if (Config{}).GoScopeConfigured() {
		t.Fatal("GoScopeConfigured = true for empty config")
	}
	cfg := Config{Go: GoConfig{
		Targets:    []string{"go"},
		Exclude:    []string{"fixtures/**"},
		DRYPaths:   []string{"go/internal"},
		DRYExclude: []string{"**/*_test.go"},
	}}
	if !cfg.GoScopeConfigured() {
		t.Fatal("GoScopeConfigured = false with targets")
	}
	scopes, excludes := cfg.GoScopeUnion()
	if !reflect.DeepEqual(scopes, []string{"go", "go/internal"}) {
		t.Fatalf("scopes = %#v", scopes)
	}
	if !reflect.DeepEqual(excludes, []string{"fixtures/**", "**/*_test.go"}) {
		t.Fatalf("excludes = %#v", excludes)
	}

	dryOnly := Config{Go: GoConfig{DRYPaths: []string{"go/cmd"}}}
	if !dryOnly.GoScopeConfigured() {
		t.Fatal("GoScopeConfigured = false with dry paths")
	}
}

func TestLoadAllowsSharedGoTypeScriptAndRustConfig(t *testing.T) {
	_, err := Load(repo.NewSnapshot("/repo", map[string]repo.File{
		"slophammer.yml": {
			Path: "slophammer.yml",
			Content: `go:
  coverage:
    threshold: 85
typescript:
  coverage:
    threshold: 85
  complexity:
    max: 8
  dry:
    copied_blocks:
      enabled: true
      min_tokens: 100
python:
  coverage:
    threshold: 85
    paths:
      - python/src
  complexity:
    max: 8
  dry:
    max_findings: 0
    paths:
      - python/src
    copied_blocks:
      enabled: true
      min_tokens: 100
  mutation:
    targets:
      - python/src
  dependency_boundaries:
    - from: python/src
      allow: []
  typecheck:
    demotions:
      - rule: deprecated
        reason: upstream false positive on decorators
rust:
  coverage:
    threshold: 85
    paths:
      - rust/crates
  complexity:
    cognitive_max: 8
  targets:
    - rust/crates
  exclude:
    - rust/target/**
  dry:
    max_findings: 0
    paths:
      - rust/crates
    copied_blocks:
      enabled: true
      min_tokens: 100
  unsafe:
    policy: forbid
    allow:
      - path: src/lib.rs
        reason: reviewed
  mutation:
    targets:
      - rust/crates/slophammer-cli/src/rust_rules
  dependency_boundaries:
    - from: rust/crates/slophammer-cli
      allow: []
`,
		},
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
}
