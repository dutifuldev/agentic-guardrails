package config

import (
	"fmt"
	"path"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
	"gopkg.in/yaml.v3"
)

const (
	DefaultFileName = "slophammer.yml"
	AltFileName     = "slophammer.yaml"

	MinimumGoCoverageThreshold = 85
	MaximumGoCRAPScore         = 8
)

type Config struct {
	SourceDir string                `yaml:"-"`
	Rules     map[string]RuleConfig `yaml:"rules"`
	Go        GoConfig              `yaml:"go"`
}

type RuleConfig struct {
	Severity  string  `yaml:"severity"`
	Disabled  bool    `yaml:"disabled"`
	Reason    string  `yaml:"reason"`
	Threshold float64 `yaml:"threshold"`
	Max       float64 `yaml:"max"`
}

type GoConfig struct {
	CoverageThreshold      float64              `yaml:"-"`
	CoverageProfile        string               `yaml:"-"`
	Targets                []string             `yaml:"targets"`
	Exclude                []string             `yaml:"exclude"`
	ExcludeEntries         []ExcludeEntry       `yaml:"-"`
	DRYMaxCandidates       int                  `yaml:"-"`
	DRYMaxCandidatesSet    bool                 `yaml:"-"`
	DRYPaths               []string             `yaml:"-"`
	DRYExclude             []string             `yaml:"-"`
	DRYExcludeEntries      []ExcludeEntry       `yaml:"-"`
	DRY                    DryConfig            `yaml:"dry"`
	CRAPMaxScore           float64              `yaml:"-"`
	Mutation               MutationConfig       `yaml:"mutation"`
	MutationExcludeEntries []ExcludeEntry       `yaml:"-"`
	DependencyBoundaries   []DependencyBoundary `yaml:"dependency_boundaries"`
}

// ExcludeEntry is one exclude list item: a plain pattern when it matches the
// conventional non-production list, and a pattern with a reason when it
// carves out production files.
type ExcludeEntry struct {
	Pattern  string
	Reason   string
	Reasoned bool
}

func (e *ExcludeEntry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		e.Pattern = value.Value
		return nil
	}
	var parsed struct {
		Pattern string `yaml:"pattern"`
		Reason  string `yaml:"reason"`
	}
	if err := value.Decode(&parsed); err != nil {
		return err
	}
	e.Pattern = parsed.Pattern
	e.Reason = parsed.Reason
	e.Reasoned = true
	return nil
}

type MutationConfig struct {
	Targets []string `yaml:"targets"`
	Exclude []string `yaml:"exclude"`
}

type DryConfig struct {
	MaxFindings    int                 `yaml:"max_findings"`
	MaxFindingsSet bool                `yaml:"-"`
	Paths          []string            `yaml:"paths"`
	Exclude        []string            `yaml:"exclude"`
	Structural     DryStructuralConfig `yaml:"structural"`
	CopiedBlocks   DryCopiedConfig     `yaml:"copied_blocks"`
}

type DryStructuralConfig struct {
	EnabledSet bool    `yaml:"-"`
	Enabled    bool    `yaml:"enabled"`
	Threshold  float64 `yaml:"threshold"`
	MinLines   int     `yaml:"min_lines"`
	MinNodes   int     `yaml:"min_nodes"`
}

type DryCopiedConfig struct {
	EnabledSet bool `yaml:"-"`
	Enabled    bool `yaml:"enabled"`
	MinTokens  int  `yaml:"min_tokens"`
}

type DependencyBoundary struct {
	From  string   `yaml:"from"`
	Allow []string `yaml:"allow"`
}

