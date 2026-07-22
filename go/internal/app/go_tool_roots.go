package app

import (
	"context"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/osolmaz/slophammer/go/internal/repo"
	"github.com/osolmaz/slophammer/go/internal/toolchecks"
)

func checkDryInModules(ctx context.Context, snapshot repo.Snapshot, options toolchecks.DryOptions, out io.Writer, errOut io.Writer, runner toolchecks.Runner) int {
	return checkGoToolInModules(ctx, snapshot, options, out, errOut, runner, dryOptionsForModule, toolchecks.CheckDry)
}

func checkCRAPInModules(ctx context.Context, snapshot repo.Snapshot, options toolchecks.CRAPOptions, out io.Writer, errOut io.Writer, runner toolchecks.Runner) int {
	return checkGoToolInModules(ctx, snapshot, options, out, errOut, runner, crapOptionsForModule, toolchecks.CheckCRAP)
}

func checkCoverageInModules(ctx context.Context, snapshot repo.Snapshot, options toolchecks.CoverageOptions, out io.Writer, errOut io.Writer, runner toolchecks.Runner) int {
	return checkGoToolInModules(ctx, snapshot, options, out, errOut, runner, coverageOptionsForModule, toolchecks.CheckCoverage)
}

func checkGoToolInModules[T any](
	ctx context.Context,
	snapshot repo.Snapshot,
	options T,
	out io.Writer,
	errOut io.Writer,
	runner toolchecks.Runner,
	optionsForModule func(T, repo.Snapshot, string) (T, bool),
	check func(context.Context, T, io.Writer, io.Writer, toolchecks.Runner) int,
) int {
	exitCode := ExitOK
	for _, moduleRoot := range goModuleRootsOrDefault(snapshot) {
		moduleOptions, ok := optionsForModule(options, snapshot, moduleRoot)
		if !ok {
			continue
		}
		code := check(ctx, moduleOptions, out, errOut, runner)
		if code == ExitError {
			return ExitError
		}
		if code == ExitFindings {
			exitCode = ExitFindings
		}
	}
	return exitCode
}

func checkMutationInModules(
	ctx context.Context,
	snapshot repo.Snapshot,
	options toolchecks.MutationOptions,
	out io.Writer,
	errOut io.Writer,
	runner toolchecks.Runner,
) int {
	resolvedOptions, err := resolveGoMutationTargets(snapshot, options)
	if err != nil {
		_, _ = io.WriteString(errOut, err.Error()+"\n")
		return ExitError
	}
	exitCode := ExitOK
	for _, moduleOptions := range mutationOptionsForModules(resolvedOptions, snapshot) {
		code := toolchecks.CheckMutation(ctx, moduleOptions, out, errOut, runner)
		if code == ExitError {
			return ExitError
		}
		if code == ExitFindings {
			exitCode = ExitFindings
		}
	}
	return exitCode
}

func goToolRoots(root string, snapshot repo.Snapshot) []string {
	moduleRoots := goModuleRootsOrDefault(snapshot)
	roots := make([]string, 0, len(moduleRoots))
	for _, moduleRoot := range moduleRoots {
		roots = append(roots, moduleToolRoot(root, moduleRoot))
	}
	return roots
}

func dryOptionsForModule(options toolchecks.DryOptions, snapshot repo.Snapshot, moduleRoot string) (toolchecks.DryOptions, bool) {
	return optionsForModuleGoFiles(options, snapshot, moduleRoot, options.Root, options.Paths, options.Exclude, setDryModuleFiles)
}

func crapOptionsForModule(options toolchecks.CRAPOptions, snapshot repo.Snapshot, moduleRoot string) (toolchecks.CRAPOptions, bool) {
	return optionsForModuleGoFilesWithCoverageProfile(options, snapshot, moduleRoot, options.Root, options.CoverageProfile, options.Targets, options.Exclude, setCRAPCoverageProfile, setCRAPModuleFiles)
}

func coverageOptionsForModule(options toolchecks.CoverageOptions, snapshot repo.Snapshot, moduleRoot string) (toolchecks.CoverageOptions, bool) {
	return optionsForModuleGoFilesWithCoverageProfile(options, snapshot, moduleRoot, options.Root, options.CoverageProfile, options.Targets, options.Exclude, setCoverageProfile, setCoverageModuleFiles)
}

