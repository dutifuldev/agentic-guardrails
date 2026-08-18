package agents

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

const (
	StartMarker = "<!-- slophammer:agents:start -->"
	EndMarker   = "<!-- slophammer:agents:end -->"
)

var RuleIDs = []string{
	"repo.agents-required",
	"repo.agents-empty",
	"repo.agents-commands-required",
	"repo.agents-command-invalid",
	"repo.agents-scope-required",
	"repo.agents-stale",
}

var lineEndingPattern = regexp.MustCompile(`\r\n?`)
var pytestDependencyPattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])pytest([^A-Za-z0-9_]|$)`)

var manifestNames = map[string]bool{
	"Cargo.toml":     true,
	"go.mod":         true,
	"package.json":   true,
	"pyproject.toml": true,
}

type PackageArea struct {
	Path      string
	Manifests []string
	Commands  []string
}

type Evidence struct {
	GlobalCommands   []string
	Packages         []PackageArea
	AllCommands      []string
	RenderedCommands []string
}

type Issue struct {
	RuleID  string
	Path    string
	Message string
}

func Derive(snapshot repo.Snapshot) Evidence {
	globalCommands := rootRunnerCommands(snapshot)
	packages := packageAreas(snapshot)
	packageCommands := []string{}
	allCommands := append([]string{}, globalCommands...)
	for _, area := range packages {
		packageCommands = append(packageCommands, area.Commands...)
		allCommands = append(allCommands, area.Commands...)
		for _, command := range area.Commands {
			allCommands = append(allCommands, commandVariant(command))
		}
	}
	renderedCommands := globalCommands
	if len(renderedCommands) == 0 {
		renderedCommands = packageCommands
	}
	return Evidence{
		GlobalCommands:   globalCommands,
		Packages:         packages,
		AllCommands:      unique(allCommands),
		RenderedCommands: unique(renderedCommands),
	}
}

func packageAreas(snapshot repo.Snapshot) []PackageArea {
	byPath := map[string]*PackageArea{}
	for _, file := range snapshot.FilesUnder(".") {
		name := path.Base(file.Path)
		if !supportedManifestFile(file) {
			continue
		}
		packagePath := path.Dir(file.Path)
		area := byPath[packagePath]
		if area == nil {
			area = &PackageArea{Path: packagePath}
			byPath[packagePath] = area
		}
		area.Manifests = append(area.Manifests, name)
		area.Commands = append(area.Commands, manifestCommands(snapshot, file, packagePath)...)
	}
	packagePaths := make([]string, 0, len(byPath))
	for packagePath := range byPath {
		packagePaths = append(packagePaths, packagePath)
	}
	sort.Strings(packagePaths)
	packages := make([]PackageArea, 0, len(packagePaths))
	for _, packagePath := range packagePaths {
		area := byPath[packagePath]
		area.Manifests = uniqueSorted(area.Manifests)
		area.Commands = unique(area.Commands)
		packages = append(packages, *area)
	}
	return packages
}

func supportedManifestFile(file repo.File) bool {
	return manifestNames[path.Base(file.Path)] &&
		!ignoredManifestPath(file.Path) &&
		!unsafePackagePath(file.Path) &&
		!reservedMarkerPackagePath(file.Path)
}

func HasUnsafePackagePath(snapshot repo.Snapshot) bool {
	return hasPackagePath(snapshot, unsafePackagePath)
}

func HasReservedMarkerPackagePath(snapshot repo.Snapshot) bool {
	return hasPackagePath(snapshot, reservedMarkerPackagePath)
}

func hasPackagePath(snapshot repo.Snapshot, matches func(string) bool) bool {
	for _, file := range snapshot.FilesUnder(".") {
		if manifestNames[path.Base(file.Path)] && !ignoredManifestPath(file.Path) && matches(file.Path) {
			return true
		}
	}
	return false
}

func unsafePackagePath(filePath string) bool {
	return strings.ContainsAny(path.Dir(filePath), "\r\n")
}

func reservedMarkerPackagePath(filePath string) bool {
	directory := path.Dir(filePath)
	return strings.Contains(directory, StartMarker) || strings.Contains(directory, EndMarker)
}

func ignoredManifestPath(filePath string) bool {
	ignored := map[string]bool{
		"fixtures": true, "templates": true, "testdata": true, "tests": true, "test": true,
		"scripts": true, "vendor": true,
		"dist": true, "build": true, "target": true, "node_modules": true,
		".venv": true, "coverage": true,
	}
	parts := strings.Split(path.Dir(filePath), "/")
	for _, part := range parts {
		if ignored[part] || (strings.HasPrefix(part, ".") && part != ".") {
			return true
		}
	}
	return false
}

func rootRunnerCommands(snapshot repo.Snapshot) []string {
	type runner struct {
		name    string
		command string
		pattern *regexp.Regexp
	}
	runners := []runner{
		{"Makefile", "make check", regexp.MustCompile(`(?m)^check\s*:`)},
		{"makefile", "make check", regexp.MustCompile(`(?m)^check\s*:`)},
		{"Taskfile.yml", "task check", regexp.MustCompile(`(?m)^\s{0,2}check\s*:`)},
		{"Taskfile.yaml", "task check", regexp.MustCompile(`(?m)^\s{0,2}check\s*:`)},
		{"justfile", "just check", regexp.MustCompile(`(?m)^check\s*:`)},
	}
	commands := []string{}
	for _, item := range runners {
		file, ok := snapshot.Files[item.name]
		if ok && item.pattern.MatchString(file.Content) {
			commands = append(commands, item.command)
		}
	}
	return unique(commands)
}

func manifestCommands(snapshot repo.Snapshot, file repo.File, packagePath string) []string {
	switch path.Base(file.Path) {
	case "go.mod":
		return []string{scopedCommand(packagePath, "go test ./...")}
	case "Cargo.toml":
		command := "cargo test"
		if regexp.MustCompile(`(?m)^\s*\[workspace\]\s*$`).MatchString(file.Content) {
			command = "cargo test --workspace"
		}
		return []string{scopedCommand(packagePath, command)}
	case "package.json":
		return packageJSONCommands(snapshot, file, packagePath)
	case "pyproject.toml":
		command := "python -m compileall ."
		if pyprojectUsesPytest(file.Content) {
			command = "python -m pytest"
			if adjacentFile(snapshot, packagePath, "uv.lock") {
				command = "uv run pytest"
			}
		}
		return []string{scopedCommand(packagePath, command)}
	default:
		return nil
	}
}

type pyprojectState struct {
	sectionDependencies bool
	arrayDependencies   bool
}

func pyprojectUsesPytest(content string) bool {
	state := pyprojectState{}
	for _, rawLine := range strings.Split(content, "\n") {
		line, _, _ := strings.Cut(rawLine, "#")
		if state.acceptsPytest(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

func (s *pyprojectState) acceptsPytest(line string) bool {
	if section, ok := tomlSection(line); ok {
		return s.acceptSection(section)
	}
	if value, ok := dependencyAssignment(line); ok {
		return s.acceptDependency(value)
	}
	return s.acceptLine(line)
}

func (s *pyprojectState) acceptSection(section string) bool {
	section = strings.ToLower(section)
	s.sectionDependencies = strings.Contains(section, "dependenc")
	s.arrayDependencies = false
	return strings.HasPrefix(section, "tool.pytest")
}

func (s *pyprojectState) acceptDependency(value string) bool {
	s.arrayDependencies = strings.Contains(value, "[") && !strings.Contains(value, "]")
	return pytestDependencyPattern.MatchString(value)
}

func (s *pyprojectState) acceptLine(line string) bool {
	matched := (s.sectionDependencies || s.arrayDependencies) && pytestDependencyPattern.MatchString(line)
	if s.arrayDependencies && strings.Contains(line, "]") {
		s.arrayDependencies = false
	}
	return matched
}

func tomlSection(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	return strings.Trim(line, "[] "), true
}

func dependencyAssignment(line string) (string, bool) {
	key, value, ok := strings.Cut(line, "=")
	return value, ok && strings.Contains(strings.ToLower(strings.TrimSpace(key)), "dependenc")
}

func packageJSONCommands(snapshot repo.Snapshot, file repo.File, packagePath string) []string {
	var parsed struct {
		Scripts map[string]any `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(file.Content), &parsed); err != nil {
		return nil
	}
	script := preferredPackageScript(parsed.Scripts)
	if script == "" {
		return nil
	}
	command := packageScriptCommand(packageManager(snapshot, packagePath), script)
	return []string{scopedCommand(packagePath, command)}
}

