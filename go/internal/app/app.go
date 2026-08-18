package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/osolmaz/slophammer/go/internal/agents"
	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/gotargets"
	"github.com/osolmaz/slophammer/go/internal/repo"
	"github.com/osolmaz/slophammer/go/internal/report"
	"github.com/osolmaz/slophammer/go/internal/rules"
	"github.com/osolmaz/slophammer/go/internal/scan"
	"github.com/osolmaz/slophammer/go/internal/toolchecks"
)

const (
	ExitOK       = 0
	ExitFindings = 1
	ExitError    = 2
)

type CheckOptions struct {
	Root            string
	Format          string
	Execute         bool
	CoverageProfile string
	OnlyRuleIDs     []string
	Baseline        BaselineMode
}

type RulesOptions struct {
	Format string
}

type AgentsInitOptions struct {
	Root   string
	DryRun bool
	Force  bool
}

func AgentsCheck(ctx context.Context, options CheckOptions, out io.Writer, errOut io.Writer) int {
	options.OnlyRuleIDs = append([]string(nil), agents.RuleIDs...)
	return Check(ctx, options, out, errOut)
}

func AgentsInit(_ context.Context, options AgentsInitOptions, out io.Writer, errOut io.Writer) int {
	snapshot, err := scan.Repo(options.Root)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "agents init failed: %v\n", err)
		return ExitError
	}
	if agents.HasUnsafePackagePath(snapshot) {
		_, _ = fmt.Fprintln(errOut, "agents init failed: package paths containing newlines are not supported")
		return ExitError
	}
	if agents.HasReservedMarkerPackagePath(snapshot) {
		_, _ = fmt.Fprintln(errOut, "agents init failed: package paths containing Slophammer evidence markers are not supported")
		return ExitError
	}
	content := agents.Render(snapshot)
	if options.DryRun {
		_, _ = io.WriteString(out, content)
		return ExitOK
	}
	targetName := "AGENTS.md"
	if existing, ok := agents.RootFile(snapshot); ok {
		targetName = existing.Path
	}
	target := filepath.Join(snapshot.Root, filepath.FromSlash(targetName))
	if err := writeAgentsFile(target, content, options.Force); err != nil {
		writeAgentsError(errOut, err)
		return ExitError
	}
	_, _ = fmt.Fprintln(out, "created AGENTS.md")
	return ExitOK
}

func writeAgentsError(errOut io.Writer, err error) {
	if os.IsExist(err) {
		_, _ = fmt.Fprintln(errOut, "agents init failed: AGENTS.md already exists; pass --force to replace it")
		return
	}
	_, _ = fmt.Fprintf(errOut, "agents init failed: %v\n", err)
}

func writeAgentsFile(target, content string, force bool) error {
	if force {
		return replaceAgentsFile(target, content)
	}
	// #nosec G302,G304 -- target is the scanned repository root, and AGENTS.md must be readable by collaborators.
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func replaceAgentsFile(target, content string) error {
	if err := rejectAgentsSymlink(target); err != nil {
		return err
	}
	temporaryPath, err := newAgentsTemporary(target)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := writeAgentsTemporary(temporaryPath, content); err != nil {
		return err
	}
	return os.Rename(temporaryPath, target)
}

func rejectAgentsSymlink(target string) error {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("AGENTS.md is a symlink; refusing forced replacement")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("AGENTS.md is not a regular file; refusing forced replacement")
	}
	return nil
}

func newAgentsTemporary(target string) (string, error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".slophammer-agents-*")
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func writeAgentsTemporary(target, content string) error {
	// #nosec G302 -- AGENTS.md is a repository document and must be readable by collaborators.
	if err := os.Chmod(target, 0o644); err != nil {
		return err
	}
	// #nosec G306 -- the target is a newly created temporary repository document.
	return os.WriteFile(target, []byte(content), 0o644)
}

func Check(ctx context.Context, options CheckOptions, out io.Writer, errOut io.Writer) int {
	return check(ctx, options, out, errOut, toolchecks.ExecRunner{})
}

