package rules

import (
	"context"
	"path"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/gotargets"
	"github.com/osolmaz/slophammer/go/internal/gotools"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

type goStaticRule struct {
	definition Definition
	satisfied  func(repo.Snapshot) bool
}

func newGoModuleRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasGoModule}
}

func newGoTestsRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasGoTestCommand}
}

func newGoVetRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasGoVetCommand}
}

func newGoLintRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasGoLintConfigAndCommand}
}

func newGoCoverageRule(definition Definition) Rule {
	return goCoverageRule{definition: definition}
}

func newGoComplexityRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasGoComplexityLint}
}

func newGoDryRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasDryCommand}
}

func newGoCRAPRule(definition Definition) Rule {
	return goStaticRule{definition: definition, satisfied: hasCRAP4GoGate}
}

func newGoMutationRule(definition Definition) Rule {
	return goMutationRule{definition: definition}
}

func (r goStaticRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r goStaticRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	roots := goProjectRoots(snapshot)
	if len(roots) == 0 {
		return nil
	}
	for _, root := range roots {
		if !r.satisfied(goProjectSnapshot(snapshot, root, roots)) {
			return []Finding{finding(r.definition)}
		}
	}
	return nil
}

type goMutationRule struct {
	definition Definition
}

type goCoverageRule struct {
	definition Definition
}

func (r goCoverageRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r goCoverageRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	return goStaticRule{definition: r.definition, satisfied: hasGoCoverageGate}.Check(context.Background(), snapshot)
}

func (r goCoverageRule) CheckWithConfig(ctx context.Context, snapshot repo.Snapshot, cfg config.Config) []Finding {
	if cfg.Go.CoverageThreshold > 0 && hasConfigBackedSlophammerGoCheckExecuteCommand(snapshot) {
		return nil
	}
	return r.Check(ctx, snapshot)
}

func (r goMutationRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r goMutationRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	roots := goProjectRoots(snapshot)
	if len(roots) == 0 {
		return nil
	}
	for _, root := range roots {
		scoped := goProjectSnapshot(snapshot, root, roots)
		if !hasMutate4GoCommandForRoot(snapshot, scoped, root, roots) {
			return []Finding{finding(r.definition)}
		}
	}
	return nil
}

func hasGoModule(snapshot repo.Snapshot) bool {
	return snapshot.HasFileNamedFold("go.mod")
}

func hasGoTestCommand(snapshot repo.Snapshot) bool {
	return hasGoSubcommand(snapshot, "test")
}

func hasGoVetCommand(snapshot repo.Snapshot) bool {
	return hasGoSubcommand(snapshot, "vet")
}

func hasGoSubcommand(snapshot repo.Snapshot, subcommand string) bool {
	return hasRunnableCommandLine(snapshot, func(tokens []string) bool {
		return lineHasGoSubcommandAllPackages(tokens, subcommand)
	})
}

func hasGoLintConfigAndCommand(snapshot repo.Snapshot) bool {
	return hasGolangCIConfig(snapshot) && hasGolangCICommand(snapshot)
}

func hasGoCoverageGate(snapshot repo.Snapshot) bool {
	return hasCommandFileMatching(snapshot, fileHasGoCoverageGate)
}

func hasConfigBackedSlophammerGoCheckExecuteCommand(snapshot repo.Snapshot) bool {
	return hasCommandFileMatching(snapshot, fileHasConfigBackedSlophammerGoCheckExecuteCommand)
}

func hasCommandFileMatching(snapshot repo.Snapshot, match func(repo.File) bool) bool {
	for _, file := range commandFiles(snapshot) {
		if match(file) {
			return true
		}
	}
	return false
}

func fileHasGoCoverageGate(file repo.File) bool {
	combined := goCoverageEvidence{}
	for _, content := range commandSections(file) {
		section := coverageEvidence(content)
		if section.complete() {
			return true
		}
		combined = combined.merge(section)
	}
	return isWorkflowFilePath(file.Path) && combined.complete()
}

type goCoverageEvidence struct {
	hasProfile   bool
	hasCoverTool bool
	hasThreshold bool
}

func coverageEvidence(content string) goCoverageEvidence {
	return goCoverageEvidence{
		hasProfile:   contentHasCommandLine(content, lineHasGoTestCoverageProfileCommand),
		hasCoverTool: contentHasCommandLine(content, lineHasGoToolCoverCommand),
		hasThreshold: hasCoverageGateThreshold(content),
	}
}

func (e goCoverageEvidence) merge(other goCoverageEvidence) goCoverageEvidence {
	return goCoverageEvidence{
		hasProfile:   e.hasProfile || other.hasProfile,
		hasCoverTool: e.hasCoverTool || other.hasCoverTool,
		hasThreshold: e.hasThreshold || other.hasThreshold,
	}
}