func preferredPackageScript(scripts map[string]any) string {
	for _, candidate := range []string{"check", "test", "build"} {
		if _, ok := scripts[candidate].(string); ok {
			return candidate
		}
	}
	return ""
}

func packageScriptCommand(manager, script string) string {
	if script == "test" && (manager == "npm" || manager == "yarn") {
		return manager + " test"
	}
	return fmt.Sprintf("%s run %s", manager, script)
}

func packageManager(snapshot repo.Snapshot, packagePath string) string {
	for _, directory := range ancestorPaths(packagePath) {
		if manager := declaredPackageManager(snapshot, directory); manager != "" {
			return manager
		}
		if adjacentFile(snapshot, directory, "pnpm-lock.yaml") {
			return "pnpm"
		}
		if adjacentFile(snapshot, directory, "yarn.lock") {
			return "yarn"
		}
		if adjacentFile(snapshot, directory, "bun.lock") || adjacentFile(snapshot, directory, "bun.lockb") {
			return "bun"
		}
	}
	return "npm"
}

func declaredPackageManager(snapshot repo.Snapshot, packagePath string) string {
	filePath := "package.json"
	if packagePath != "." {
		filePath = packagePath + "/package.json"
	}
	file, ok := snapshot.Files[filePath]
	if !ok {
		return ""
	}
	var parsed struct {
		PackageManager string `json:"packageManager"`
	}
	if json.Unmarshal([]byte(file.Content), &parsed) != nil {
		return ""
	}
	manager, _, _ := strings.Cut(parsed.PackageManager, "@")
	if manager == "npm" || manager == "pnpm" || manager == "yarn" || manager == "bun" {
		return manager
	}
	return ""
}

