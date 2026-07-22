package rules

import (
	"context"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

// conventionalGoPathSegments is the path-level form of the conventional
// non-production list in specs/CONFIG.md.
var conventionalGoPathSegments = map[string]struct{}{
	"tests":        {},
	"fixtures":     {},
	"templates":    {},
	"testdata":     {},
	"dist":         {},
	"build":        {},
	"coverage":     {},
	"target":       {},
	"node_modules": {},
	"vendor":       {},
	"scripts":      {},
	"benches":      {},
}

// goScopeRule enforces that configured scope accounts for every production
// Go file: each one is either inside a configured scope or covered by a
// conventional or reasoned exclude, so narrowing scope cannot hide code.
type goScopeRule struct {
	definition Definition
}

func newGoScopeRule(definition Definition) Rule {
	return goScopeRule{definition: definition}
}

func (r goScopeRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r goScopeRule) Check(context.Context, repo.Snapshot) []Finding {
	return nil
}

func (r goScopeRule) CheckWithConfig(_ context.Context, snapshot repo.Snapshot, cfg config.Config) []Finding {
	if !cfg.GoScopeConfigured() {
		return nil
	}
	uncovered := uncoveredProductionGoDirs(snapshot, cfg)
	if len(uncovered) == 0 {
		return nil
	}
	return []Finding{{
		RuleID:   r.definition.ID,
		Severity: r.definition.Severity,
		Path:     r.definition.Path,
		Message:  r.definition.Message + ": " + strings.Join(uncovered, ", "),
	}}
}

// GoScopeCoverage counts production files the configured scope covers versus
// all production files. Nil when no scope is configured.
func GoScopeCoverage(snapshot repo.Snapshot, cfg config.Config) *ScopeCoverage {
	if !cfg.GoScopeConfigured() {
		return nil
	}
	scopes, _ := cfg.GoScopeUnion()
	production := productionGoFiles(snapshot)
	scanned := 0
	for _, filePath := range production {
		if inScopeTargets(filePath, scopes) {
			scanned++
		}
	}
	return &ScopeCoverage{Scanned: scanned, ProductionFiles: len(production)}
}

func uncoveredProductionGoDirs(snapshot repo.Snapshot, cfg config.Config) []string {
	scopes, excludes := cfg.GoScopeUnion()
	dirs := map[string]struct{}{}
	for _, filePath := range productionGoFiles(snapshot) {
		if inScopeTargets(filePath, scopes) || matchesAnyGlob(filePath, excludes) {
			continue
		}
		dirs[parentDir(filePath)] = struct{}{}
	}
	return sortedRootSet(dirs)
}

func productionGoFiles(snapshot repo.Snapshot) []string {
	files := make([]string, 0)
	for filePath := range snapshot.Files {
		if strings.HasSuffix(filePath, ".go") && !conventionalGoPath(filePath) {
			files = append(files, filePath)
		}
	}
	sort.Strings(files)
	return files
}

func conventionalGoPath(filePath string) bool {
	if strings.HasSuffix(filePath, "_test.go") || strings.Contains(filePath, "generated") {
		return true
	}
	return pathHasAnySegment(filePath, conventionalGoPathSegments)
}

func pathHasAnySegment(filePath string, segments map[string]struct{}) bool {
	for _, segment := range strings.Split(strings.ReplaceAll(filePath, "\\", "/"), "/") {
		if _, ok := segments[segment]; ok {
			return true
		}
	}
	return false
}

func inScopeTargets(filePath string, targets []string) bool {
	for _, target := range targets {
		normalized := strings.TrimRight(target, "/")
		if normalized == "." || pathHasPrefix(filePath, normalized) {
			return true
		}
	}
	return false
}

func matchesAnyGlob(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := doublestar.Match(pattern, filePath); err == nil && matched {
			return true
		}
	}
	return false
}

func parentDir(filePath string) string {
	if index := strings.LastIndex(filePath, "/"); index >= 0 {
		return filePath[:index]
	}
	return "."
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-12T22:50:25+08:00","module_hash":"ffe9d3b77d4332b0fa504bbd835175653b60bc34d2d363d5e8e262e190c5f112","functions":[{"id":"func/newGoScopeRule","name":"newGoScopeRule","line":37,"end_line":39,"hash":"68da0ecae4c831461908c945e6f6b0dae9223b7c14b827ade87b2c7718ce57e2"},{"id":"func/goScopeRule.Metadata","name":"goScopeRule.Metadata","line":41,"end_line":43,"hash":"7339708f26482ab352d658d39b2029cadac5a803748fbf83bf50171026256a4d"},{"id":"func/goScopeRule.Check","name":"goScopeRule.Check","line":45,"end_line":47,"hash":"1c029eb91f635bf51cd59e8a6a142808af2ad70cd9a598df6c93ebda446e619e"},{"id":"func/goScopeRule.CheckWithConfig","name":"goScopeRule.CheckWithConfig","line":49,"end_line":63,"hash":"3ad7f82c6c46f38be37bf847552454ecb09464f45f48278cd51b3d8be7da36af"},{"id":"func/GoScopeCoverage","name":"GoScopeCoverage","line":67,"end_line":80,"hash":"631dddcfc9bdb4ab35aa4f0746a8bdcc9698da6b6f8faae576ea44b7038128a4"},{"id":"func/uncoveredProductionGoDirs","name":"uncoveredProductionGoDirs","line":82,"end_line":92,"hash":"a7ce9c9f87865a19bdb05beb58b83c184205d648bf33c3a61b6128df7502add6"},{"id":"func/productionGoFiles","name":"productionGoFiles","line":94,"end_line":103,"hash":"6546b7542febfc5bd352cb77ccd4956f64704a884c673bf861963b434abf7114"},{"id":"func/conventionalGoPath","name":"conventionalGoPath","line":105,"end_line":110,"hash":"35316ec87b3f1bc7bdb49d98d3e3816ed615f2a42154102e4d3e4c62e63ccbbf"},{"id":"func/pathHasAnySegment","name":"pathHasAnySegment","line":112,"end_line":119,"hash":"6cec91d1ac55ae696c68e0abb175b075259f6b5d3a79497a2494b5514726a9ed"},{"id":"func/inScopeTargets","name":"inScopeTargets","line":121,"end_line":129,"hash":"7f0bb60556e9de8e1d4520b3e4bd2ccd7b23048687f962bece8a75e8c101d1ad"},{"id":"func/matchesAnyGlob","name":"matchesAnyGlob","line":131,"end_line":138,"hash":"78e89b54bef8ab750cd06a2697ca8e99749455c0656da86ed289ac622bec1c3f"},{"id":"func/parentDir","name":"parentDir","line":140,"end_line":145,"hash":"4e970f77be2547564426b070fb97519bbe9a7e2360b94141a0fc507101036ee8"}]}
// mutate4go-manifest-end