func (cfg *GoConfig) UnmarshalYAML(value *yaml.Node) error {
	type dryStructuralConfig struct {
		Enabled   *bool   `yaml:"enabled"`
		Threshold float64 `yaml:"threshold"`
		MinLines  int     `yaml:"min_lines"`
		MinNodes  int     `yaml:"min_nodes"`
	}
	type dryCopiedConfig struct {
		Enabled   *bool `yaml:"enabled"`
		MinTokens int   `yaml:"min_tokens"`
	}
	type dryConfig struct {
		MaxFindings  *int                `yaml:"max_findings"`
		Paths        []string            `yaml:"paths"`
		Exclude      []ExcludeEntry      `yaml:"exclude"`
		Structural   dryStructuralConfig `yaml:"structural"`
		CopiedBlocks dryCopiedConfig     `yaml:"copied_blocks"`
	}
	type goCoverageConfig struct {
		Threshold float64 `yaml:"threshold"`
		Profile   string  `yaml:"profile"`
	}
	type goCRAPConfig struct {
		MaxScore float64 `yaml:"max_score"`
	}
	type mutationConfig struct {
		Targets []string       `yaml:"targets"`
		Exclude []ExcludeEntry `yaml:"exclude"`
	}
	type goConfig struct {
		Coverage             goCoverageConfig     `yaml:"coverage"`
		Targets              []string             `yaml:"targets"`
		Exclude              []ExcludeEntry       `yaml:"exclude"`
		DRY                  dryConfig            `yaml:"dry"`
		CRAP                 goCRAPConfig         `yaml:"crap"`
		Mutation             mutationConfig       `yaml:"mutation"`
		DependencyBoundaries []DependencyBoundary `yaml:"dependency_boundaries"`
	}
	var parsed goConfig
	if err := value.Decode(&parsed); err != nil {
		return err
	}
	cfg.CoverageThreshold = parsed.Coverage.Threshold
	cfg.CoverageProfile = parsed.Coverage.Profile
	cfg.Targets = parsed.Targets
	cfg.ExcludeEntries = parsed.Exclude
	cfg.Exclude = excludePatterns(parsed.Exclude)
	if parsed.DRY.MaxFindings != nil {
		cfg.DRY.MaxFindings = *parsed.DRY.MaxFindings
		cfg.DRY.MaxFindingsSet = true
		cfg.DRYMaxCandidates = *parsed.DRY.MaxFindings
		cfg.DRYMaxCandidatesSet = true
	}
	if len(parsed.DRY.Paths) > 0 {
		cfg.DRY.Paths = parsed.DRY.Paths
		cfg.DRYPaths = parsed.DRY.Paths
	}
	if len(parsed.DRY.Exclude) > 0 {
		cfg.DRYExcludeEntries = parsed.DRY.Exclude
		cfg.DRY.Exclude = excludePatterns(parsed.DRY.Exclude)
		cfg.DRYExclude = cfg.DRY.Exclude
	}
	cfg.DRY.Structural.Threshold = parsed.DRY.Structural.Threshold
	cfg.DRY.Structural.MinLines = parsed.DRY.Structural.MinLines
	cfg.DRY.Structural.MinNodes = parsed.DRY.Structural.MinNodes
	if parsed.DRY.Structural.Enabled != nil {
		cfg.DRY.Structural.Enabled = *parsed.DRY.Structural.Enabled
		cfg.DRY.Structural.EnabledSet = true
	}
	cfg.DRY.CopiedBlocks.MinTokens = parsed.DRY.CopiedBlocks.MinTokens
	if parsed.DRY.CopiedBlocks.Enabled != nil {
		cfg.DRY.CopiedBlocks.Enabled = *parsed.DRY.CopiedBlocks.Enabled
		cfg.DRY.CopiedBlocks.EnabledSet = true
	}
	cfg.CRAPMaxScore = parsed.CRAP.MaxScore
	cfg.Mutation = MutationConfig{
		Targets: parsed.Mutation.Targets,
		Exclude: excludePatterns(parsed.Mutation.Exclude),
	}
	cfg.MutationExcludeEntries = parsed.Mutation.Exclude
	cfg.DependencyBoundaries = parsed.DependencyBoundaries
	return nil
}

func excludePatterns(entries []ExcludeEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	patterns := make([]string, 0, len(entries))
	for _, entry := range entries {
		patterns = append(patterns, entry.Pattern)
	}
	return patterns
}

