package toolchecks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/dry"
	"github.com/osolmaz/slophammer/go/internal/gotools"
)

const (
	DefaultMaximumDRYCandidates = 0
	DefaultMaximumCRAPScore     = 8
	DefaultMinimumCoverage      = 85
)

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error)
}

type ExecRunner struct{}

type CommandResult struct {
	Stdout []byte
	Stderr []byte
}

func (ExecRunner) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	// #nosec G204 -- callers provide tool commands intentionally through the runner boundary.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

type DryOptions struct {
	Root                string
	MaximumCandidates   int
	MaximumSet          bool
	ShowReport          bool
	Format              string
	Paths               []string
	Exclude             []string
	StructuralEnabled   bool
	StructuralSet       bool
	StructuralThreshold float64
	StructuralMinLines  int
	StructuralMinNodes  int
	CopiedBlockEnabled  bool
	CopiedBlockSet      bool
	CopiedBlockTokens   int
}

func (options DryOptions) RootPath() string {
	return options.Root
}

type CRAPOptions struct {
	Root            string
	MaximumScore    float64
	MaximumSet      bool
	CoverageProfile string
	Targets         []string
	Exclude         []string
}

func (options CRAPOptions) RootPath() string {
	return options.Root
}

type CoverageOptions struct {
	Root            string
	Threshold       float64
	ThresholdSet    bool
	CoverageProfile string
	Targets         []string
	Exclude         []string
}

func (options CoverageOptions) RootPath() string {
	return options.Root
}

type MutationOptions struct {
	Root    string
	Target  string
	Targets []string
	Exclude []string
	Scan    bool
}

func (options MutationOptions) RootPath() string {
	return options.Root
}

func CheckDry(ctx context.Context, options DryOptions, out io.Writer, errOut io.Writer, runner Runner) int {
	_ = ctx
	_ = runner
	root := defaultRoot(options.Root)
	maximumCandidates := dryCandidateLimit(options)

	report, err := dry.Find(dry.Options{
		Root:                root,
		Paths:               dryPaths(options),
		StructuralEnabled:   dryStructuralEnabled(options),
		StructuralSet:       options.StructuralSet,
		StructuralThreshold: options.StructuralThreshold,
		StructuralMinLines:  options.StructuralMinLines,
		StructuralMinNodes:  options.StructuralMinNodes,
		CopiedBlockEnabled:  dryCopiedBlockEnabled(options),
		CopiedBlockSet:      options.CopiedBlockSet,
		CopiedBlockTokens:   options.CopiedBlockTokens,
	})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "dry check failed: %v\n", err)
		return 2
	}

	if options.Format == "json" || options.ShowReport {
		content, err := dry.WriteJSON(report)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "dry report render failed: %v\n", err)
			return 2
		}
		writeBytes(out, content)
	}
	if options.Format == "text" {
		_, _ = io.WriteString(out, dry.FormatText(report))
	}

	candidateCount := len(report.Findings)
	if options.Format != "json" {
		_, _ = fmt.Fprintf(out, "DRY candidates: %d; maximum: %d\n", candidateCount, maximumCandidates)
	}
	if candidateCount > maximumCandidates {
		return 1
	}
	return 0
}

func dryStructuralEnabled(options DryOptions) bool {
	return dryBoolDefault(options.StructuralSet, options.StructuralEnabled)
}

func dryCopiedBlockEnabled(options DryOptions) bool {
	return dryBoolDefault(options.CopiedBlockSet, options.CopiedBlockEnabled)
}

func dryBoolDefault(configured bool, value bool) bool {
	if configured {
		return value
	}
	return true
}

func dryCandidateLimit(options DryOptions) int {
	if !options.MaximumSet && options.MaximumCandidates == 0 {
		return DefaultMaximumDRYCandidates
	}
	return options.MaximumCandidates
}

func CheckCRAP(ctx context.Context, options CRAPOptions, out io.Writer, errOut io.Writer, runner Runner) int {
	root := defaultRoot(options.Root)
	maximumScore := crapScoreLimit(options)
	targets, profileTargets, targeted := targetedCRAPScope(options)
	if targeted {
		return checkTargetedCRAP(ctx, root, targets, profileTargets, maximumScore, options.CoverageProfile, out, errOut, runner)
	}

	result, err := runner.Run(ctx, root, "go", gotools.CRAP4Go.GoRunArgs(gotools.Latest, crapTargets(options)...)...)
	writeBytes(out, result.Stdout)
	writeBytes(errOut, result.Stderr)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "crap4go failed: %v\n", err)
		return 2
	}

	violations, err := CRAPViolations(result.Stdout, maximumScore)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "crap4go report parse failed: %v\n", err)
		return 2
	}
	for _, violation := range violations {
		_, _ = fmt.Fprintf(errOut, "CRAP score %.1f exceeds maximum %.1f for %s\n", violation.Score, maximumScore, violation.Name)
	}
	if len(violations) > 0 {
		return 1
	}
	return 0
}