func setDryModuleFiles(moduleOptions *toolchecks.DryOptions, root string, files []string) {
	moduleOptions.Root, moduleOptions.Paths = root, files
}

func setCRAPModuleFiles(moduleOptions *toolchecks.CRAPOptions, root string, files []string) {
	moduleOptions.Root, moduleOptions.Targets = root, files
}

func setCoverageModuleFiles(moduleOptions *toolchecks.CoverageOptions, root string, files []string) {
	moduleOptions.Root, moduleOptions.Targets = root, files
}

func setCRAPCoverageProfile(moduleOptions *toolchecks.CRAPOptions, profile string) {
	moduleOptions.CoverageProfile = profile
}

func setCoverageProfile(moduleOptions *toolchecks.CoverageOptions, profile string) {
	moduleOptions.CoverageProfile = profile
}

func optionsForModuleGoFilesWithCoverageProfile[T any](
	options T,
	snapshot repo.Snapshot,
	moduleRoot string,
	root string,
	profile string,
	includes []string,
	excludes []string,
	setProfile func(*T, string),
	apply func(*T, string, []string),
) (T, bool) {
	setProfile(&options, rootedCoverageProfile(root, profile))
	return optionsForModuleGoFiles(options, snapshot, moduleRoot, root, includes, excludes, apply)
}

func optionsForModuleGoFiles[T any](
	options T,
	snapshot repo.Snapshot,
	moduleRoot string,
	root string,
	includes []string,
	excludes []string,
	apply func(*T, string, []string),
) (T, bool) {
	moduleOptions := options
	files, ok := scopedGoFilePathsForModule(snapshot, moduleRoot, includes, excludes)
	if !ok {
		var zero T
		return zero, false
	}
	apply(&moduleOptions, moduleToolRoot(root, moduleRoot), files)
	return moduleOptions, true
}

func scopedGoFilePathsForModule(snapshot repo.Snapshot, moduleRoot string, includes []string, excludes []string) ([]string, bool) {
	if len(includes) == 0 && len(excludes) == 0 {
		return nil, true
	}
	files := goFilePathsForModule(snapshot, moduleRoot, includes, excludes)
	if len(files) == 0 {
		return nil, false
	}
	return files, true
}

func goFilePathsForModule(snapshot repo.Snapshot, moduleRoot string, includes []string, excludes []string) []string {
	roots := goIncludeRoots(moduleRoot, includes)
	moduleRoots := goModuleRoots(snapshot)
	files := make([]string, 0)
	for filePath := range snapshot.Files {
		if !strings.HasSuffix(filePath, ".go") ||
			isUnderOtherModuleRoot(filePath, moduleRoot, moduleRoots) ||
			!isUnderGoRoot(filePath, roots) ||
			isGoFileExcluded(filePath, moduleRoot, excludes) {
			continue
		}
		files = append(files, trimModuleRoot(filePath, moduleRoot))
	}
	sort.Strings(files)
	return files
}

func goIncludeRoots(moduleRoot string, includes []string) []string {
	if len(includes) == 0 {
		return []string{moduleRoot}
	}
	roots := make([]string, 0, len(includes))
	for _, include := range includes {
		root, ok := goIncludeRoot(moduleRoot, include)
		if ok {
			roots = append(roots, root)
		}
	}
	return roots
}

func goIncludeRoot(moduleRoot string, include string) (string, bool) {
	if strings.TrimSpace(include) == "" {
		return "", false
	}
	include = cleanSlashPath(include)
	switch {
	case include == ".":
		return moduleRoot, true
	case moduleRoot == ".":
		return include, true
	case include == moduleRoot || strings.HasPrefix(include, moduleRoot+"/"):
		return include, true
	default:
		return "", false
	}
}

func isUnderGoRoot(filePath string, roots []string) bool {
	for _, root := range roots {
		if root == "." || filePath == root || strings.HasPrefix(filePath, root+"/") {
			return true
		}
	}
	return false
}

func isUnderOtherModuleRoot(filePath string, moduleRoot string, moduleRoots []string) bool {
	for _, otherRoot := range moduleRoots {
		if otherRoot == "." || otherRoot == moduleRoot {
			continue
		}
		if moduleRoot != "." && !strings.HasPrefix(otherRoot, moduleRoot+"/") {
			continue
		}
		if strings.HasPrefix(filePath, otherRoot+"/") {
			return true
		}
	}
	return false
}