func check(ctx context.Context, options CheckOptions, out io.Writer, errOut io.Writer, runner toolchecks.Runner) int {
	if err := validateOnlyRuleIDs(options.OnlyRuleIDs); err != nil {
		_, _ = fmt.Fprintf(errOut, "check failed: %v\n", err)
		return ExitError
	}
	snapshot, err := scan.Repo(options.Root)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "scan failed: %v\n", err)
		return ExitError
	}
	cfg, err := config.Load(snapshot)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "config failed: %v\n", err)
		return ExitError
	}
	ruleSet := filterRuleSet(rules.DefaultRules(), options.OnlyRuleIDs)
	result := rules.RunWithConfig(ctx, snapshot, ruleSet, cfg)
	if options.Execute {
		findings := append([]rules.Finding(nil), result.Findings...)
		findings = append(findings, executeGoChecks(ctx, snapshot, options, cfg, runner)...)
		result = rules.NewReport(findings)
	}
	result.Scope = rules.GoScopeCoverage(snapshot, cfg)
	return finishCheck(options, result, out, errOut)
}

// finishCheck applies the requested baseline mode and renders the report.
// Baseline write mode replaces the report with a write summary; baseline
// check mode marks matched findings and appends the debt to text output.
func finishCheck(options CheckOptions, result rules.Report, out io.Writer, errOut io.Writer) int {
	switch options.Baseline {
	case BaselineWrite:
		summary, err := writeBaselineFile(options.Root, result)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "check failed: %v\n", err)
			return ExitError
		}
		_, _ = io.WriteString(out, summary)
		return ExitOK
	case BaselineCheck:
		if err := applyBaselineCheck(options.Root, &result); err != nil {
			_, _ = fmt.Fprintf(errOut, "check failed: %v\n", err)
			return ExitError
		}
	case BaselineOff:
	}
	return renderCheckReport(options, result, out, errOut)
}

func renderCheckReport(options CheckOptions, result rules.Report, out io.Writer, errOut io.Writer) int {
	if err := writeReport(out, options.Format, result); err != nil {
		_, _ = fmt.Fprintf(errOut, "report failed: %v\n", err)
		return ExitError
	}
	if options.Baseline == BaselineCheck && textFormat(options.Format) {
		_, _ = io.WriteString(out, baselineDebtLine(result))
	}
	if result.OK {
		return ExitOK
	}
	return ExitFindings
}

func textFormat(format string) bool {
	return format == "" || format == "text"
}

func validateOnlyRuleIDs(onlyRuleIDs []string) error {
	if len(onlyRuleIDs) == 0 {
		return nil
	}
	known := make(map[string]bool)
	for _, definition := range rules.DefaultDefinitions() {
		known[definition.ID] = true
	}
	var unknown []string
	for _, ruleID := range onlyRuleIDs {
		if !known[ruleID] {
			unknown = append(unknown, ruleID)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown rule: %s", strings.Join(unknown, ", "))
	}
	return nil
}

func ruleIDSet(ruleIDs []string) map[string]bool {
	wanted := make(map[string]bool, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		wanted[ruleID] = true
	}
	return wanted
}

func filterRuleSet(ruleSet []rules.Rule, onlyRuleIDs []string) []rules.Rule {
	if len(onlyRuleIDs) == 0 {
		return ruleSet
	}
	wanted := ruleIDSet(onlyRuleIDs)
	filtered := make([]rules.Rule, 0, len(ruleSet))
	for _, rule := range ruleSet {
		if wanted[rule.Metadata().ID] {
			filtered = append(filtered, rule)
		}
	}
	return filtered
}

func ruleSelected(onlyRuleIDs []string, ruleID string) bool {
	if len(onlyRuleIDs) == 0 {
		return true
	}
	return ruleIDSet(onlyRuleIDs)[ruleID]
}

func Explain(ruleID string, out io.Writer, errOut io.Writer) int {
	text, ok := rules.Explain(rules.DefaultRules(), ruleID)
	if !ok {
		_, _ = fmt.Fprintf(errOut, "unknown rule: %s\n", ruleID)
		return ExitError
	}
	_, err := io.WriteString(out, text)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "write failed: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func Rules(options RulesOptions, out io.Writer, errOut io.Writer) int {
	switch options.Format {
	case "", "text":
		return writeRulesText(out, errOut)
	case "json":
		return writeRulesJSON(out, errOut)
	default:
		_, _ = fmt.Fprintf(errOut, "unsupported rules format: %s\n", options.Format)
		return ExitError
	}
}

func writeRulesText(out io.Writer, errOut io.Writer) int {
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "RULE ID\tCATEGORY\tSEVERITY\tSTATUS\tTOOL")
	for _, definition := range rules.DefaultDefinitions() {
		_, _ = fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\n",
			definition.ID,
			definition.Category,
			definition.Severity,
			definition.Status,
			definition.Tool,
		)
	}
	if err := writer.Flush(); err != nil {
		_, _ = fmt.Fprintf(errOut, "write failed: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func writeRulesJSON(out io.Writer, errOut io.Writer) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(rules.DefaultDefinitions()); err != nil {
		_, _ = fmt.Fprintf(errOut, "write failed: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func CheckGoDry(ctx context.Context, options toolchecks.DryOptions, out io.Writer, errOut io.Writer) int {
	return runConfiguredGoTool(ctx, options, out, errOut, applyDryConfig, checkDryInModules)
}

func CheckGoCRAP(ctx context.Context, options toolchecks.CRAPOptions, out io.Writer, errOut io.Writer) int {
	return runConfiguredGoTool(ctx, options, out, errOut, applyCRAPConfig, checkCRAPInModules)
}

func CheckGoCoverage(ctx context.Context, options toolchecks.CoverageOptions, out io.Writer, errOut io.Writer) int {
	return runConfiguredGoTool(ctx, options, out, errOut, applyCoverageConfig, checkCoverageInModules)
}

func CheckGoMutation(ctx context.Context, options toolchecks.MutationOptions, out io.Writer, errOut io.Writer) int {
	return runConfiguredGoTool(ctx, options, out, errOut, applyMutationConfig, checkMutationInModules)
}

func writeReport(out io.Writer, format string, result rules.Report) error {
	switch format {
	case "", "text":
		return report.WriteText(out, result)
	case "json":
		return report.WriteJSON(out, result)
	case "sarif":
		return report.WriteSARIF(out, result)
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func runWithCommandConfig(root string, errOut io.Writer, run func(repo.Snapshot, config.Config) int) int {
	snapshot, err := scan.Repo(commandRoot(root))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "config failed: %v\n", err)
		return ExitError
	}
	cfg, err := config.Load(snapshot)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "config failed: %v\n", err)
		return ExitError
	}
	return run(snapshot, cfg)
}