func (e goCoverageEvidence) complete() bool {
	return e.hasProfile && e.hasCoverTool && e.hasThreshold
}

func hasGoComplexityLint(snapshot repo.Snapshot) bool {
	for _, file := range golangCIConfigFiles(snapshot) {
		if configEnablesComplexityLinter(file.Content) {
			return true
		}
	}
	return false
}

func hasDryCommand(snapshot repo.Snapshot) bool {
	return hasConfiguredCommandFile(snapshot, hasConfiguredDRYMaximum(snapshot), fileHasDryCommand)
}

func fileHasDryCommand(file repo.File, hasConfiguredMaximum bool) bool {
	if hasConfiguredMaximum && fileHasConfigBackedSlophammerGoCheckExecuteCommand(file) {
		return true
	}
	for _, content := range commandSections(file) {
		if contentHasGoToolCommand(content, gotools.Dry4Go) ||
			contentHasSlophammerGoCommand(content, "dry", "") {
			return true
		}
	}
	return false
}

func hasCRAP4GoGate(snapshot repo.Snapshot) bool {
	return hasConfiguredCommandFile(snapshot, hasConfiguredCRAPThreshold(snapshot), fileHasCRAP4GoGate)
}

func hasConfiguredCommandFile(snapshot repo.Snapshot, configured bool, fileHasCommand func(repo.File, bool) bool) bool {
	for _, file := range commandFiles(snapshot) {
		if fileHasCommand(file, configured) {
			return true
		}
	}
	return false
}

func fileHasCRAP4GoGate(file repo.File, hasConfiguredThreshold bool) bool {
	if hasConfiguredThreshold &&
		(fileHasConfigBackedSlophammerGoCommand(file, "crap") ||
			fileHasConfigBackedSlophammerGoCheckExecuteCommand(file)) {
		return true
	}
	for _, content := range commandSections(file) {
		if contentHasSlophammerGoCommand(content, "crap", "--max-score") {
			return true
		}
		if hasCRAPThreshold(content) && contentHasGoToolCommand(content, gotools.CRAP4Go) {
			return true
		}
	}
	return false
}

// mutate4go exits zero even when mutants survive, so a direct invocation
// is not a gate; only the slophammer-go wrapper forms, which fail on
// survivors and uncovered changed sites, count as evidence.
func hasMutate4GoCommand(snapshot repo.Snapshot) bool {
	hasConfiguredTargets := hasConfiguredGoMutationScope(snapshot, "", []string{""})
	for _, file := range commandFiles(snapshot) {
		for _, content := range commandSections(file) {
			if contentHasSlophammerGoCommand(content, "mutate", "--target") ||
				(hasConfiguredTargets && fileHasConfigBackedSlophammerGoCommand(file, "mutate")) {
				return true
			}
		}
	}
	return false
}

func hasMutate4GoCommandForRoot(full repo.Snapshot, scoped repo.Snapshot, root string, roots []string) bool {
	if hasSlophammerGoMutationTargetCommand(scoped) {
		return true
	}
	if hasModuleLocalSlophammerConfig(full, root) && hasConfiguredGoMutationScopeInSnapshot(scoped) {
		return hasLocalConfigBackedGoMutationCommand(full, scoped, root)
	}
	if hasConfiguredGoMutationScope(full, root, roots) {
		return hasConfigBackedSlophammerGoMutationCommand(scoped) ||
			hasRepoRootConfigBackedSlophammerGoMutationCommand(full)
	}
	return repoRootConfiguredGoMutationScopeExcludesRoot(full, root, roots)
}

func hasLocalConfigBackedGoMutationCommand(full repo.Snapshot, scoped repo.Snapshot, root string) bool {
	if !hasConfiguredGoMutationScopeInSnapshot(scoped) {
		return false
	}
	return hasModuleLocalConfigBackedSlophammerGoMutationCommand(full, root) ||
		hasConfigBackedSlophammerGoMutationCommandAtRoot(full, root) ||
		hasConfigBackedSlophammerGoMutationCommandInWorkingDir(full, root) ||
		hasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir(full, root)
}

func hasSlophammerGoMutationTargetCommand(snapshot repo.Snapshot) bool {
	for _, file := range commandFiles(snapshot) {
		for _, content := range commandSections(file) {
			if contentHasSlophammerGoCommand(content, "mutate", "--target") {
				return true
			}
		}
	}
	return false
}

func hasConfigBackedSlophammerGoMutationCommand(snapshot repo.Snapshot) bool {
	for _, file := range commandFiles(snapshot) {
		if fileHasConfigBackedSlophammerGoCommand(file, "mutate") ||
			fileHasConfigBackedSlophammerGoCheckExecuteCommand(file) {
			return true
		}
	}
	return false
}