func targetedCRAPScope(options CRAPOptions) ([]string, []string, bool) {
	targets := crapTargets(options)
	if len(targets) > 0 {
		return targets, targets, true
	}
	if strings.TrimSpace(options.CoverageProfile) != "" {
		return []string{"."}, nil, true
	}
	return nil, nil, false
}

func checkTargetedCRAP(
	ctx context.Context,
	root string,
	targets []string,
	profileTargets []string,
	maximumScore float64,
	coverageProfile string,
	out io.Writer,
	errOut io.Writer,
	runner Runner,
) int {
	analysis, ok := targetedCRAPAnalysis(ctx, root, targets, coverageProfile, errOut, runner)
	if !ok {
		return 2
	}
	if len(analysis.complexity) == 0 && checkNoComplexityCRAP(ctx, root, analysis.inputs.modulePath, profileTargets, coverageProfile, maximumScore, out, errOut, runner) {
		return 0
	}
	if len(analysis.complexity) == 0 {
		return 2
	}
	coverage, ok := targetedCRAPCoverage(ctx, root, analysis.inputs, coverageProfile, profileTargets, analysis.complexity, out, errOut, runner)
	if !ok {
		return 2
	}
	return reportTargetedCRAP(analysis.complexity, coverage, maximumScore, out, errOut)
}

type targetedCRAPData struct {
	inputs     targetedCoverageConfig
	complexity map[string]functionComplexity
}

func targetedCRAPAnalysis(ctx context.Context, root string, targets []string, coverageProfile string, errOut io.Writer, runner Runner) (targetedCRAPData, bool) {
	generateCoverage := coverageProfile == ""
	inputs, ok := targetedCRAPInputs(ctx, root, targets, generateCoverage, generateCoverage, errOut, runner)
	if !ok {
		return targetedCRAPData{}, false
	}
	complexity, ok := gocycloComplexity(ctx, root, targets, errOut, runner)
	if !ok {
		return targetedCRAPData{}, false
	}
	return targetedCRAPData{inputs: inputs, complexity: complexity}, true
}

func reportTargetedCRAP(complexity map[string]functionComplexity, coverage map[string]float64, maximumScore float64, out io.Writer, errOut io.Writer) int {
	violations := targetedCRAPViolations(complexity, coverage, maximumScore)
	for _, violation := range violations {
		_, _ = fmt.Fprintf(errOut, "CRAP score %.1f exceeds maximum %.1f for %s\n", violation.Score, maximumScore, violation.Name)
	}
	if len(violations) > 0 {
		return 1
	}
	_, _ = fmt.Fprintf(out, "CRAP scores meet maximum %.1f\n", maximumScore)
	return 0
}

func checkNoComplexityCRAP(
	ctx context.Context,
	root string,
	modulePath string,
	profileTargets []string,
	coverageProfile string,
	maximumScore float64,
	out io.Writer,
	errOut io.Writer,
	runner Runner,
) bool {
	if !checkEmptyCRAPCoverageProfile(ctx, root, modulePath, profileTargets, coverageProfile, errOut, runner) {
		return false
	}
	_, _ = fmt.Fprintf(out, "CRAP scores meet maximum %.1f\n", maximumScore)
	return true
}

func targetedCRAPCoverage(
	ctx context.Context,
	root string,
	inputs targetedCoverageConfig,
	coverageProfile string,
	profileTargets []string,
	complexity map[string]functionComplexity,
	out io.Writer,
	errOut io.Writer,
	runner Runner,
) (map[string]float64, bool) {
	profilePath, coverOutput, scopedFiles, cleanup, ok := targetedCoverageProfile(ctx, root, inputs.modulePath, coverageProfile, profileTargets, inputs.coverPackages, inputs.testPackages, out, errOut, runner)
	defer cleanup()
	if !ok {
		return nil, false
	}
	coverage, ok := coverFunctionCoverageFromOutput(inputs.modulePath, coverOutput)
	if !ok {
		coverage = map[string]float64{}
	}
	scopedFiles, ok = generatedCRAPCoverageScope(ctx, root, coverageProfile, profileTargets, scopedFiles, errOut, runner)
	if !ok {
		return nil, false
	}
	if !addRawFunctionLineCoverage(root, profilePath, inputs.modulePath, scopedFiles, coverage, complexity, errOut) {
		return nil, false
	}
	if !validateTargetedCRAPCoverage(root, profilePath, coverageProfile, scopedFiles, complexity, coverage, errOut) {
		return nil, false
	}
	return coverage, true
}