func runConfiguredGoTool[T interface{ RootPath() string }](
	ctx context.Context,
	options T,
	out io.Writer,
	errOut io.Writer,
	apply func(*T, config.Config),
	run func(context.Context, repo.Snapshot, T, io.Writer, io.Writer, toolchecks.Runner) int,
) int {
	return runWithCommandConfig(options.RootPath(), errOut, func(snapshot repo.Snapshot, cfg config.Config) int {
		apply(&options, cfg)
		return run(ctx, snapshot, options, out, errOut, toolchecks.ExecRunner{})
	})
}

func commandRoot(root string) string {
	if root == "" {
		return "."
	}
	return root
}

func applyDryConfig(options *toolchecks.DryOptions, cfg config.Config) {
	if !options.MaximumSet && cfg.Go.DRYMaxCandidatesSet {
		options.MaximumCandidates = cfg.Go.DRYMaxCandidates
		options.MaximumSet = true
	}
	paths, exclude := cfg.GoDRYScope()
	options.Paths = append([]string(nil), paths...)
	options.Exclude = append([]string(nil), exclude...)
	if cfg.Go.DRY.Structural.EnabledSet {
		options.StructuralEnabled = cfg.Go.DRY.Structural.Enabled
		options.StructuralSet = true
	}
	options.StructuralThreshold = cfg.Go.DRY.Structural.Threshold
	options.StructuralMinLines = cfg.Go.DRY.Structural.MinLines
	options.StructuralMinNodes = cfg.Go.DRY.Structural.MinNodes
	if cfg.Go.DRY.CopiedBlocks.EnabledSet {
		options.CopiedBlockEnabled = cfg.Go.DRY.CopiedBlocks.Enabled
		options.CopiedBlockSet = true
	}
	options.CopiedBlockTokens = cfg.Go.DRY.CopiedBlocks.MinTokens
}

func applyCRAPConfig(options *toolchecks.CRAPOptions, cfg config.Config) {
	metric := crapGoMetric(options, cfg)
	applyConfiguredGoMetricWithCoverageProfile(metric, &options.CoverageProfile, cfg)
}

func applyCoverageConfig(options *toolchecks.CoverageOptions, cfg config.Config) {
	applyConfiguredGoMetricWithCoverageProfile(coverageGoMetric(options, cfg), &options.CoverageProfile, cfg)
}