func hasRepoRootConfigBackedSlophammerGoMutationCommand(snapshot repo.Snapshot) bool {
	moduleRoots := sortedRootSet(goModuleRootSet(snapshot))
	for _, file := range commandFiles(snapshot) {
		if commandFileIsUnderNestedGoModule(file.Path, moduleRoots) {
			continue
		}
		if fileHasConfigBackedSlophammerGoCommand(file, "mutate") ||
			fileHasConfigBackedSlophammerGoCheckExecuteCommand(file) {
			return true
		}
	}
	return false
}

func commandFileIsUnderNestedGoModule(filePath string, moduleRoots []string) bool {
	for _, root := range moduleRoots {
		if root != "" && strings.HasPrefix(filePath, root+"/") {
			return true
		}
	}
	return false
}

func hasConfigBackedSlophammerGoMutationCommandAtRoot(snapshot repo.Snapshot, root string) bool {
	return hasCommandFileForRoot(snapshot, root, func(file repo.File, root string) bool {
		if fileHasConfigBackedSlophammerGoCommandAtRoot(file, "mutate", root) ||
			fileHasConfigBackedSlophammerGoCheckExecuteCommandAtRoot(file, root) {
			return true
		}
		return false
	})
}

func hasCommandFileForRoot(snapshot repo.Snapshot, root string, match func(repo.File, string) bool) bool {
	if root == "" {
		root = "."
	}
	for _, file := range commandFiles(snapshot) {
		if match(file, root) {
			return true
		}
	}
	return false
}

func hasConfigBackedSlophammerGoMutationCommandInWorkingDir(snapshot repo.Snapshot, root string) bool {
	return hasCommandFileForRoot(snapshot, root, fileHasConfigBackedSlophammerGoMutationCommandInWorkingDir)
}

func fileHasConfigBackedSlophammerGoMutationCommandInWorkingDir(file repo.File, root string) bool {
	for _, content := range commandSections(file) {
		if contentHasConfigBackedSlophammerGoCommandInWorkingDir(content, "mutate", root) ||
			contentHasConfigBackedSlophammerGoCheckExecuteCommandInWorkingDir(content, root) {
			return true
		}
	}
	return false
}

func contentHasConfigBackedSlophammerGoCommandInWorkingDir(content string, subcommand string, workingDir string) bool {
	return contentHasConfigBackedSlophammerGoCommandInWorkingDirWith(content, workingDir, func(tokens []string, index int) (int, bool) {
		return slophammerGoCommandArgsStart(tokens, index, subcommand)
	})
}

func contentHasConfigBackedSlophammerGoCheckExecuteCommandInWorkingDir(content string, workingDir string) bool {
	return contentHasConfigBackedSlophammerGoCommandInWorkingDirWith(content, workingDir, func(tokens []string, index int) (int, bool) {
		argsStart, ok := slophammerGoCommandArgsStart(tokens, index, "check")
		if !ok || !lineHasBooleanFlag(tokens[argsStart:], "--execute") {
			return 0, false
		}
		return argsStart, true
	})
}

func contentHasConfigBackedSlophammerGoCommandInWorkingDirWith(
	content string,
	workingDir string,
	commandArgsStart func([]string, int) (int, bool),
) bool {
	workingDir = cleanRuleSlashPath(workingDir)
	return contentHasCommandLine(content, func(tokens []string) bool {
		for i := 0; i < len(tokens); i++ {
			argsStart, ok := commandArgsStart(tokens, i)
			if !ok {
				continue
			}
			priorDir, ok := priorCDWorkingDirectory(tokens[:i])
			if !ok || cleanRuleSlashPath(priorDir) != workingDir {
				continue
			}
			args := tokens[argsStart:]
			if token, ok := firstSlophammerGoPathArgument(args); ok {
				return pathIsConfigRootArgument(token, ".")
			}
			return true
		}
		return false
	})
}

func hasModuleLocalConfigBackedSlophammerGoMutationCommand(snapshot repo.Snapshot, root string) bool {
	if root == "" {
		return hasConfigBackedSlophammerGoMutationCommand(snapshot)
	}
	prefix := root + "/"
	for _, file := range commandFiles(snapshot) {
		if isWorkflowFilePath(file.Path) || !strings.HasPrefix(file.Path, prefix) {
			continue
		}
		if fileHasConfigBackedSlophammerGoCommand(file, "mutate") ||
			fileHasConfigBackedSlophammerGoCheckExecuteCommand(file) {
			return true
		}
	}
	return false
}

func hasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir(snapshot repo.Snapshot, root string) bool {
	return hasCommandFileForRoot(snapshot, root, fileHasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir)
}

func fileHasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir(file repo.File, root string) bool {
	if !isWorkflowFilePath(file.Path) {
		return false
	}
	root = cleanRuleSlashPath(root)
	for _, block := range workflowCommandBlocks(file.Content) {
		if !workflowBlockUsesWorkingDirectory(block, root) {
			continue
		}
		runContent := workflowBlockRunContent(block)
		if contentHasConfigBackedSlophammerGoCommand(runContent, "mutate", ".") ||
			contentHasConfigBackedSlophammerGoCheckExecuteCommand(runContent, ".") {
			return true
		}
	}
	return false
}

func workflowBlockUsesWorkingDirectory(block string, root string) bool {
	workingDir, ok := workflowBlockWorkingDirectory(block)
	return ok && cleanRuleSlashPath(workingDir) == root
}

func workflowBlockWorkingDirectory(block string) (string, bool) {
	workingDirectory := ""
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "working-directory:") {
			continue
		}
		workingDirectory = strings.TrimSpace(strings.TrimPrefix(trimmed, "working-directory:"))
	}
	return workingDirectory, workingDirectory != ""
}

func cleanRuleSlashPath(value string) string {
	return path.Clean(strings.ReplaceAll(cleanCommandToken(value), "\\", "/"))
}

func fileHasConfigBackedSlophammerGoCommandAtRoot(file repo.File, subcommand string, configRootPath string) bool {
	if isWorkflowFilePath(file.Path) {
		for _, block := range workflowCommandBlocks(file.Content) {
			blockRootPath := configRootPath
			if workingDirectory, ok := workflowBlockWorkingDirectory(block); ok {
				blockRootPath = configRootPathFromWorkflowWorkingDirectory(configRootPath, workingDirectory)
			}
			if contentHasConfigBackedSlophammerGoCommand(workflowBlockRunContent(block), subcommand, blockRootPath) {
				return true
			}
		}
		return false
	}
	for _, content := range commandSections(file) {
		if contentHasConfigBackedSlophammerGoCommand(content, subcommand, configRootPath) {
			return true
		}
	}
	return false
}

func fileHasConfigBackedSlophammerGoCheckExecuteCommandAtRoot(file repo.File, configRootPath string) bool {
	if isWorkflowFilePath(file.Path) {
		for _, block := range workflowCommandBlocks(file.Content) {
			blockRootPath := configRootPath
			if workingDirectory, ok := workflowBlockWorkingDirectory(block); ok {
				blockRootPath = configRootPathFromWorkflowWorkingDirectory(configRootPath, workingDirectory)
			}
			if contentHasConfigBackedSlophammerGoCheckExecuteCommand(workflowBlockRunContent(block), blockRootPath) {
				return true
			}
		}
		return false
	}
	for _, content := range commandSections(file) {
		if contentHasConfigBackedSlophammerGoCheckExecuteCommand(content, configRootPath) {
			return true
		}
	}
	return false
}

func configRootPathFromWorkflowWorkingDirectory(configRootPath string, workingDirectory string) string {
	return relativeRuleSlashPath(cleanRuleSlashPath(workingDirectory), cleanRuleSlashPath(configRootPath))
}

func relativeRuleSlashPath(from string, to string) string {
	fromParts := cleanRuleSlashPathParts(from)
	toParts := cleanRuleSlashPathParts(to)
	common := 0
	for common < len(fromParts) && common < len(toParts) && fromParts[common] == toParts[common] {
		common++
	}
	parts := make([]string, 0, len(fromParts)-common+len(toParts)-common)
	for range fromParts[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, toParts[common:]...)
	if len(parts) == 0 {
		return "."
	}
	return path.Join(parts...)
}

func cleanRuleSlashPathParts(value string) []string {
	cleaned := cleanRuleSlashPath(value)
	if cleaned == "." || cleaned == "/" {
		return nil
	}
	return strings.Split(strings.Trim(cleaned, "/"), "/")
}

func hasModuleLocalSlophammerConfig(snapshot repo.Snapshot, root string) bool {
	if root == "" {
		return snapshot.HasFileFold(config.DefaultFileName) || snapshot.HasFileFold(config.AltFileName)
	}
	return snapshot.HasFileFold(root+"/"+config.DefaultFileName) || snapshot.HasFileFold(root+"/"+config.AltFileName)
}

func hasConfiguredGoMutationScopeInSnapshot(snapshot repo.Snapshot) bool {
	cfg, err := config.Load(snapshot)
	if err != nil {
		return false
	}
	targets, exclude := cfg.GoMutationScope()
	if len(targets) == 0 {
		return false
	}
	_, err = resolveConfiguredGoMutationScope(snapshot, targets, exclude)
	return err == nil
}

func hasConfiguredCRAPThreshold(snapshot repo.Snapshot) bool {
	cfg, err := config.Load(snapshot)
	return err == nil && cfg.Go.CRAPMaxScore > 0
}

func hasConfiguredDRYMaximum(snapshot repo.Snapshot) bool {
	cfg, err := config.Load(snapshot)
	return err == nil && cfg.Go.DRYMaxCandidatesSet
}