func Load(snapshot repo.Snapshot) (Config, error) {
	file, ok := configFile(snapshot)
	if !ok {
		return Config{}, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(file.Content), &root); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file.Path, err)
	}
	if err := validateKnownKeys(&root); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file.Path, err)
	}
	var cfg Config
	if err := root.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file.Path, err)
	}
	cfg.SourceDir = configSourceDir(file.Path)
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file.Path, err)
	}
	return cfg, nil
}

func (cfg Config) GoMutationScope() ([]string, []string) {
	targets := cfg.Go.Targets
	exclude := cfg.Go.Exclude
	if len(cfg.Go.Mutation.Targets) > 0 {
		targets = cfg.Go.Mutation.Targets
		exclude = cfg.Go.Mutation.Exclude
	} else if len(cfg.Go.Mutation.Exclude) > 0 {
		exclude = cfg.Go.Mutation.Exclude
	}
	return scopedConfigPaths(cfg.SourceDir, targets), scopedConfigExcludePaths(cfg.SourceDir, exclude)
}

func (cfg Config) GoDRYScope() ([]string, []string) {
	paths := cfg.Go.DRYPaths
	exclude := cfg.Go.DRYExclude
	if len(paths) == 0 {
		paths = cfg.Go.Targets
	}
	if len(exclude) == 0 {
		exclude = cfg.Go.Exclude
	}
	return scopedConfigPaths(cfg.SourceDir, paths), scopedConfigExcludePaths(cfg.SourceDir, exclude)
}

func (cfg Config) GoScope() ([]string, []string) {
	return scopedConfigPaths(cfg.SourceDir, cfg.Go.Targets), scopedConfigExcludePaths(cfg.SourceDir, cfg.Go.Exclude)
}

// GoScopeConfigured reports whether any Go check scope is configured, which
// is what arms scope-completeness checking and coverage counts.
func (cfg Config) GoScopeConfigured() bool {
	return len(cfg.Go.Targets) > 0 || len(cfg.Go.DRYPaths) > 0 || len(cfg.Go.Mutation.Targets) > 0
}