func ancestorPaths(packagePath string) []string {
	paths := []string{}
	for current := packagePath; ; current = path.Dir(current) {
		paths = append(paths, current)
		if current == "." {
			return paths
		}
	}
}

func adjacentFile(snapshot repo.Snapshot, packagePath, name string) bool {
	filePath := name
	if packagePath != "." {
		filePath = packagePath + "/" + name
	}
	_, ok := snapshot.Files[filePath]
	return ok
}

func commandVariant(command string) string {
	if strings.HasPrefix(command, "cd '") {
		if separator := strings.LastIndex(command, "' && "); separator >= 0 {
			return command[separator+len("' && "):]
		}
	}
	return command
}

func scopedCommand(packagePath, command string) string {
	if packagePath == "." {
		return command
	}
	escaped := strings.ReplaceAll(packagePath, "'", "'\\''")
	return fmt.Sprintf("cd '%s' && %s", escaped, command)
}

func Render(snapshot repo.Snapshot) string {
	evidence := Derive(snapshot)
	return strings.Join([]string{
		"# AGENTS.md",
		"",
		"These instructions apply to this repository.",
		"",
		RenderEvidenceBlock(evidence),
		"",
		"## Working rules",
		"",
		"- Keep changes small and reviewable.",
		"- Add or update tests when behavior changes.",
		"- Run the repository checks before you finish.",
		"- Do not weaken existing checks to make a change pass.",
		"",
	}, "\n")
}