func hasConfiguredGoMutationScope(snapshot repo.Snapshot, root string, roots []string) bool {
	cfg, err := config.Load(snapshot)
	if err != nil {
		return false
	}
	targets, exclude := cfg.GoMutationScope()
	if len(targets) == 0 {
		return false
	}
	resolved, err := resolveConfiguredGoMutationScope(snapshot, targets, exclude)
	if err != nil {
		return false
	}
	for _, filePath := range resolved {
		if gotargets.ContainsPath(root, filePath) && !isUnderOtherGoRoot(filePath, root, roots) {
			return true
		}
	}
	return false
}

func repoRootConfiguredGoMutationScopeExcludesRoot(snapshot repo.Snapshot, root string, roots []string) bool {
	cfg, err := config.Load(snapshot)
	if err != nil || cfg.SourceDir != "." {
		return false
	}
	targets, exclude := cfg.GoMutationScope()
	if len(targets) == 0 {
		return false
	}
	resolved, err := resolveConfiguredGoMutationScope(snapshot, targets, exclude)
	if err != nil {
		return false
	}
	for _, filePath := range resolved {
		if gotargets.ContainsPath(root, filePath) && !isUnderOtherGoRoot(filePath, root, roots) {
			return false
		}
	}
	return true
}

func resolveConfiguredGoMutationScope(snapshot repo.Snapshot, targets []string, exclude []string) ([]string, error) {
	return gotargets.ResolveWithSingleModuleFallback(snapshot, gotargets.Options{
		Targets: targets,
		Exclude: exclude,
	}, sortedRootSet(goModuleRootSet(snapshot)), "")
}

func finding(definition Definition) Finding {
	return Finding{
		RuleID:   definition.ID,
		Severity: definition.Severity,
		Path:     definition.Path,
		Message:  definition.Message,
	}
}