// GoScopeUnion returns the union of every configured Go scope and every
// configured exclude pattern, for scope-completeness checking. Mutation
// targets participate so narrowing mutation scope stays visible.
func (cfg Config) GoScopeUnion() ([]string, []string) {
	targets, exclude := cfg.GoScope()
	dryPaths, dryExclude := cfg.GoDRYScope()
	mutationTargets, mutationExclude := cfg.GoMutationScope()
	union := append(targets, dryPaths...)
	union = append(union, mutationTargets...)
	excludes := append(exclude, dryExclude...)
	excludes = append(excludes, mutationExclude...)
	return dedupeStrings(union), dedupeStrings(excludes)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func (cfg Config) GoCoverageProfile() string {
	return scopedConfigPath(cfg.SourceDir, cfg.Go.CoverageProfile)
}

func configFile(snapshot repo.Snapshot) (repo.File, bool) {
	for _, name := range []string{DefaultFileName, AltFileName} {
		for filePath, file := range snapshot.Files {
			if filePath == name {
				return file, true
			}
		}
	}
	for _, file := range snapshot.FilesNamedFold(DefaultFileName, AltFileName) {
		return file, true
	}
	return repo.File{}, false
}

func configSourceDir(filePath string) string {
	dir := path.Dir(strings.ReplaceAll(filePath, "\\", "/"))
	if dir == "." || dir == "/" {
		return "."
	}
	return strings.TrimPrefix(dir, "./")
}

func scopedConfigPaths(sourceDir string, values []string) []string {
	scoped := make([]string, 0, len(values))
	for _, value := range values {
		if sourceDir == "" || sourceDir == "." {
			scoped = append(scoped, value)
			continue
		}
		scoped = append(scoped, path.Join(sourceDir, value))
	}
	return scoped
}

func scopedConfigPath(sourceDir string, value string) string {
	if value == "" || sourceDir == "" || sourceDir == "." || isAbsoluteConfigPath(value) {
		return value
	}
	return path.Join(sourceDir, value)
}

func isAbsoluteConfigPath(value string) bool {
	return path.IsAbs(value) || isWindowsAbsoluteConfigPath(value)
}

func isWindowsAbsoluteConfigPath(value string) bool {
	return strings.HasPrefix(value, `\\`) || isWindowsDriveAbsolutePath(value)
}

func isWindowsDriveAbsolutePath(value string) bool {
	if len(value) < 3 {
		return false
	}
	return isASCIIAlpha(value[0]) && value[1] == ':' && isWindowsPathSeparator(value[2])
}

func isASCIIAlpha(value byte) bool {
	return (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

func isWindowsPathSeparator(value byte) bool {
	return value == '\\' || value == '/'
}

func scopedConfigExcludePaths(sourceDir string, values []string) []string {
	scoped := make([]string, 0, len(values))
	for _, value := range values {
		if sourceDir == "" || sourceDir == "." || !strings.Contains(value, "/") {
			scoped = append(scoped, value)
			continue
		}
		scoped = append(scoped, value)
		scoped = append(scoped, path.Join(sourceDir, value))
	}
	return scoped
}

func validateKnownKeys(root *yaml.Node) error {
	node, ok, err := documentMapping(root, "root")
	if err != nil || !ok {
		return err
	}
	return validateMappingKeys(node, "root", set("rules", "go", "typescript", "rust", "python"), validateTopLevelSection)
}

func validateTopLevelSection(key string, value *yaml.Node) error {
	switch key {
	case "rules":
		return validateRulesKeys(value)
	case "go":
		return validateGoKeys(value)
	case "typescript":
		return validateTypeScriptKeys(value)
	case "rust":
		return validateRustKeys(value)
	case "python":
		return validatePythonKeys(value)
	default:
		return nil
	}
}

func validatePythonKeys(node *yaml.Node) error {
	return validateMappingKeys(
		node,
		"python",
		set("coverage", "complexity", "dry", "mutation", "dependency_boundaries", "typecheck"),
		validatePythonSection,
	)
}

func validatePythonSection(key string, value *yaml.Node) error {
	switch key {
	case "coverage":
		return validateSectionSequenceKey(value, "python.coverage", set("threshold", "paths", "exclude"), "exclude", validateExcludeEntryKeys)
	case "complexity":
		return validateMappingKeys(value, "python.complexity", set("max"), nil)
	case "mutation":
		return validateSectionSequenceKey(value, "python.mutation", set("targets", "exclude"), "exclude", validateExcludeEntryKeys)
	case "dry":
		return validateCopiedBlockDryKeys(value, "python.dry")
	case "dependency_boundaries":
		return validateDependencyBoundaryKeys(value, "python.dependency_boundaries")
	case "typecheck":
		return validatePythonTypecheckKeys(value)
	default:
		return nil
	}
}

// validatePythonTypecheckKeys validates reasoned ty rule demotions: each
// entry names the demoted rule and the reason the demotion is justified.
func validatePythonTypecheckKeys(node *yaml.Node) error {
	return validateMappingKeys(node, "python.typecheck", set("demotions"), func(_ string, value *yaml.Node) error {
		return validateMappingSequenceKeys(value, "python.typecheck.demotions", set("rule", "reason"))
	})
}

func validateRulesKeys(node *yaml.Node) error {
	return validateMappingKeys(node, "rules", nil, func(ruleID string, value *yaml.Node) error {
		return validateMappingKeys(value, "rules."+ruleID, set("severity", "disabled", "reason", "threshold", "max"), nil)
	})
}

func validateGoKeys(node *yaml.Node) error {
	return validateMappingKeys(
		node,
		"go",
		set(
			"coverage",
			"targets",
			"exclude",
			"dry",
			"crap",
			"mutation",
			"dependency_boundaries",
		),
		func(key string, value *yaml.Node) error {
			switch key {
			case "coverage":
				return validateMappingKeys(value, "go.coverage", set("threshold", "profile"), nil)
			case "exclude":
				return validateExcludeEntryKeys(value, "go.exclude")
			case "dry":
				return validateGoDryKeys(value)
			case "crap":
				return validateMappingKeys(value, "go.crap", set("max_score"), nil)
			case "mutation":
				return validateGoMutationKeys(value)
			case "dependency_boundaries":
				return validateDependencyBoundaryKeys(value, "go.dependency_boundaries")
			default:
				return nil
			}
		},
	)
}

func validateGoMutationKeys(node *yaml.Node) error {
	return validateSectionSequenceKey(node, "go.mutation", set("targets", "exclude"), "exclude", validateExcludeEntryKeys)
}

// validateSectionSequenceKey validates a mapping section whose keys are
// limited to allowed and whose sequenceKey value gets its own validator.
func validateSectionSequenceKey(
	node *yaml.Node,
	field string,
	allowed map[string]struct{},
	sequenceKey string,
	validateSequence func(*yaml.Node, string) error,
) error {
	return validateMappingKeys(node, field, allowed, func(key string, value *yaml.Node) error {
		if key == sequenceKey {
			return validateSequence(value, field+"."+sequenceKey)
		}
		return nil
	})
}

func validateGoDryKeys(node *yaml.Node) error {
	return validateMappingKeys(
		node,
		"go.dry",
		set("max_findings", "paths", "exclude", "structural", "copied_blocks"),
		func(key string, value *yaml.Node) error {
			switch key {
			case "exclude":
				return validateExcludeEntryKeys(value, "go.dry.exclude")
			case "structural":
				return validateMappingKeys(value, "go.dry.structural", set("enabled", "threshold", "min_lines", "min_nodes"), nil)
			case "copied_blocks":
				return validateMappingKeys(value, "go.dry.copied_blocks", set("enabled", "min_tokens"), nil)
			default:
				return nil
			}
		},
	)
}

func validateTypeScriptKeys(node *yaml.Node) error {
	return validateMappingKeys(
		node,
		"typescript",
		set("coverage", "complexity", "dry", "mutation", "dependency_boundaries"),
		validateTypeScriptSection,
	)
}

func validateTypeScriptSection(key string, value *yaml.Node) error {
	switch key {
	case "coverage":
		return validateMappingKeys(value, "typescript.coverage", set("threshold", "paths", "exclude"), nil)
	case "complexity":
		return validateMappingKeys(value, "typescript.complexity", set("max"), nil)
	case "mutation":
		return validateMappingKeys(value, "typescript.mutation", set("targets"), nil)
	case "dry":
		return validateCopiedBlockDryKeys(value, "typescript.dry")
	case "dependency_boundaries":
		return validateDependencyBoundaryKeys(value, "typescript.dependency_boundaries")
	default:
		return nil
	}
}

func validateCopiedBlockDryKeys(node *yaml.Node, field string) error {
	return validateMappingKeys(
		node,
		field,
		set("max_findings", "paths", "exclude", "copied_blocks"),
		func(key string, value *yaml.Node) error {
			if key == "copied_blocks" {
				return validateMappingKeys(value, field+".copied_blocks", set("enabled", "min_tokens"), nil)
			}
			if key == "exclude" {
				return validateExcludeEntryKeys(value, field+".exclude")
			}
			return nil
		},
	)
}

func validateRustKeys(node *yaml.Node) error {
	return validateMappingKeys(
		node,
		"rust",
		set(
			"coverage",
			"complexity",
			"targets",
			"exclude",
			"dry",
			"unsafe",
			"mutation",
			"dependency_boundaries",
		),
		func(key string, value *yaml.Node) error {
			switch key {
			case "coverage":
				return validateMappingKeys(value, "rust.coverage", set("threshold", "paths", "exclude"), nil)
			case "complexity":
				return validateMappingKeys(value, "rust.complexity", set("cognitive_max"), nil)
			case "dry":
				return validateCopiedBlockDryKeys(value, "rust.dry")
			case "unsafe":
				return validateRustUnsafeKeys(value)
			case "mutation":
				return validateMappingKeys(value, "rust.mutation", set("targets", "exclude"), nil)
			case "dependency_boundaries":
				return validateDependencyBoundaryKeys(value, "rust.dependency_boundaries")
			default:
				return nil
			}
		},
	)
}

func validateRustUnsafeKeys(node *yaml.Node) error {
	return validateSectionSequenceKey(node, "rust.unsafe", set("policy", "allow"), "allow", validateUnsafeAllowKeys)
}

func validateUnsafeAllowKeys(node *yaml.Node, field string) error {
	return validateMappingSequenceKeys(node, field, set("path", "reason"))
}

func validateDependencyBoundaryKeys(node *yaml.Node, field string) error {
	return validateMappingSequenceKeys(node, field, set("from", "allow"))
}

func validateMappingSequenceKeys(node *yaml.Node, field string, allowed map[string]struct{}) error {
	return validateSequenceItems(node, field, func(item *yaml.Node, itemField string) error {
		return validateMappingKeys(item, itemField, allowed, nil)
	})
}

// validateExcludeEntryKeys accepts plain pattern strings and strict
// {pattern, reason} mappings, the two exclude entry shapes.
func validateExcludeEntryKeys(node *yaml.Node, field string) error {
	return validateSequenceItems(node, field, func(item *yaml.Node, itemField string) error {
		if item.Kind == yaml.ScalarNode {
			return nil
		}
		return validateMappingKeys(item, itemField, set("pattern", "reason"), nil)
	})
}

func validateSequenceItems(node *yaml.Node, field string, validateItem func(*yaml.Node, string) error) error {
	if node.Kind == 0 || node.Tag == "!!null" {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s must be a sequence", field)
	}
	for i, item := range node.Content {
		if err := validateItem(item, fmt.Sprintf("%s[%d]", field, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateMappingKeys(
	node *yaml.Node,
	field string,
	allowed map[string]struct{},
	visit func(string, *yaml.Node) error,
) error {
	if node.Kind == 0 || node.Tag == "!!null" {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a mapping", field)
	}
	seen := map[string]struct{}{}
	for i := 0; i < len(node.Content); i += 2 {
		if err := validateMappingEntry(field, allowed, visit, seen, node.Content[i], node.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func validateMappingEntry(
	field string,
	allowed map[string]struct{},
	visit func(string, *yaml.Node) error,
	seen map[string]struct{},
	keyNode *yaml.Node,
	valueNode *yaml.Node,
) error {
	key, err := mappingKey(field, keyNode)
	if err != nil {
		return err
	}
	if _, ok := seen[key]; ok {
		return fmt.Errorf("%s.%s is duplicated", field, key)
	}
	seen[key] = struct{}{}
	if err := validateAllowedKey(field, allowed, key); err != nil {
		return err
	}
	if visit == nil {
		return nil
	}
	return visit(key, valueNode)
}

func mappingKey(field string, node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Value == "" {
		return "", fmt.Errorf("%s contains an invalid key", field)
	}
	return node.Value, nil
}

func validateAllowedKey(field string, allowed map[string]struct{}, key string) error {
	if allowed == nil {
		return nil
	}
	if _, ok := allowed[key]; ok {
		return nil
	}
	return fmt.Errorf("%s.%s is not supported", field, key)
}

func documentMapping(root *yaml.Node, field string) (*yaml.Node, bool, error) {
	node := unwrapDocumentNode(root)
	if emptyYAMLNode(node) {
		return nil, false, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, false, fmt.Errorf("%s must be a mapping", field)
	}
	return node, true, nil
}

func unwrapDocumentNode(root *yaml.Node) *yaml.Node {
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		return root.Content[0]
	}
	return root
}

func emptyYAMLNode(node *yaml.Node) bool {
	return node.Kind == 0 || node.Tag == "!!null"
}

func set(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func validate(cfg Config) error {
	for ruleID, rule := range cfg.Rules {
		if err := validateRule(ruleID, rule); err != nil {
			return err
		}
	}
	for i, boundary := range cfg.Go.DependencyBoundaries {
		if err := validateDependencyBoundary(i, boundary); err != nil {
			return err
		}
	}
	if err := validateGoExcludeReasons(cfg.Go); err != nil {
		return err
	}
	return validateGoTargets(cfg.Go)
}

// validateGoExcludeReasons enforces the scope-completeness contract: a
// string exclude may only name conventional non-production patterns, and
// excludes that carve out production files must carry a non-empty reason.
func validateGoExcludeReasons(cfg GoConfig) error {
	sections := []struct {
		name    string
		entries []ExcludeEntry
	}{
		{"go.exclude", cfg.ExcludeEntries},
		{"go.dry.exclude", cfg.DRYExcludeEntries},
		{"go.mutation.exclude", cfg.MutationExcludeEntries},
	}
	for _, section := range sections {
		if err := validateExcludeEntries(section.name, section.entries); err != nil {
			return err
		}
	}
	return nil
}

func validateExcludeEntries(section string, entries []ExcludeEntry) error {
	for _, entry := range entries {
		if !entry.Reasoned && !conventionalExcludePattern(entry.Pattern) {
			return fmt.Errorf("%s requires a reason for production paths", section)
		}
		if entry.Reasoned && strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("%s reasons must not be empty", section)
		}
	}
	return nil
}

// conventionalExcludePattern reports whether a pattern names only the
// conventional non-production list from specs/CONFIG.md, which scope may
// exclude without a reason.
func conventionalExcludePattern(pattern string) bool {
	markers := []string{
		"_test.",
		".test.",
		".spec.",
		"tests/",
		"fixtures/",
		"templates/",
		"testdata/",
		"dist/",
		"build/",
		"coverage/",
		"target/",
		"node_modules/",
		"vendor/",
		"generated",
		"scripts/",
	}
	for _, marker := range markers {
		if strings.Contains(pattern, marker) {
			return true
		}
	}
	return false
}

func validateGoTargets(cfg GoConfig) error {
	if err := validateDryBudgets(cfg); err != nil {
		return err
	}
	if err := validateDryEngineTargets(cfg.DRY); err != nil {
		return err
	}
	return validateGoThresholds(cfg)
}

func validateDryBudgets(cfg GoConfig) error {
	if cfg.DRY.MaxFindingsSet && cfg.DRY.MaxFindings < 0 {
		return fmt.Errorf("go.dry.max_findings must be non-negative")
	}
	return nil
}

func validateGoThresholds(cfg GoConfig) error {
	if cfg.CoverageThreshold > 0 && cfg.CoverageThreshold < MinimumGoCoverageThreshold {
		return fmt.Errorf("go.coverage.threshold must be at least %.1f", float64(MinimumGoCoverageThreshold))
	}
	if cfg.CRAPMaxScore > 0 && cfg.CRAPMaxScore > MaximumGoCRAPScore {
		return fmt.Errorf("go.crap.max_score must be at most %.1f", float64(MaximumGoCRAPScore))
	}
	return nil
}

func validateDryEngineTargets(cfg DryConfig) error {
	if cfg.Structural.Threshold < 0 || cfg.Structural.Threshold > 1 {
		return fmt.Errorf("go.dry.structural.threshold must be between 0 and 1")
	}
	if cfg.Structural.MinLines < 0 {
		return fmt.Errorf("go.dry.structural.min_lines must be non-negative")
	}
	if cfg.Structural.MinNodes < 0 {
		return fmt.Errorf("go.dry.structural.min_nodes must be non-negative")
	}
	if cfg.CopiedBlocks.MinTokens < 0 {
		return fmt.Errorf("go.dry.copied_blocks.min_tokens must be non-negative")
	}
	return nil
}

func validateRule(ruleID string, rule RuleConfig) error {
	switch rule.Severity {
	case "", "error", "warn":
	default:
		return fmt.Errorf("rules.%s.severity must be error or warn", ruleID)
	}
	if rule.Disabled && strings.TrimSpace(rule.Reason) == "" {
		return fmt.Errorf("rules.%s.reason is required when disabled is true", ruleID)
	}
	return nil
}

func validateDependencyBoundary(index int, boundary DependencyBoundary) error {
	if boundary.From == "" {
		return fmt.Errorf("go.dependency_boundaries[%d].from cannot be empty", index)
	}
	return nil
}