func RenderEvidenceBlock(evidence Evidence) string {
	lines := []string{StartMarker, "## Repository checks", ""}
	if len(evidence.RenderedCommands) > 0 {
		lines = append(lines, "Run these commands before you finish:", "", "```sh")
		lines = append(lines, evidence.RenderedCommands...)
		lines = append(lines, "```")
	} else {
		lines = append(lines, "No verification command could be derived from repository files.")
	}
	lines = append(lines, "", "## Package areas", "")
	if len(evidence.Packages) == 0 {
		lines = append(lines, "- No package manifest was detected.")
	} else {
		for _, area := range evidence.Packages {
			manifests := make([]string, 0, len(area.Manifests))
			for _, manifest := range area.Manifests {
				manifests = append(manifests, "`"+manifest+"`")
			}
			lines = append(lines, fmt.Sprintf("- `%s`: %s", area.Path, strings.Join(manifests, ", ")))
		}
	}
	lines = append(lines, EndMarker)
	return strings.Join(lines, "\n")
}

func Evaluate(snapshot repo.Snapshot) []Issue {
	rootFile, ok := RootFile(snapshot)
	if !ok {
		return []Issue{{RuleID: "repo.agents-required", Path: "AGENTS.md", Message: "AGENTS.md is required"}}
	}
	evidence := Derive(snapshot)
	issues := baseIssues(rootFile.Content, evidence)
	issues = append(issues, managedIssues(rootFile.Content, evidence)...)
	if HasUnsafePackagePath(snapshot) {
		issues = append(issues, Issue{
			RuleID:  "repo.agents-scope-required",
			Path:    "AGENTS.md",
			Message: "Package paths containing newlines cannot be represented safely in AGENTS.md",
		})
	}
	if HasReservedMarkerPackagePath(snapshot) {
		issues = append(issues, Issue{
			RuleID:  "repo.agents-scope-required",
			Path:    "AGENTS.md",
			Message: "Package paths containing Slophammer evidence markers cannot be represented safely in AGENTS.md",
		})
	}
	issues = append(issues, scopeIssues(snapshot, evidence)...)
	return issues
}

func baseIssues(content string, evidence Evidence) []Issue {
	issues := []Issue{}
	if !usefulContent(content) && !containsAnyCommand(content, evidence.AllCommands) {
		issues = append(issues, Issue{RuleID: "repo.agents-empty", Path: "AGENTS.md", Message: "AGENTS.md must contain useful repository instructions"})
	}
	if len(evidence.AllCommands) > 0 && !containsAnyCommand(content, evidence.AllCommands) {
		issues = append(issues, Issue{RuleID: "repo.agents-commands-required", Path: "AGENTS.md", Message: "AGENTS.md must name a verification command supported by the repository"})
	}
	return issues
}

func managedIssues(content string, evidence Evidence) []Issue {
	managed, exists := managedBlock(content)
	if !exists {
		return nil
	}
	invalid := []string{}
	for _, command := range managedCommands(managed) {
		if !slices.Contains(evidence.AllCommands, command) {
			invalid = append(invalid, command)
		}
	}
	issues := []Issue{}
	if len(invalid) > 0 {
		issues = append(issues, Issue{RuleID: "repo.agents-command-invalid", Path: "AGENTS.md", Message: "AGENTS.md contains generated commands without repository evidence: " + strings.Join(invalid, ", ")})
	}
	if duplicateManagedMarkers(content) || managed != RenderEvidenceBlock(evidence) {
		issues = append(issues, Issue{RuleID: "repo.agents-stale", Path: "AGENTS.md", Message: "The Slophammer AGENTS.md evidence block is stale"})
	}
	return issues
}