const workflowStepBoundary = "\nSLOPHAMMER_WORKFLOW_STEP_BOUNDARY\n"

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-13T01:10:20+08:00","module_hash":"0ac9184403d9f8ad80816716bcf25f134bcba67467728f28be640b41ef2b3992","functions":[{"id":"func/newGoModuleRule","name":"newGoModuleRule","line":19,"end_line":21,"hash":"0eb606839c4537d03f67acf1f2ac02231644e565b78077e78eceaa47bcbd8121"},{"id":"func/newGoTestsRule","name":"newGoTestsRule","line":23,"end_line":25,"hash":"045379857a96d906a00bdb49cab98ba495d438866eb86801b1d0bd5af82ae951"},{"id":"func/newGoVetRule","name":"newGoVetRule","line":27,"end_line":29,"hash":"454d106e3acd41f3f8671eecdb819ae2d4b6c8458595a5707f3f322068b4e7d2"},{"id":"func/newGoLintRule","name":"newGoLintRule","line":31,"end_line":33,"hash":"292afa4acd32639e792fe0ee321b1dc4bc20457ffdedf6507cd76b9164445ec3"},{"id":"func/newGoCoverageRule","name":"newGoCoverageRule","line":35,"end_line":37,"hash":"9662a84a57e19998878ae7441341d11aec7e08f3b66c453969f88d86755cbbad"},{"id":"func/newGoComplexityRule","name":"newGoComplexityRule","line":39,"end_line":41,"hash":"eeca4bcb4caa1bec3d615bb0472fb156f0fd7dd9b59a9a9824b983585e018abb"},{"id":"func/newGoDryRule","name":"newGoDryRule","line":43,"end_line":45,"hash":"de4211a7d6ef16249fc47e876ea9a47a9324b21e7ff4e8f023387d3472b6ddf4"},{"id":"func/newGoCRAPRule","name":"newGoCRAPRule","line":47,"end_line":49,"hash":"a754ef6f98a78d19baf415b409d8c4b836a0c5e5231e15755807a6662d8210cc"},{"id":"func/newGoMutationRule","name":"newGoMutationRule","line":51,"end_line":53,"hash":"7ad16db76338e908bea8f89b2ea80f4ee2216c99a282646c011bf50c90ee99d4"},{"id":"func/goStaticRule.Metadata","name":"goStaticRule.Metadata","line":55,"end_line":57,"hash":"03042842e36c5afe12be3a19aaf45cdb1b024b8f3ce26bb9e45619235b84f745"},{"id":"func/goStaticRule.Check","name":"goStaticRule.Check","line":59,"end_line":70,"hash":"5c19806b0ea0ae5e82849c3697147bec6ed59814669ebf6b92092d37aacb72ac"},{"id":"func/goCoverageRule.Metadata","name":"goCoverageRule.Metadata","line":80,"end_line":82,"hash":"0481daacf712bf041be6ccd923a0417d5de3ae4e72d9ac2999f35d7250f7896a"},{"id":"func/goCoverageRule.Check","name":"goCoverageRule.Check","line":84,"end_line":86,"hash":"2bc19c73b6b72e133b482a5e4e009189c6e6c24111114f2fdfbec87208502131"},{"id":"func/goCoverageRule.CheckWithConfig","name":"goCoverageRule.CheckWithConfig","line":88,"end_line":93,"hash":"8b38a9174760ae332446f3da2eadecbe17a710d0b1df41fd6d9d6c17f4c8b3ba"},{"id":"func/goMutationRule.Metadata","name":"goMutationRule.Metadata","line":95,"end_line":97,"hash":"a48f3f764064ff6b436693db7a0b99eefd61d8a23255a1185bb867065936a407"},{"id":"func/goMutationRule.Check","name":"goMutationRule.Check","line":99,"end_line":111,"hash":"21d9cd93c8c22b2548b5aefd415acd56bc67fab48d7b500251431e06b5975dc5"},{"id":"func/hasGoModule","name":"hasGoModule","line":113,"end_line":115,"hash":"c991aa1f424bafca0777c16a851f8f1cbf456dd42672d657d4148b7ecce1c53f"},{"id":"func/hasGoTestCommand","name":"hasGoTestCommand","line":117,"end_line":119,"hash":"7dacf4ef67988c0630a16ed300b39cb44142f783a7f7d78cf9b7421c1d2cd7a8"},{"id":"func/hasGoVetCommand","name":"hasGoVetCommand","line":121,"end_line":123,"hash":"2ae266a917c53d8cb257cc1fe7945f60b5cf6fe7323d1fe7ae4b447983df1689"},{"id":"func/hasGoSubcommand","name":"hasGoSubcommand","line":125,"end_line":129,"hash":"6b70463391a35d08ee427415b3295abf1aedd48944d7abbb5c4bb903e9bfe5af"},{"id":"func/hasGoLintConfigAndCommand","name":"hasGoLintConfigAndCommand","line":131,"end_line":133,"hash":"19c28a54753f6bbf916dcc50f0e8e469ec777848cd5a39813e5c2492e38632e3"},{"id":"func/hasGoCoverageGate","name":"hasGoCoverageGate","line":135,"end_line":137,"hash":"c154e01f5abb785f2b1acce72cf0c583d9cc6195192c5c13315c5255308d5644"},{"id":"func/hasConfigBackedSlophammerGoCheckExecuteCommand","name":"hasConfigBackedSlophammerGoCheckExecuteCommand","line":139,"end_line":141,"hash":"5b226f420f0a6b2d111cfd6d5e038a906929a72471808e4673f031716d0bc5bf"},{"id":"func/hasCommandFileMatching","name":"hasCommandFileMatching","line":143,"end_line":150,"hash":"5ba32de4595f76db5281d4ed1658e03ae55cd49641eb0e8c474e7635b280bba0"},{"id":"func/fileHasGoCoverageGate","name":"fileHasGoCoverageGate","line":152,"end_line":162,"hash":"83d2e3031ddbe18a0ca97614d9c168b7d71ffd775db4a3a9046b3512db259514"},{"id":"func/coverageEvidence","name":"coverageEvidence","line":170,"end_line":176,"hash":"a85fdf969fd7c9551ebb720a6053858ef4ed8198dde4f36d3a91659a2d43d9a4"},{"id":"func/goCoverageEvidence.merge","name":"goCoverageEvidence.merge","line":178,"end_line":184,"hash":"646aecf93f3b61522502db9d367fd44e065a1d416ab1d7eb257e94bef97af4f2"},{"id":"func/goCoverageEvidence.complete","name":"goCoverageEvidence.complete","line":186,"end_line":188,"hash":"ec900a3cd0c5e5540c437a4e67702e589d6e4247361adabf8541c7ed768f1bef"},{"id":"func/hasGoComplexityLint","name":"hasGoComplexityLint","line":190,"end_line":197,"hash":"403d5e21143bd5ecce79c80dcf7217438412467c3f8da8a939bf5e7111818a73"},{"id":"func/hasDryCommand","name":"hasDryCommand","line":199,"end_line":201,"hash":"01958e66d2d137cb090f6e254326168d0808c3ce3bb61c62be616e8a7d409945"},{"id":"func/fileHasDryCommand","name":"fileHasDryCommand","line":203,"end_line":214,"hash":"e1864a5ca706f51383e0668f7e1ce7d1d9ab95cee53f60ad441b106667b6501c"},{"id":"func/hasCRAP4GoGate","name":"hasCRAP4GoGate","line":216,"end_line":218,"hash":"a906a7fb6b20ccf601aa23bb9aaf899cca4987bb1041bbef25d2cb5b877fd553"},{"id":"func/hasConfiguredCommandFile","name":"hasConfiguredCommandFile","line":220,"end_line":227,"hash":"f4054412538732dbc6ce6a19d96aaa0a142eac21daa430826288c791151f53b7"},{"id":"func/fileHasCRAP4GoGate","name":"fileHasCRAP4GoGate","line":229,"end_line":244,"hash":"2f390f875f822f8de023315f0ff6f43d6eedcda1177ea590c8b82a61fc78a7f3"},{"id":"func/hasMutate4GoCommand","name":"hasMutate4GoCommand","line":249,"end_line":260,"hash":"dbd4c6ab7be369b31ccd8c6219acca5c67a1c8d089f9ce139fe9e8b9095e95d6"},{"id":"func/hasMutate4GoCommandForRoot","name":"hasMutate4GoCommandForRoot","line":262,"end_line":274,"hash":"4a91420de70edc83f8ef54b79989d7c6839239be5fc62aaa7f6caf83f4385a74"},{"id":"func/hasLocalConfigBackedGoMutationCommand","name":"hasLocalConfigBackedGoMutationCommand","line":276,"end_line":284,"hash":"c8da432618e7a0bc1de9da17bce8fea8c34863a48bb62c2b133c2a3ea535d9f9"},{"id":"func/hasSlophammerGoMutationTargetCommand","name":"hasSlophammerGoMutationTargetCommand","line":286,"end_line":295,"hash":"6460f9fbe20b348647d098c87bcbb899b2c2507659769e845d30d9e638062564"},{"id":"func/hasConfigBackedSlophammerGoMutationCommand","name":"hasConfigBackedSlophammerGoMutationCommand","line":297,"end_line":305,"hash":"9b439777078f4363dae38df02f1b0f5025e1af27236f28ac0c11248ce919c462"},{"id":"func/hasRepoRootConfigBackedSlophammerGoMutationCommand","name":"hasRepoRootConfigBackedSlophammerGoMutationCommand","line":307,"end_line":319,"hash":"aaec3a177da0ac23c184e1187e58b104cadf29df1497d749262e8026ba991251"},{"id":"func/commandFileIsUnderNestedGoModule","name":"commandFileIsUnderNestedGoModule","line":321,"end_line":328,"hash":"665cb270bcb6d3e33cba1d936c18aeeaf5102a8ea0f5715e4a2dcc3c7fc5d9d6"},{"id":"func/hasConfigBackedSlophammerGoMutationCommandAtRoot","name":"hasConfigBackedSlophammerGoMutationCommandAtRoot","line":330,"end_line":338,"hash":"62b5076b39cad53da3debe71f93becd77ba8763afdd4e00e881f23f6706b28cf"},{"id":"func/hasCommandFileForRoot","name":"hasCommandFileForRoot","line":340,"end_line":350,"hash":"5d8f14813d73cfe387a99edd95f75ee8e4743abf659b19663dc79df9578975a3"},{"id":"func/hasConfigBackedSlophammerGoMutationCommandInWorkingDir","name":"hasConfigBackedSlophammerGoMutationCommandInWorkingDir","line":352,"end_line":354,"hash":"0398f614733f6d8d97478aa59c9063961fd92b22c3b8797f864daa0410b168b6"},{"id":"func/fileHasConfigBackedSlophammerGoMutationCommandInWorkingDir","name":"fileHasConfigBackedSlophammerGoMutationCommandInWorkingDir","line":356,"end_line":364,"hash":"28120f44f18854b72f0aa9486fcadf90a8f50d6546a9168b18b0e34806264e88"},{"id":"func/contentHasConfigBackedSlophammerGoCommandInWorkingDir","name":"contentHasConfigBackedSlophammerGoCommandInWorkingDir","line":366,"end_line":370,"hash":"f85884796eb4d5b78ff950fe6765b57a732e75cfd9b64e3ea18ba5abd5b5b3ca"},{"id":"func/contentHasConfigBackedSlophammerGoCheckExecuteCommandInWorkingDir","name":"contentHasConfigBackedSlophammerGoCheckExecuteCommandInWorkingDir","line":372,"end_line":380,"hash":"5fb2ab2154e66efe2d663bc86a94fed93ef633731d50a8322f868c14b900ee44"},{"id":"func/contentHasConfigBackedSlophammerGoCommandInWorkingDirWith","name":"contentHasConfigBackedSlophammerGoCommandInWorkingDirWith","line":382,"end_line":406,"hash":"05442b8cff91dc132fa85c10bf80b2ee2db768e935b265f353413e0ea7168625"},{"id":"func/hasModuleLocalConfigBackedSlophammerGoMutationCommand","name":"hasModuleLocalConfigBackedSlophammerGoMutationCommand","line":408,"end_line":423,"hash":"f6cea8eab37084221f6d37ac362699d2c65bdc9b6ee40c58c270492ea27a406e"},{"id":"func/hasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir","name":"hasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir","line":425,"end_line":427,"hash":"1d7adf46b60a2192d63e1d1ff0effb7876fe2908ad8c23b4b44d0d9e61958566"},{"id":"func/fileHasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir","name":"fileHasConfigBackedSlophammerGoMutationCommandInWorkflowWorkingDir","line":429,"end_line":445,"hash":"730e83f201f37ceefc84569b602d1d32f34450f4337969c994d13628c63e77d9"},{"id":"func/workflowBlockUsesWorkingDirectory","name":"workflowBlockUsesWorkingDirectory","line":447,"end_line":450,"hash":"7558736544ad9b90f58908a3d98ef1a48c8eb7945a92d5396bb0be3c805d72c9"},{"id":"func/workflowBlockWorkingDirectory","name":"workflowBlockWorkingDirectory","line":452,"end_line":462,"hash":"2d771308d69eaf4a2d9d6b251d73db7c56adc984e3f3f1f7050f081b453f59d6"},{"id":"func/cleanRuleSlashPath","name":"cleanRuleSlashPath","line":464,"end_line":466,"hash":"59734c6a4cff793138a9260b410e139c04372e02e2271ac7d6e1120e6e789360"},{"id":"func/fileHasConfigBackedSlophammerGoCommandAtRoot","name":"fileHasConfigBackedSlophammerGoCommandAtRoot","line":468,"end_line":487,"hash":"53c8f92cffb380488efb983ba6975642de2ab7efdfe13a1ed3c9551246606afe"},{"id":"func/fileHasConfigBackedSlophammerGoCheckExecuteCommandAtRoot","name":"fileHasConfigBackedSlophammerGoCheckExecuteCommandAtRoot","line":489,"end_line":508,"hash":"daeb2499b01f0f69b80bdadb447e3ce885667aad5a1a21b147338e9c24554de1"},{"id":"func/configRootPathFromWorkflowWorkingDirectory","name":"configRootPathFromWorkflowWorkingDirectory","line":510,"end_line":512,"hash":"b58bba3a4182e7be0249b42a5806f6101027944a6e66ae9feb2be9dd71595aac"},{"id":"func/relativeRuleSlashPath","name":"relativeRuleSlashPath","line":514,"end_line":530,"hash":"57ac0601d5cbf5654c64d133260d17e6291795ee596abea50d62d12163d08251"},{"id":"func/cleanRuleSlashPathParts","name":"cleanRuleSlashPathParts","line":532,"end_line":538,"hash":"b9f55ad52a2513c222e06e29c12189bb69e8476fb4bd04331c69bdd816b8a1ed"},{"id":"func/hasModuleLocalSlophammerConfig","name":"hasModuleLocalSlophammerConfig","line":540,"end_line":545,"hash":"90904cef49d43759357bd95ce59441e1fd8d1d734c2cd79eca4933cece0dd674"},{"id":"func/hasConfiguredGoMutationScopeInSnapshot","name":"hasConfiguredGoMutationScopeInSnapshot","line":547,"end_line":558,"hash":"df420b927362225dd5ad5651a01cbf43dce81850ecbea353e813be9cdd25a1fc"},{"id":"func/hasConfiguredCRAPThreshold","name":"hasConfiguredCRAPThreshold","line":560,"end_line":563,"hash":"251594276e7e9c73fb2126547644eb862b3b0d953a6a66c07545bfeed2e678c3"},{"id":"func/hasConfiguredDRYMaximum","name":"hasConfiguredDRYMaximum","line":565,"end_line":568,"hash":"6b85d1cc3f839b81f4d01622fa8b8bbad698295aca81607b7c61d8d1f0793e1d"},{"id":"func/hasConfiguredGoMutationScope","name":"hasConfiguredGoMutationScope","line":570,"end_line":589,"hash":"f21a7f183ff6836be20961955dd5df3e2a58bacd81203a29b83e648f36b874c8"},{"id":"func/repoRootConfiguredGoMutationScopeExcludesRoot","name":"repoRootConfiguredGoMutationScopeExcludesRoot","line":591,"end_line":610,"hash":"31d661c737bb9032c6f547b6900a2f67eca4796dfb9197745362aaf29ccd22c6"},{"id":"func/resolveConfiguredGoMutationScope","name":"resolveConfiguredGoMutationScope","line":612,"end_line":617,"hash":"47ae76bd6651a14c5842cb900ee49f576cf9d3a7ff08a4a36647f53e099083e4"},{"id":"func/finding","name":"finding","line":619,"end_line":626,"hash":"62a8bdd3ea891e5fe4cae9ca4c5ff2392a4bb0328c5239fb810a5a5a677bad24"}]}
// mutate4go-manifest-end