type goMetricConfig struct {
	value      *float64
	valueSet   *bool
	configured float64
	targetsOut *[]string
	excludeOut *[]string
}

func crapGoMetric(options *toolchecks.CRAPOptions, cfg config.Config) goMetricConfig {
	return goMetricConfig{&options.MaximumScore, &options.MaximumSet, cfg.Go.CRAPMaxScore, &options.Targets, &options.Exclude}
}

func coverageGoMetric(options *toolchecks.CoverageOptions, cfg config.Config) goMetricConfig {
	return goMetricConfig{&options.Threshold, &options.ThresholdSet, cfg.Go.CoverageThreshold, &options.Targets, &options.Exclude}
}

func applyConfiguredGoMetric(metric goMetricConfig, cfg config.Config) {
	applyConfiguredGoFloat(metric.value, metric.valueSet, metric.configured)
	applyGoScope(metric.targetsOut, metric.excludeOut, cfg)
}

func applyConfiguredGoMetricWithCoverageProfile(metric goMetricConfig, profile *string, cfg config.Config) {
	applyConfiguredGoMetric(metric, cfg)
	applyConfiguredCoverageProfile(profile, cfg)
}

func applyConfiguredCoverageProfile(profile *string, cfg config.Config) {
	if *profile == "" {
		*profile = cfg.GoCoverageProfile()
	}
}

func applyConfiguredGoFloat(value *float64, valueSet *bool, configured float64) {
	if !*valueSet && configured > 0 {
		*value = configured
		*valueSet = true
	}
}

func applyGoScope(targetsOut *[]string, excludeOut *[]string, cfg config.Config) {
	targets, exclude := cfg.GoScope()
	*targetsOut = append([]string(nil), targets...)
	*excludeOut = append([]string(nil), exclude...)
}

func applyMutationConfig(options *toolchecks.MutationOptions, cfg config.Config) {
	targets, exclude := cfg.GoMutationScope()
	options.Exclude = exclude
	if options.Target != "" || len(options.Targets) > 0 {
		return
	}
	options.Targets = targets
}

type goToolEnv struct {
	snapshot        repo.Snapshot
	options         CheckOptions
	cfg             config.Config
	runner          toolchecks.Runner
	root            string
	coverageProfile string
}

func executeGoChecks(ctx context.Context, snapshot repo.Snapshot, checkOptions CheckOptions, cfg config.Config, runner toolchecks.Runner) []rules.Finding {
	coverageProfile := checkOptions.CoverageProfile
	if coverageProfile == "" {
		coverageProfile = cfg.GoCoverageProfile()
	}
	env := goToolEnv{
		snapshot:        snapshot,
		options:         checkOptions,
		cfg:             cfg,
		runner:          runner,
		root:            commandRoot(checkOptions.Root),
		coverageProfile: coverageProfile,
	}
	var findings []rules.Finding
	findings = append(findings, executeGoDryCheck(ctx, env)...)
	findings = append(findings, executeGoMetricChecks(ctx, env)...)
	findings = append(findings, executeGoMutationCheck(ctx, env)...)
	return findings
}

func executeGoDryCheck(ctx context.Context, env goToolEnv) []rules.Finding {
	if !ruleSelected(env.options.OnlyRuleIDs, rules.GoDryRequiredRuleID) || !env.cfg.Go.DRYMaxCandidatesSet {
		return nil
	}
	paths, exclude := env.cfg.GoDRYScope()
	options := toolchecks.DryOptions{
		Root:                env.root,
		MaximumCandidates:   env.cfg.Go.DRYMaxCandidates,
		MaximumSet:          true,
		Paths:               append([]string(nil), paths...),
		Exclude:             append([]string(nil), exclude...),
		StructuralEnabled:   env.cfg.Go.DRY.Structural.Enabled,
		StructuralSet:       env.cfg.Go.DRY.Structural.EnabledSet,
		StructuralThreshold: env.cfg.Go.DRY.Structural.Threshold,
		StructuralMinLines:  env.cfg.Go.DRY.Structural.MinLines,
		StructuralMinNodes:  env.cfg.Go.DRY.Structural.MinNodes,
		CopiedBlockEnabled:  env.cfg.Go.DRY.CopiedBlocks.Enabled,
		CopiedBlockSet:      env.cfg.Go.DRY.CopiedBlocks.EnabledSet,
		CopiedBlockTokens:   env.cfg.Go.DRY.CopiedBlocks.MinTokens,
	}
	return appendToolFinding(nil, rules.GoDryRequiredRuleID, env.cfg, "DRY check exceeded the configured candidate budget", func(out, errOut io.Writer) int {
		return checkDryInModules(ctx, env.snapshot, options, out, errOut, env.runner)
	})
}