func duplicateManagedMarkers(content string) bool {
	return strings.Count(content, StartMarker) > 1 || strings.Count(content, EndMarker) > 1
}

func RootFile(snapshot repo.Snapshot) (repo.File, bool) {
	if file, ok := snapshot.Files["AGENTS.md"]; ok {
		return file, true
	}
	for _, file := range snapshot.FilesUnder(".") {
		if !strings.Contains(file.Path, "/") && strings.EqualFold(file.Path, "AGENTS.md") {
			return file, true
		}
	}
	return repo.File{}, false
}

func usefulContent(content string) bool {
	withoutComments := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(content, "")
	kept := []string{}
	for _, line := range strings.Split(withoutComments, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "```" {
			continue
		}
		kept = append(kept, line)
	}
	usefulCharacters := 0
	for _, character := range strings.Join(kept, " ") {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			usefulCharacters++
		}
	}
	return usefulCharacters >= 20
}

func containsAnyCommand(content string, commands []string) bool {
	lines := strings.Split(content, "\n")
	for _, command := range commands {
		if strings.Contains(content, "`"+command+"`") {
			return true
		}
		for _, line := range lines {
			if strings.TrimSpace(line) == command {
				return true
			}
		}
	}
	return false
}

func managedBlock(content string) (string, bool) {
	start := strings.Index(content, StartMarker)
	if start < 0 {
		return "", false
	}
	end := strings.Index(content[start:], EndMarker)
	if end < 0 {
		return normalizeLineEndings(strings.TrimSpace(content[start:])), true
	}
	end += start + len(EndMarker)
	return normalizeLineEndings(strings.TrimSpace(content[start:end])), true
}

func normalizeLineEndings(content string) string {
	return lineEndingPattern.ReplaceAllString(content, "\n")
}

func managedCommands(block string) []string {
	match := regexp.MustCompile("(?s)```sh\\n(.*?)\\n```").FindStringSubmatch(block)
	if len(match) != 2 {
		return nil
	}
	commands := []string{}
	for _, line := range strings.Split(match[1], "\n") {
		if command := strings.TrimSpace(line); command != "" {
			commands = append(commands, command)
		}
	}
	return commands
}

func scopeIssues(snapshot repo.Snapshot, evidence Evidence) []Issue {
	issues := []Issue{}
	for _, area := range evidence.Packages {
		if area.Path == "." {
			continue
		}
		acceptable := append(append([]string{}, evidence.GlobalCommands...), area.Commands...)
		for _, command := range area.Commands {
			acceptable = append(acceptable, commandVariant(command))
		}
		acceptable = unique(acceptable)
		if len(acceptable) == 0 {
			continue
		}
		governing, ok := governingAgentsFile(snapshot, area.Path)
		if ok && containsAnyCommand(governing.Content, acceptable) {
			continue
		}
		findingPath := "AGENTS.md"
		if area.Path != "." {
			findingPath = area.Path + "/AGENTS.md"
		}
		issues = append(issues, Issue{RuleID: "repo.agents-scope-required", Path: findingPath, Message: "Package instructions must name a verification command for this package"})
	}
	return issues
}

func governingAgentsFile(snapshot repo.Snapshot, packagePath string) (repo.File, bool) {
	parts := []string{}
	if packagePath != "." {
		parts = strings.Split(packagePath, "/")
	}
	for length := len(parts); length >= 0; length-- {
		directory := strings.Join(parts[:length], "/")
		wanted := "AGENTS.md"
		if directory != "" {
			wanted = directory + "/AGENTS.md"
		}
		if file, ok := snapshot.Files[wanted]; ok {
			return file, true
		}
		for _, file := range snapshot.FilesUnder(directory) {
			if strings.EqualFold(file.Path, wanted) {
				return file, true
			}
		}
	}
	return repo.File{}, false
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func uniqueSorted(values []string) []string {
	values = unique(values)
	sort.Strings(values)
	return values
}