func generatedCRAPCoverageScope(ctx context.Context, root string, coverageProfile string, profileTargets []string, scopedFiles map[string]bool, errOut io.Writer, runner Runner) (map[string]bool, bool) {
	if strings.TrimSpace(coverageProfile) != "" {
		return scopedFiles, true
	}
	return requiredCoverageScopeFiles(ctx, root, profileTargets, errOut, runner)
}

func validateTargetedCRAPCoverage(root string, profilePath string, coverageProfile string, scopedFiles map[string]bool, complexity map[string]functionComplexity, coverage map[string]float64, errOut io.Writer) bool {
	if strings.TrimSpace(coverageProfile) != "" {
		return validateCRAPCoverageComplete(profilePath, complexity, coverage, errOut)
	}
	return validateGeneratedCRAPCoverageComplete(root, profilePath, scopedFiles, complexity, coverage, errOut)
}

func validateCRAPCoverageComplete(profilePath string, complexity map[string]functionComplexity, coverage map[string]float64, errOut io.Writer) bool {
	for key, item := range complexity {
		if _, ok := coverage[key]; ok {
			continue
		}
		_, _ = fmt.Fprintf(errOut, "coverage profile %s does not include coverage for analyzed function %s.%s at %s\n", profilePath, item.Package, item.Name, key)
		return false
	}
	return true
}

func validateGeneratedCRAPCoverageComplete(root string, profilePath string, scopedFiles map[string]bool, complexity map[string]functionComplexity, coverage map[string]float64, errOut io.Writer) bool {
	for key, item := range complexity {
		if _, ok := coverage[key]; ok {
			continue
		}
		if !generatedFunctionCanHaveCoverage(root, key, scopedFiles) {
			continue
		}
		if generatedFunctionIsLiteral(root, key, item) {
			_, _ = fmt.Fprintf(errOut, "coverage profile %s does not include coverage for analyzed function literal %s.%s at %s\n", profilePath, item.Package, item.Name, key)
			return false
		}
		_, _ = fmt.Fprintf(errOut, "coverage profile %s does not include coverage for analyzed function %s.%s at %s\n", profilePath, item.Package, item.Name, key)
		return false
	}
	return true
}

func generatedFunctionCanHaveCoverage(root string, key string, scopedFiles map[string]bool) bool {
	filePath, _, ok := splitFunctionLineKey(key)
	if !ok {
		return true
	}
	if scopedFiles != nil && !scopedFiles[filePath] {
		return false
	}
	return goFileMayHaveCoverageBlocks(root, filePath)
}

func generatedFunctionIsLiteral(root string, key string, item functionComplexity) bool {
	filePath, line, ok := splitFunctionLineKey(key)
	if !ok {
		return false
	}
	_, ok = functionLiteralRange(root, filePath, line, item.Column)
	return ok
}

func checkEmptyCRAPCoverageProfile(ctx context.Context, root string, modulePath string, profileTargets []string, coverageProfile string, errOut io.Writer, runner Runner) bool {
	if strings.TrimSpace(coverageProfile) == "" {
		return true
	}
	profilePath, ok := suppliedCoverageProfilePath(root, coverageProfile, errOut)
	if !ok {
		return false
	}
	_, _, ok = suppliedCoverageProfileOutput(ctx, root, modulePath, profilePath, profileTargets, errOut, runner)
	return ok
}

type targetedCoverageConfig struct {
	modulePath    string
	coverPackages []string
	testPackages  []string
}

func targetedCRAPInputs(ctx context.Context, root string, targets []string, includeCoverPackages bool, includeTestPackages bool, errOut io.Writer, runner Runner) (targetedCoverageConfig, bool) {
	modulePath, ok := goListModulePath(ctx, root, errOut, runner)
	if !ok {
		return targetedCoverageConfig{}, false
	}
	if !includeCoverPackages {
		return targetedCoverageConfig{modulePath: modulePath}, true
	}
	coverPackages, ok := goListPackages(ctx, root, packageDirs(targets), errOut, runner)
	if !ok {
		return targetedCoverageConfig{}, false
	}
	if !includeTestPackages {
		return targetedCoverageConfig{modulePath: modulePath, coverPackages: coverPackages}, true
	}
	testPackages, ok := goListPackages(ctx, root, []string{"./..."}, errOut, runner)
	if !ok {
		return targetedCoverageConfig{}, false
	}
	return targetedCoverageConfig{modulePath: modulePath, coverPackages: coverPackages, testPackages: testPackages}, true
}