func executeGoMetricChecks(ctx context.Context, env goToolEnv) []rules.Finding {
	targets, exclude := env.cfg.GoScope()
	var findings []rules.Finding
	if ruleSelected(env.options.OnlyRuleIDs, rules.GoCoverageRequiredRuleID) && env.cfg.Go.CoverageThreshold > 0 {
		options := toolchecks.CoverageOptions{
			Root:            env.root,
			Threshold:       env.cfg.Go.CoverageThreshold,
			ThresholdSet:    true,
			CoverageProfile: env.coverageProfile,
			Targets:         append([]string(nil), targets...),
			Exclude:         append([]string(nil), exclude...),
		}
		findings = appendToolFinding(findings, rules.GoCoverageRequiredRuleID, env.cfg, "Go coverage is below the configured threshold", func(out, errOut io.Writer) int {
			return checkCoverageInModules(ctx, env.snapshot, options, out, errOut, env.runner)
		})
	}
	if ruleSelected(env.options.OnlyRuleIDs, rules.GoCRAPRequiredRuleID) && env.cfg.Go.CRAPMaxScore > 0 {
		options := toolchecks.CRAPOptions{
			Root:            env.root,
			MaximumScore:    env.cfg.Go.CRAPMaxScore,
			MaximumSet:      true,
			CoverageProfile: env.coverageProfile,
			Targets:         append([]string(nil), targets...),
			Exclude:         append([]string(nil), exclude...),
		}
		findings = appendToolFinding(findings, rules.GoCRAPRequiredRuleID, env.cfg, "crap4go found functions above the configured score", func(out, errOut io.Writer) int {
			return checkCRAPInModules(ctx, env.snapshot, options, out, errOut, env.runner)
		})
	}
	return findings
}

// The static rule credits `check --execute` as mutation evidence, so
// execute must run the survivor-failing gate, not a scan that cannot fail
// on a surviving mutant.
func executeGoMutationCheck(ctx context.Context, env goToolEnv) []rules.Finding {
	targets, exclude := env.cfg.GoMutationScope()
	if !ruleSelected(env.options.OnlyRuleIDs, rules.GoMutationRequiredRuleID) || len(targets) == 0 {
		return nil
	}
	options := toolchecks.MutationOptions{
		Root:    env.root,
		Targets: targets,
		Exclude: exclude,
	}
	return appendToolFinding(nil, rules.GoMutationRequiredRuleID, env.cfg, "mutation gate failed for at least one configured target", func(out, errOut io.Writer) int {
		return checkMutationInModules(ctx, env.snapshot, options, out, errOut, env.runner)
	})
}

func resolveGoMutationTargets(snapshot repo.Snapshot, options toolchecks.MutationOptions) (toolchecks.MutationOptions, error) {
	targets := mutationTargetPatterns(options)
	resolved, err := gotargets.ResolveWithSingleModuleFallback(snapshot, gotargets.Options{
		Targets: targets,
		Exclude: options.Exclude,
	}, goModuleRoots(snapshot), ".")
	if err != nil {
		return toolchecks.MutationOptions{}, err
	}
	options.Target = ""
	options.Targets = resolved
	return options, nil
}

func mutationTargetPatterns(options toolchecks.MutationOptions) []string {
	if options.Target != "" {
		return []string{options.Target}
	}
	return append([]string(nil), options.Targets...)
}

func appendToolFinding(
	findings []rules.Finding,
	ruleID string,
	cfg config.Config,
	message string,
	run func(io.Writer, io.Writer) int,
) []rules.Finding {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run(&out, &errOut)
	if code == ExitOK {
		return findings
	}
	if code == ExitError {
		message = strings.TrimSpace(message + ": " + firstNonEmpty(errOut.String(), out.String()))
	}
	return append(findings, rules.Finding{
		RuleID:   ruleID,
		Severity: rules.Severity(cfg.RuleSeverity(ruleID, string(rules.SeverityError))),
		Path:     "slophammer.yml",
		Message:  message,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "tool returned an error"
}
