package rules

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func goProjectRoots(snapshot repo.Snapshot) []string {
	rootsByPath := goModuleRootSet(snapshot)
	if len(rootsByPath) == 0 && hasUnscopedGoSignal(snapshot) {
		rootsByPath[""] = struct{}{}
	}
	if len(rootsByPath) > 0 && hasGoSourceOutsideModuleRoots(snapshot, rootsByPath) {
		rootsByPath[""] = struct{}{}
	}
	return sortedRootSet(rootsByPath)
}

func goModuleRootSet(snapshot repo.Snapshot) map[string]struct{} {
	rootsByPath := map[string]struct{}{}
	for filePath := range snapshot.Files {
		if !isEmbeddedFixturePath(filePath) && strings.EqualFold(path.Base(filePath), "go.mod") {
			rootsByPath[goModuleRootPath(filePath)] = struct{}{}
		}
	}
	return rootsByPath
}

func goModuleRootPath(filePath string) string {
	root := path.Dir(filePath)
	if root == "." {
		return ""
	}
	return root
}

func sortedRootSet(rootsByPath map[string]struct{}) []string {
	roots := make([]string, 0, len(rootsByPath))
	for root := range rootsByPath {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func hasUnscopedGoSignal(snapshot repo.Snapshot) bool {
	for filePath := range snapshot.Files {
		if isEmbeddedFixturePath(filePath) {
			continue
		}
		if strings.HasSuffix(filePath, ".go") {
			return true
		}
	}
	return hasRunnableCommandLine(goProjectSnapshot(snapshot, "", []string{""}), lineHasGoCommandSignal)
}

func hasGoSourceOutsideModuleRoots(snapshot repo.Snapshot, rootsByPath map[string]struct{}) bool {
	for filePath := range snapshot.Files {
		if isEmbeddedFixturePath(filePath) || !strings.HasSuffix(filePath, ".go") {
			continue
		}
		if !isUnderAnyGoRoot(filePath, rootsByPath) {
			return true
		}
	}
	return false
}

func isUnderAnyGoRoot(filePath string, rootsByPath map[string]struct{}) bool {
	for root := range rootsByPath {
		if root == "" {
			return true
		}
		if strings.HasPrefix(filePath, root+"/") {
			return true
		}
	}
	return false
}

func goProjectSnapshot(snapshot repo.Snapshot, root string, roots []string) repo.Snapshot {
	files := map[string]repo.File{}
	priorities := map[string]int{}
	hasLocalGoConfig := hasModuleLocalGoConfig(snapshot, root)
	for filePath, file := range snapshot.Files {
		if hasLocalGoConfig && isRepoRootGoConfigFile(filePath) {
			continue
		}
		scopedFile, ok := scopedGoProjectFile(filePath, file, root, roots)
		if !ok {
			continue
		}
		priority := scopedGoProjectFilePriority(filePath, root)
		if priorities[scopedFile.Path] > priority {
			continue
		}
		files[scopedFile.Path] = scopedFile
		priorities[scopedFile.Path] = priority
	}
	return repo.NewSnapshot(snapshot.Root, files)
}

func hasModuleLocalGoConfig(snapshot repo.Snapshot, root string) bool {
	if root == "" {
		return false
	}
	return snapshot.HasFileFold(root+"/.golangci.yml") || snapshot.HasFileFold(root+"/.golangci.yaml")
}

func scopedGoProjectFilePriority(filePath string, root string) int {
	if root != "" && strings.HasPrefix(filePath, root+"/") {
		return 2
	}
	return 1
}

func scopedGoProjectFile(filePath string, file repo.File, root string, roots []string) (repo.File, bool) {
	if isEmbeddedFixturePath(filePath) {
		return repo.File{}, false
	}
	if isWorkflowFilePath(filePath) {
		content, ok := scopedWorkflowContent(file.Content, root, roots)
		if !ok {
			return repo.File{}, false
		}
		return repo.File{Path: file.Path, Content: content}, true
	}
	if root == "" {
		return file, !isUnderOtherGoRoot(filePath, root, roots)
	}
	return scopedNestedGoProjectFile(filePath, file, root, roots)
}

func scopedNestedGoProjectFile(filePath string, file repo.File, root string, roots []string) (repo.File, bool) {
	if isRepoRootSlophammerConfigFile(filePath) {
		return file, true
	}
	if isRepoRootGoConfigFile(filePath) {
		return file, true
	}
	if isRepoRootCommandFile(filePath) {
		content, ok := scopedRootCommandContent(filePath, file.Content, root, roots)
		if !ok {
			return repo.File{}, false
		}
		return repo.File{Path: file.Path, Content: content}, true
	}
	prefix := root + "/"
	if !strings.HasPrefix(filePath, prefix) || isUnderOtherGoRoot(filePath, root, roots) {
		return repo.File{}, false
	}
	scopedPath := strings.TrimPrefix(filePath, prefix)
	return repo.File{Path: scopedPath, Content: file.Content}, true
}

func isUnderOtherGoRoot(filePath, root string, roots []string) bool {
	for _, otherRoot := range roots {
		if otherRoot == "" || otherRoot == root {
			continue
		}
		if root == "" && strings.HasPrefix(filePath, otherRoot+"/") {
			return true
		}
		if strings.HasPrefix(otherRoot, root+"/") && strings.HasPrefix(filePath, otherRoot+"/") {
			return true
		}
	}
	return false
}

// scopedWorkflowContent drops neutralized workflow evidence before scoping it
// per module root, so structural neutralization (continue-on-error,
// literal-false conditions, non-integration triggers) cannot be bypassed
// through the module-scoping path.
func scopedWorkflowContent(content, root string, roots []string) (string, bool) {
	if filtered, ok := bindingFilteredWorkflow(content); ok {
		content = filtered
	}
	if strings.TrimSpace(content) == "" {
		return "", true
	}
	if onlyGoRoot(root, roots) {
		return content, true
	}
	return filterWorkflowContentForRoot(content, root, roots)
}

func scopedRootCommandContent(filePath string, content string, root string, roots []string) (string, bool) {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	inRootBlock := false
	carryRootContext := carriesRootCommandContext(filePath)
	for _, line := range lines {
		if workflowMentionsOtherGoRoot(line, root, roots) {
			inRootBlock = false
			continue
		}
		if workflowMentionsGoRoot(line, root, roots) {
			inRootBlock = carryRootContext
			kept = append(kept, line)
			continue
		}
		if carryRootContext && inRootBlock {
			if lineHasCDCommand(line) {
				inRootBlock = false
				continue
			}
			kept = append(kept, line)
		}
	}
	scoped := strings.Join(kept, "\n")
	return scoped, strings.TrimSpace(scoped) != ""
}

func filterWorkflowContentForRoot(content, root string, roots []string) (string, bool) {
	blocks := workflowStepBlocks(content)
	kept := make([]string, 0, len(blocks))
	for _, block := range blocks {
		scopedBlock, ok := scopedWorkflowStepBlock(block, root, roots)
		if ok {
			kept = append(kept, scopedBlock)
		}
	}
	scoped := strings.Join(kept, workflowStepBoundary)
	return scoped, strings.TrimSpace(scoped) != ""
}

func onlyGoRoot(root string, roots []string) bool {
	return root == "" && len(roots) == 1 && roots[0] == root
}

func workflowStepAppliesToRoot(content, root string, roots []string) bool {
	if root == "" {
		return !workflowMentionsOtherGoRoot(content, root, roots)
	}
	return workflowMentionsGoRoot(content, root, roots)
}

func scopedWorkflowStepBlock(content, root string, roots []string) (string, bool) {
	if !workflowStepAppliesToRoot(content, root, roots) {
		return "", false
	}
	if root == "" || !workflowMentionsOtherGoRoot(content, root, roots) {
		return content, true
	}
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	inRootBlock := false
	for _, line := range lines {
		keep, active := scopedWorkflowStepLine(line, root, roots, inRootBlock)
		inRootBlock = active
		if keep {
			kept = append(kept, line)
			continue
		}
		runLine, ok := workflowRunLine(strings.TrimSpace(line))
		if _, block := workflowRunBlockScalar(runLine); ok && block {
			kept = append(kept, line)
		}
	}
	scoped := strings.Join(kept, "\n")
	return scoped, strings.TrimSpace(scoped) != ""
}

func scopedWorkflowStepLine(line string, root string, roots []string, inRootBlock bool) (bool, bool) {
	if workflowMentionsOtherGoRoot(line, root, roots) || (inRootBlock && lineHasCDCommand(line)) {
		return false, false
	}
	if workflowMentionsGoRoot(line, root, roots) {
		return true, true
	}
	return inRootBlock, inRootBlock
}

func workflowStepBlocks(content string) []string {
	lines := strings.Split(content, "\n")
	scan := workflowStepScan{}
	for _, line := range lines {
		scan.visitLine(line)
	}
	blocks := appendWorkflowStepBlock(scan.blocks, scan.current)
	if len(blocks) == 0 {
		return []string{content}
	}
	return blocks
}

type workflowStepScan struct {
	blocks        []string
	globalContext []string
	jobContext    []string
	current       []string
	inJobs        bool
	inSteps       bool
	seenJob       bool
}

func (s *workflowStepScan) visitLine(line string) {
	if s.visitWorkflowStructure(line) {
		return
	}
	if len(s.current) == 0 && isWorkflowWorkingDirectory(line) {
		s.recordWorkingDirectory(line)
	}
	if s.inJobs && s.inSteps && isWorkflowStepStart(line) {
		s.startStep(line)
		return
	}
	if len(s.current) > 0 {
		s.current = append(s.current, line)
	}
}

func (s *workflowStepScan) visitWorkflowStructure(line string) bool {
	if s.enterJobs(line) {
		return true
	}
	if s.inJobs && isWorkflowJobStart(line) {
		s.startJob()
		return true
	}
	if s.inJobs && isWorkflowStepsStart(line) {
		s.inSteps = true
		return true
	}
	return false
}

func (s *workflowStepScan) enterJobs(line string) bool {
	if strings.TrimSpace(line) != "jobs:" {
		return false
	}
	s.inJobs = true
	return true
}

func (s *workflowStepScan) startJob() {
	s.blocks = appendWorkflowStepBlock(s.blocks, s.current)
	s.current = nil
	s.jobContext = append([]string{}, s.globalContext...)
	s.inSteps = false
	s.seenJob = true
}

func (s *workflowStepScan) recordWorkingDirectory(line string) {
	if s.seenJob {
		s.jobContext = append(s.jobContext, line)
		return
	}
	s.globalContext = append(s.globalContext, line)
	s.jobContext = append([]string{}, s.globalContext...)
}

func (s *workflowStepScan) startStep(line string) {
	s.blocks = appendWorkflowStepBlock(s.blocks, s.current)
	s.current = append(append([]string{}, s.jobContext...), line)
}

func appendWorkflowStepBlock(blocks []string, lines []string) []string {
	if len(lines) == 0 {
		return blocks
	}
	if !workflowBlockHasRun(lines) {
		lines = withoutWorkflowRunDefaults(lines)
	}
	if len(lines) == 0 {
		return blocks
	}
	return append(blocks, strings.Join(lines, "\n"))
}

func workflowBlockHasRun(lines []string) bool {
	for _, line := range lines {
		if _, ok := workflowRunLine(strings.TrimSpace(line)); ok {
			return true
		}
	}
	return false
}

func withoutWorkflowRunDefaults(lines []string) []string {
	for i, line := range lines {
		if isWorkflowStepStart(line) {
			return lines[i:]
		}
	}
	return lines
}

func workflowMentionsOtherGoRoot(content, root string, roots []string) bool {
	for _, otherRoot := range roots {
		if otherRoot != "" && otherRoot != root && workflowMentionsGoRoot(content, otherRoot, roots) {
			return true
		}
	}
	return false
}

func lineHasCDCommand(line string) bool {
	tokens := strings.Fields(line)
	for i, token := range tokens {
		if cleanCommandToken(token) == "cd" && isCommandToken(tokens, i) {
			return true
		}
	}
	return false
}

func workflowMentionsGoRoot(content, root string, roots []string) bool {
	normalized := strings.ReplaceAll(content, "\\", "/")
	return workflowReferencesRootExact(normalized, root, roots, workingDirectoryPattern(root)) ||
		workflowReferencesRootExact(normalized, root, roots, cdRootPattern(root)) ||
		workflowReferencesRootExact(normalized, root, roots, goCFlagRootPattern(root)) ||
		workflowReferencesRootSubpath(normalized, root, roots)
}

func workflowReferencesRootExact(content, root string, roots []string, pattern *regexp.Regexp) bool {
	for _, match := range pattern.FindAllStringIndex(content, -1) {
		if !rootPathIsNestedModule(content[match[0]:], root, roots) {
			return true
		}
	}
	return false
}

func workflowReferencesRootSubpath(content, root string, roots []string) bool {
	for _, match := range rootSubpathPattern(root).FindAllStringIndex(content, -1) {
		if !rootPathIsNestedModule(content[match[0]:], root, roots) {
			return true
		}
	}
	return false
}

func rootPathIsNestedModule(match, root string, roots []string) bool {
	start := strings.TrimLeft(match, " \t\r\n'\";:&|()[]{}")
	start = strings.TrimPrefix(start, "./")
	for _, otherRoot := range roots {
		if otherRoot == "" || otherRoot == root || !strings.HasPrefix(otherRoot, root+"/") {
			continue
		}
		if strings.HasPrefix(start, otherRoot+"/") || hasRootPathBoundary(start, otherRoot) {
			return true
		}
	}
	return false
}

func hasRootPathBoundary(value, root string) bool {
	if !strings.HasPrefix(value, root) {
		return false
	}
	if len(value) == len(root) {
		return true
	}
	return strings.ContainsRune(" \t\r\n'\";:&|)]}", rune(value[len(root)]))
}

func workingDirectoryPattern(root string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)\bworking-directory:\s*['"]?(?:\./)?` + regexp.QuoteMeta(root) + `['"]?(?:[[:space:]]|$)`)
}

func cdRootPattern(root string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)(?:^|[[:space:];&|])cd\s+['"]?(?:\./)?` + regexp.QuoteMeta(root) + `['"]?(?:[[:space:];&|]|$)`)
}

func goCFlagRootPattern(root string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)(?:^|[[:space:];&|])go\s+-C(?:=|\s+)['"]?(?:\./)?` + regexp.QuoteMeta(root) + `['"]?(?:[[:space:];&|]|$)`)
}

func rootSubpathPattern(root string) *regexp.Regexp {
	return regexp.MustCompile(`(?:^|[^[:alnum:]_./-])(?:\./)?` + regexp.QuoteMeta(root) + `/`)
}

func isWorkflowStepStart(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "- ")
}

func isWorkflowStepsStart(line string) bool {
	return strings.TrimSpace(line) == "steps:"
}

func isWorkflowJobStart(line string) bool {
	if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
		trimmed := strings.TrimSpace(line)
		return strings.HasSuffix(trimmed, ":")
	}
	return false
}