func CheckCoverage(ctx context.Context, options CoverageOptions, out io.Writer, errOut io.Writer, runner Runner) int {
	root := defaultRoot(options.Root)
	threshold := coverageThreshold(options)
	inputs, ok := targetedCoverageInputs(ctx, root, options, errOut, runner)
	if !ok {
		return 2
	}
	profilePath, coverOutput, scopedFiles, cleanup, ok := targetedCoverageProfile(ctx, root, inputs.modulePath, options.CoverageProfile, coverageTargets(options), inputs.coverPackages, inputs.testPackages, out, errOut, runner)
	defer cleanup()
	if !ok {
		return 2
	}
	total, ok := coverageTotal(profilePath, inputs.modulePath, options.CoverageProfile, scopedFiles, coverOutput, errOut)
	if !ok {
		_, _ = fmt.Fprintf(errOut, "coverage profile %s did not include total coverage\n", profilePath)
		return 2
	}
	if total < threshold {
		_, _ = fmt.Fprintf(errOut, "coverage %.1f%% is below required %.1f%%\n", total, threshold)
		return 1
	}
	_, _ = fmt.Fprintf(out, "coverage %.1f%% meets required %.1f%%\n", total, threshold)
	return 0
}

func goListModulePath(ctx context.Context, root string, errOut io.Writer, runner Runner) (string, bool) {
	result, err := runner.Run(ctx, root, "go", "list", "-m")
	writeBytes(errOut, result.Stderr)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "go list -m failed: %v\n", err)
		return "", false
	}
	modulePath := strings.TrimSpace(string(result.Stdout))
	if modulePath == "" {
		_, _ = fmt.Fprintln(errOut, "go list -m returned an empty module path")
		return "", false
	}
	return modulePath, true
}

func goListPackages(ctx context.Context, root string, patterns []string, errOut io.Writer, runner Runner) ([]string, bool) {
	args := append([]string{"list"}, patterns...)
	result, err := runner.Run(ctx, root, "go", args...)
	writeBytes(errOut, result.Stderr)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "go list failed: %v\n", err)
		return nil, false
	}
	packages := strings.Fields(string(result.Stdout))
	if len(packages) == 0 {
		_, _ = fmt.Fprintf(errOut, "go list returned no packages for %s\n", strings.Join(patterns, " "))
		return nil, false
	}
	return packages, true
}

func packageDirs(targets []string) []string {
	seen := map[string]bool{}
	dirs := make([]string, 0, len(targets))
	for _, target := range targets {
		dir := path.Dir(strings.ReplaceAll(target, "\\", "/"))
		if dir == "." {
			dir = "."
		}
		pattern := "./" + strings.TrimPrefix(dir, "./")
		if pattern == "./." {
			pattern = "."
		}
		if !seen[pattern] {
			seen[pattern] = true
			dirs = append(dirs, pattern)
		}
	}
	sort.Strings(dirs)
	return dirs
}

type functionComplexity struct {
	Name       string
	Package    string
	Complexity float64
	Column     int
}

func gocycloComplexity(ctx context.Context, root string, targets []string, errOut io.Writer, runner Runner) (map[string]functionComplexity, bool) {
	args := gotools.Gocyclo.GoRunArgs(gotools.Latest, append([]string{"-over", "0"}, targets...)...)
	result, err := runner.Run(ctx, root, "go", args...)
	writeBytes(errOut, result.Stderr)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "gocyclo failed: %v\n", err)
		return nil, false
	}
	complexity := map[string]functionComplexity{}
	for _, line := range strings.Split(string(result.Stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		lineKey, column, ok := fileLineKeyAndColumn(fields[3])
		if !ok {
			continue
		}
		key := complexityLocationKey(root, lineKey, column)
		complexity[key] = functionComplexity{
			Name:       fields[2],
			Package:    fields[1],
			Complexity: value,
			Column:     column,
		}
	}
	return complexity, true
}

func complexityLocationKey(root string, lineKey string, column int) string {
	filePath, line, ok := splitFunctionLineKey(lineKey)
	if !ok {
		return lineKey
	}
	if _, ok := functionLiteralRange(root, filePath, line, column); ok {
		return fmt.Sprintf("%s:%d", lineKey, column)
	}
	return lineKey
}