func isGoFileExcluded(filePath string, moduleRoot string, excludes []string) bool {
	modulePath := trimModuleRoot(filePath, moduleRoot)
	for _, exclude := range excludes {
		exclude = cleanSlashPath(exclude)
		if exclude == "" {
			continue
		}
		if pathMatchesGoPattern(filePath, exclude) || pathMatchesGoPattern(modulePath, exclude) {
			return true
		}
	}
	return false
}

func pathMatchesGoPattern(filePath string, pattern string) bool {
	matched, _ := doublestar.Match(pattern, filePath)
	return matched
}

func mutationOptionsForModules(options toolchecks.MutationOptions, snapshot repo.Snapshot) []toolchecks.MutationOptions {
	targets := mutationTargets(options)
	if len(targets) == 0 {
		options.Root = firstGoToolRoot(options.Root, snapshot)
		return []toolchecks.MutationOptions{options}
	}

	moduleRoots := goModuleRoots(snapshot)
	if len(moduleRoots) == 0 {
		options.Target = ""
		options.Targets = targets
		return []toolchecks.MutationOptions{options}
	}

	byRoot := map[string][]string{}
	for _, target := range targets {
		moduleRoot := targetModuleRoot(target, moduleRoots)
		toolRoot := moduleToolRoot(options.Root, moduleRoot)
		byRoot[toolRoot] = append(byRoot[toolRoot], trimModuleRoot(target, moduleRoot))
	}

	roots := make([]string, 0, len(byRoot))
	for root := range byRoot {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	grouped := make([]toolchecks.MutationOptions, 0, len(roots))
	for _, root := range roots {
		grouped = append(grouped, toolchecks.MutationOptions{
			Root:    root,
			Targets: byRoot[root],
			Scan:    options.Scan,
		})
	}
	return grouped
}

func firstGoToolRoot(root string, snapshot repo.Snapshot) string {
	roots := goToolRoots(root, snapshot)
	return roots[0]
}

func moduleToolRoot(root string, moduleRoot string) string {
	if moduleRoot == "." {
		return commandRoot(root)
	}
	return filepath.Join(commandRoot(root), filepath.FromSlash(moduleRoot))
}

func rootedCoverageProfile(root string, profile string) string {
	if profile == "" || filepath.IsAbs(profile) {
		return profile
	}
	absolute, err := filepath.Abs(filepath.Join(commandRoot(root), profile))
	if err != nil {
		return filepath.Join(commandRoot(root), profile)
	}
	return absolute
}

func goModuleRoots(snapshot repo.Snapshot) []string {
	roots := make([]string, 0)
	for filePath := range snapshot.Files {
		if path.Base(filePath) != "go.mod" || isSkippedGoModulePath(filePath) {
			continue
		}
		root := path.Dir(filePath)
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func goModuleRootsOrDefault(snapshot repo.Snapshot) []string {
	roots := goModuleRoots(snapshot)
	if len(roots) > 0 {
		return roots
	}
	return []string{"."}
}

func targetModuleRoot(target string, moduleRoots []string) string {
	for i := len(moduleRoots) - 1; i >= 0; i-- {
		moduleRoot := moduleRoots[i]
		if moduleRoot == "." || target == moduleRoot || strings.HasPrefix(target, moduleRoot+"/") {
			return moduleRoot
		}
	}
	return "."
}

func trimModuleRoot(target string, moduleRoot string) string {
	if moduleRoot == "." {
		return target
	}
	return strings.TrimPrefix(strings.TrimPrefix(target, moduleRoot), "/")
}

func cleanSlashPath(filePath string) string {
	return path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
}

func mutationTargets(options toolchecks.MutationOptions) []string {
	if options.Target != "" {
		return []string{options.Target}
	}
	targets := make([]string, 0, len(options.Targets))
	for _, target := range options.Targets {
		if strings.TrimSpace(target) != "" {
			targets = append(targets, target)
		}
	}
	return targets
}

func isSkippedGoModulePath(filePath string) bool {
	return filePath == "vendor/go.mod" ||
		strings.Contains(filePath, "/vendor/") ||
		strings.HasPrefix(filePath, "fixtures/") ||
		strings.HasPrefix(filePath, "templates/")
}