func isWorkflowWorkingDirectory(line string) bool {
	return strings.Contains(strings.TrimSpace(line), "working-directory:")
}

func isWorkflowFilePath(filePath string) bool {
	dir, name := path.Split(filePath)
	return dir == ".github/workflows/" && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
}

func isRepoRootGoConfigFile(filePath string) bool {
	return filePath == ".golangci.yml" || filePath == ".golangci.yaml"
}

func isRepoRootSlophammerConfigFile(filePath string) bool {
	return filePath == "slophammer.yml" || filePath == "slophammer.yaml"
}

func isRepoRootCommandFile(filePath string) bool {
	switch filePath {
	case "Makefile", "Taskfile.yml", "Taskfile.yaml", "justfile":
		return true
	default:
		return strings.HasPrefix(filePath, "scripts/")
	}
}

func carriesRootCommandContext(filePath string) bool {
	return strings.HasPrefix(filePath, "scripts/")
}

var embeddedFixtureSegments = map[string]struct{}{
	"examples":  {},
	"fixtures":  {},
	"samples":   {},
	"templates": {},
	"testdata":  {},
	"vendor":    {},
}

func isEmbeddedFixturePath(filePath string) bool {
	return pathHasAnySegment(filePath, embeddedFixtureSegments)
}

func hasCommand(snapshot repo.Snapshot, needles ...string) bool {
	return repo.ContainsAny(commandFiles(snapshot), needles...)
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-12T22:50:23+08:00","module_hash":"2a48c1c4ecbdb88c9c038f26f095faa2bf62079af4bf27b2932a008c885ad9e5","functions":[{"id":"func/goProjectRoots","name":"goProjectRoots","line":12,"end_line":21,"hash":"aea3a16fb3cd5c9374dc82f86d78bb4d00d051a53ab040c748dad1022352deba"},{"id":"func/goModuleRootSet","name":"goModuleRootSet","line":23,"end_line":31,"hash":"3fa81e200252106e2d936b020f90ac49c2806cb30d42a82db2731f34005d7f75"},{"id":"func/goModuleRootPath","name":"goModuleRootPath","line":33,"end_line":39,"hash":"85d9ca1d4bedd12456af4c4ff6a89135e235ea69d571ce41178a4af975e31421"},{"id":"func/sortedRootSet","name":"sortedRootSet","line":41,"end_line":48,"hash":"bf45018317d34fddb88928ab5e778f1821e635b09bc681729027a6e44a9e2ab4"},{"id":"func/hasUnscopedGoSignal","name":"hasUnscopedGoSignal","line":50,"end_line":60,"hash":"118a5e8004b30899cfd624d88658e009f00fc732265389bf016479d4d8714f10"},{"id":"func/hasGoSourceOutsideModuleRoots","name":"hasGoSourceOutsideModuleRoots","line":62,"end_line":72,"hash":"37bb7a061f00d70fb924cc41bcf90b9c052e90485ede96a857c4e46391959508"},{"id":"func/isUnderAnyGoRoot","name":"isUnderAnyGoRoot","line":74,"end_line":84,"hash":"141e1953499c0db05e45cc57081be13ebddb87727d5c8a45c9e3d5bea6e73d4c"},{"id":"func/goProjectSnapshot","name":"goProjectSnapshot","line":86,"end_line":106,"hash":"2d62ae6dba821f8835fc93aaaa4b3fa1350ecb8b20ee1712ea4263b60452d42a"},{"id":"func/hasModuleLocalGoConfig","name":"hasModuleLocalGoConfig","line":108,"end_line":113,"hash":"3805a8949bf17805c98879e83ea0517178e442b210bdefb6ab912e70d1ceed56"},{"id":"func/scopedGoProjectFilePriority","name":"scopedGoProjectFilePriority","line":115,"end_line":120,"hash":"625afc7695beed16cb0684fcc6398a79349fad421029f94938e922dd875e86ae"},{"id":"func/scopedGoProjectFile","name":"scopedGoProjectFile","line":122,"end_line":137,"hash":"af88becbad3314d15229ace5c2bbcd9326e20de1db6c485463a8bd043a9594be"},{"id":"func/scopedNestedGoProjectFile","name":"scopedNestedGoProjectFile","line":139,"end_line":159,"hash":"be05048894b59f3d04d2defecb9fd339a943bb4fef96267bf3ecd1d8f6aa8950"},{"id":"func/isUnderOtherGoRoot","name":"isUnderOtherGoRoot","line":161,"end_line":174,"hash":"126ae7fdf2a707143c0acc48056c46965e8bad3b3efa8c865fcfe5e7b17fb9bb"},{"id":"func/scopedWorkflowContent","name":"scopedWorkflowContent","line":180,"end_line":191,"hash":"703cee2066a847a3be9ef2f3006c66e88db919224c230ba77541e5e616659e57"},{"id":"func/scopedRootCommandContent","name":"scopedRootCommandContent","line":193,"end_line":218,"hash":"cfe55502d4ac4f8317c10e180a6f241c6bfd5c19e18b171ee3df344576d7a175"},{"id":"func/filterWorkflowContentForRoot","name":"filterWorkflowContentForRoot","line":220,"end_line":231,"hash":"99e2481ebe44ecc140695d33dae48b908eb69f4b77e0bb12d6834e11130ac784"},{"id":"func/onlyGoRoot","name":"onlyGoRoot","line":233,"end_line":235,"hash":"e4179f945213b7c38dcb5876674a789639883de54d233e950536df63cd1f9e8b"},{"id":"func/workflowStepAppliesToRoot","name":"workflowStepAppliesToRoot","line":237,"end_line":242,"hash":"31a9078dc94ec2bf113c2cd210606272f668d1713961f931ccbfc9ff0cbe5839"},{"id":"func/scopedWorkflowStepBlock","name":"scopedWorkflowStepBlock","line":244,"end_line":268,"hash":"0a2ff2cfd3f44724f4ec174c14d398fcea085522a459846e6bd51e8791976cba"},{"id":"func/scopedWorkflowStepLine","name":"scopedWorkflowStepLine","line":270,"end_line":278,"hash":"bfa3297c0ef82be2bf71189d2bb8812f75b9829de78fcf830848a014f3854a74"},{"id":"func/workflowStepBlocks","name":"workflowStepBlocks","line":280,"end_line":291,"hash":"76bf242f18d4767d8ef9e4799c188ed0dce112a29f4859f94266bb4feaff697f"},{"id":"func/workflowStepScan.visitLine","name":"workflowStepScan.visitLine","line":303,"end_line":317,"hash":"78275de992724023a9b4560f57249284200b77f78f8c03097cc2818bcda236b7"},{"id":"func/workflowStepScan.visitWorkflowStructure","name":"workflowStepScan.visitWorkflowStructure","line":319,"end_line":332,"hash":"315bb8f5361502732638239271daa97c3b5416e6e0b7661570190621ce57add0"},{"id":"func/workflowStepScan.enterJobs","name":"workflowStepScan.enterJobs","line":334,"end_line":340,"hash":"71352e7b8a7de3dc7093efe953e868a938eb08485d2b1beb8339eb9d0f39e02c"},{"id":"func/workflowStepScan.startJob","name":"workflowStepScan.startJob","line":342,"end_line":348,"hash":"4cb5a32a6ea91cfadf04b3c8abcb931667564c91abca28accf45d3d9e43282df"},{"id":"func/workflowStepScan.recordWorkingDirectory","name":"workflowStepScan.recordWorkingDirectory","line":350,"end_line":357,"hash":"73f9d7c43d85b06529df8561687ea62856060a8557c5ad7bbb7e85b346355ff6"},{"id":"func/workflowStepScan.startStep","name":"workflowStepScan.startStep","line":359,"end_line":362,"hash":"4c1ce96f07ede8079d20f1491976c625d8633eefe1edd8569b5d88f51ec6ec99"},{"id":"func/appendWorkflowStepBlock","name":"appendWorkflowStepBlock","line":364,"end_line":375,"hash":"4f8b39b933c1d04886bf92eb2f86d531ea4b62eed28483e2b780a73872c8baa4"},{"id":"func/workflowBlockHasRun","name":"workflowBlockHasRun","line":377,"end_line":384,"hash":"b4d7a202e4c7882df7c9e144d858b07889f15e54cf21f0ae638779077742b886"},{"id":"func/withoutWorkflowRunDefaults","name":"withoutWorkflowRunDefaults","line":386,"end_line":393,"hash":"fd981d1944c18fcfdcb7bf1d2f45fc20da3a5dd0957213a729515aa5ca3a6b90"},{"id":"func/workflowMentionsOtherGoRoot","name":"workflowMentionsOtherGoRoot","line":395,"end_line":402,"hash":"d637771ee359a3b398202374e02cf51ff4872123a654db2b6efcd4a0c5b8641f"},{"id":"func/lineHasCDCommand","name":"lineHasCDCommand","line":404,"end_line":412,"hash":"f0a2b1b6b2fb4c88eb8cda218a0695c37514da4a5c114e432b311b9088cd9f00"},{"id":"func/workflowMentionsGoRoot","name":"workflowMentionsGoRoot","line":414,"end_line":420,"hash":"54db03e7c4035c589cb9f66fe370c11816de9b7b5507f7a3448e99cf9a063f3a"},{"id":"func/workflowReferencesRootExact","name":"workflowReferencesRootExact","line":422,"end_line":429,"hash":"eec0ea6d88e58c5875c058e44c4d1bfe123f6bbd7a7686f6c1ac5c27f4a08341"},{"id":"func/workflowReferencesRootSubpath","name":"workflowReferencesRootSubpath","line":431,"end_line":438,"hash":"f2ea28c821d1a0ff1eb0020898068727f14297b49633ae8d9a24bbe1d12b2cb0"},{"id":"func/rootPathIsNestedModule","name":"rootPathIsNestedModule","line":440,"end_line":452,"hash":"8305db668e322a54004b1def9a3ff039d611a4c1b5df8866f8f05a7640a9c8df"},{"id":"func/hasRootPathBoundary","name":"hasRootPathBoundary","line":454,"end_line":462,"hash":"20339e2eca7e558da74d65608382aab86482ceea4cf47db538b2a1b5ccf0022e"},{"id":"func/workingDirectoryPattern","name":"workingDirectoryPattern","line":464,"end_line":466,"hash":"6236814674f916bbdbd7137c15b1b5b789d7b86dbc0e641b234158cba0194a6d"},{"id":"func/cdRootPattern","name":"cdRootPattern","line":468,"end_line":470,"hash":"eb28b59b67483a84500cfd45c8d510770e1d9af65fb741e113f77e7d9a3e1edb"},{"id":"func/goCFlagRootPattern","name":"goCFlagRootPattern","line":472,"end_line":474,"hash":"5e954a0a65dc56a0a700d87797d022aa2156e90dd20952237cc16a2918fea949"},{"id":"func/rootSubpathPattern","name":"rootSubpathPattern","line":476,"end_line":478,"hash":"3d8281879d717b7df1c822ee7d300fac07e62d1d64de30fa706e296eba1832e0"},{"id":"func/isWorkflowStepStart","name":"isWorkflowStepStart","line":480,"end_line":483,"hash":"90b362033feb94736cdfa4700f1583a727db7ffdfa549d5df7e96248f1d961b2"},{"id":"func/isWorkflowStepsStart","name":"isWorkflowStepsStart","line":485,"end_line":487,"hash":"97cd3ebfa7124c323337d22e0e4425719149d5a555a24bd784be986e157ae67b"},{"id":"func/isWorkflowJobStart","name":"isWorkflowJobStart","line":489,"end_line":495,"hash":"2fe509303ac2c4d6a96c389523407be37e13ccc708a1490c9a018f80d07cabea"},{"id":"func/isWorkflowWorkingDirectory","name":"isWorkflowWorkingDirectory","line":497,"end_line":499,"hash":"682b2c25ee04d61445a15b597226ad905fc73f2cab6eabde56ee909141f9a002"},{"id":"func/isWorkflowFilePath","name":"isWorkflowFilePath","line":501,"end_line":504,"hash":"d56f65f6cc2f3da87f0abdaef3674b3c94e74eae21ad2b9da9aaaac7eac12cba"},{"id":"func/isRepoRootGoConfigFile","name":"isRepoRootGoConfigFile","line":506,"end_line":508,"hash":"40ee8778aa00589828e8bd6c378fe3d02a8e3004c23a730828a195ca16db23dc"},{"id":"func/isRepoRootSlophammerConfigFile","name":"isRepoRootSlophammerConfigFile","line":510,"end_line":512,"hash":"9126d0df48f398d5b55b340a3a9aa72235d5c955ef6a4d51ecf6c173052f6e14"},{"id":"func/isRepoRootCommandFile","name":"isRepoRootCommandFile","line":514,"end_line":521,"hash":"eadebae5c1e8201dcb43ac5ea37705a5954eafee8ec151de7fac114a41ca45e2"},{"id":"func/carriesRootCommandContext","name":"carriesRootCommandContext","line":523,"end_line":525,"hash":"f9eada7429e7a24957e281e7babcf5021b037cf92483f1ae71f35dd60948d890"},{"id":"func/isEmbeddedFixturePath","name":"isEmbeddedFixturePath","line":536,"end_line":538,"hash":"fb3ac23e39f3d1f9fdfde2cc9fd93a7ed02fbc2f801ed667f923d8dd781bf941"},{"id":"func/hasCommand","name":"hasCommand","line":540,"end_line":542,"hash":"602374c541cd47b54d2baa8bf5d6a88e0ebdb11536ad39d714b858595208695c"}]}
// mutate4go-manifest-end