func targetedCRAPViolations(complexity map[string]functionComplexity, coverage map[string]float64, maximumScore float64) []CRAPViolation {
	violations := make([]CRAPViolation, 0)
	for key, item := range complexity {
		covered, ok := coverage[key]
		if !ok {
			continue
		}
		uncovered := 1 - covered/100
		score := item.Complexity*item.Complexity*uncovered*uncovered*uncovered + item.Complexity
		rounded := roundTenths(score)
		if rounded > maximumScore {
			violations = append(violations, CRAPViolation{
				Name:  item.Package + "." + item.Name,
				Score: rounded,
			})
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Score == violations[j].Score {
			return violations[i].Name < violations[j].Name
		}
		return violations[i].Score > violations[j].Score
	})
	return violations
}

func roundTenths(value float64) float64 {
	rounded, _ := strconv.ParseFloat(fmt.Sprintf("%.1f", value), 64)
	return rounded
}

func coverFileLineKey(location string, modulePath string) (string, bool) {
	if !strings.HasPrefix(location, modulePath+"/") {
		return "", false
	}
	return fileLineKey(strings.TrimPrefix(location, modulePath+"/"))
}

func coverFilePath(location string, modulePath string) (string, bool) {
	if !strings.HasPrefix(location, modulePath+"/") {
		return "", false
	}
	filePath, _, _ := strings.Cut(strings.TrimPrefix(location, modulePath+"/"), ":")
	if filePath == "" {
		return "", false
	}
	return cleanGoTargetPath(filePath), true
}

func fileLineKey(location string) (string, bool) {
	key, _, ok := fileLineKeyAndColumn(location)
	return key, ok
}

func fileLineKeyAndColumn(location string) (string, int, bool) {
	parts := strings.Split(location, ":")
	if len(parts) < 2 {
		return "", 0, false
	}
	filePath := strings.ReplaceAll(parts[0], "\\", "/")
	line := parts[1]
	if filePath == "" || line == "" {
		return "", 0, false
	}
	column := 0
	if len(parts) >= 3 {
		parsed, err := strconv.Atoi(parts[2])
		if err == nil && parsed > 0 {
			column = parsed
		}
	}
	return filePath + ":" + line, column, true
}

func crapTargets(options CRAPOptions) []string {
	return nonEmptyStrings(options.Targets)
}

func crapScoreLimit(options CRAPOptions) float64 {
	return configuredFloat(options.MaximumScore, options.MaximumSet, DefaultMaximumCRAPScore)
}

func coverageTargets(options CoverageOptions) []string {
	return nonEmptyStrings(options.Targets)
}

func coveragePackagePatterns(options CoverageOptions) []string {
	targets := coverageTargets(options)
	if len(targets) == 0 {
		return []string{"./..."}
	}
	return packageDirs(targets)
}

func coverageThreshold(options CoverageOptions) float64 {
	return configuredFloat(options.Threshold, options.ThresholdSet, DefaultMinimumCoverage)
}

func configuredFloat(value float64, configured bool, fallback float64) float64 {
	if configured || value != 0 {
		return value
	}
	return fallback
}

func nonEmptyStrings(values []string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			kept = append(kept, value)
		}
	}
	return kept
}

func CountDRYCandidates(report []byte) (int, error) {
	var parsed map[string][]json.RawMessage
	if err := json.Unmarshal(report, &parsed); err != nil {
		return 0, err
	}
	if findings, ok := parsed["findings"]; ok {
		return len(findings), nil
	}
	if candidates, ok := parsed["candidates"]; ok {
		return len(candidates), nil
	}
	return 0, errors.New("missing findings field")
}

func DryPaths(options DryOptions) []string {
	return dryPaths(options)
}

type CRAPViolation struct {
	Name  string
	Score float64
}

func CRAPViolations(report []byte, maximumScore float64) ([]CRAPViolation, error) {
	var violations []CRAPViolation
	for _, line := range strings.Split(string(report), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		score, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		if score > maximumScore {
			violations = append(violations, CRAPViolation{Name: fields[0], Score: score})
		}
	}
	return violations, nil
}

func defaultRoot(root string) string {
	if root == "" {
		return "."
	}
	return root
}

func dryPaths(options DryOptions) []string {
	paths := make([]string, 0, len(options.Paths))
	for _, targetPath := range options.Paths {
		if targetPath != "" {
			paths = append(paths, targetPath)
		}
	}
	if len(paths) == 0 {
		return []string{"."}
	}
	return paths
}

func writeBytes(out io.Writer, content []byte) {
	if len(content) == 0 {
		return
	}
	_, _ = out.Write(content)
	if content[len(content)-1] != '\n' {
		_, _ = fmt.Fprintln(out)
	}
}
