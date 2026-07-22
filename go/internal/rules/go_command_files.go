package rules

import (
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func commandFiles(snapshot repo.Snapshot) []repo.File {
	return preparedCommandFiles(commandFileMap(snapshot))
}

func commandFileMap(snapshot repo.Snapshot) map[string]repo.File {
	filesByPath := map[string]repo.File{}
	evidence := addBindingWorkflowFiles(snapshot, filesByPath)
	addReachableCommandFiles(snapshot, filesByPath, evidence)
	return filesByPath
}

// addBindingWorkflowFiles adds the binding command content of each workflow
// and returns the combined text used as the reachability root.
func addBindingWorkflowFiles(snapshot repo.Snapshot, filesByPath map[string]repo.File) string {
	var evidence strings.Builder
	for _, file := range snapshot.WorkflowFiles() {
		content := bindingWorkflowCommandText(file.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		filesByPath[file.Path] = repo.File{Path: file.Path, Content: content}
		evidence.WriteString(content)
		evidence.WriteString("\n")
	}
	return evidence.String()
}

// addReachableCommandFiles credits scripts, Makefiles, Taskfiles, and
// justfiles only when binding workflow evidence invokes them, following
// script-to-script references one level deep.
func addReachableCommandFiles(snapshot repo.Snapshot, filesByPath map[string]repo.File, evidence string) {
	candidates := commandFileCandidates(snapshot)
	firstHop := reachableCandidates(candidates, evidence)
	extended := evidence + joinedContents(firstHop)
	for _, file := range reachableCandidates(candidates, extended) {
		filesByPath[file.Path] = file
	}
}

func commandFileCandidates(snapshot repo.Snapshot) map[string]repo.File {
	candidates := map[string]repo.File{}
	for _, file := range snapshot.FilesNamedFold("Makefile", "Taskfile.yml", "Taskfile.yaml", "justfile") {
		candidates[file.Path] = file
	}
	for _, file := range snapshot.FilesUnder("scripts") {
		candidates[file.Path] = file
	}
	for _, file := range snapshot.FilesUnder("go/scripts") {
		candidates[file.Path] = file
	}
	for path, file := range snapshot.Files {
		if isScriptPath(path) {
			candidates[file.Path] = file
		}
	}
	return candidates
}

func reachableCandidates(candidates map[string]repo.File, evidence string) []repo.File {
	reachable := make([]repo.File, 0, len(candidates))
	for _, file := range candidates {
		if commandFileReachable(file.Path, evidence) {
			reachable = append(reachable, file)
		}
	}
	return reachable
}

func commandFileReachable(filePath string, evidence string) bool {
	reference, ok := runnerReference(filePath)
	if !ok {
		reference = pathBaseName(filePath)
	}
	return containsCommandWord(evidence, reference)
}

// runnerReference maps runner-driven command files to the command that
// invokes them, since workflows reference the runner rather than the file.
func runnerReference(filePath string) (string, bool) {
	switch strings.ToLower(pathBaseName(filePath)) {
	case "makefile":
		return "make", true
	case "taskfile.yml", "taskfile.yaml":
		return "task", true
	case "justfile":
		return "just", true
	default:
		return "", false
	}
}

func pathBaseName(filePath string) string {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	if index := strings.LastIndex(normalized, "/"); index >= 0 {
		return normalized[index+1:]
	}
	return normalized
}

func containsCommandWord(evidence string, word string) bool {
	for index := strings.Index(evidence, word); index >= 0; {
		if commandWordBoundary(evidence, index, len(word)) {
			return true
		}
		next := strings.Index(evidence[index+1:], word)
		if next < 0 {
			return false
		}
		index += 1 + next
	}
	return false
}

func commandWordBoundary(evidence string, index int, length int) bool {
	if index > 0 && isCommandWordByte(evidence[index-1]) {
		return false
	}
	end := index + length
	return end >= len(evidence) || !isCommandWordByte(evidence[end])
}

func isCommandWordByte(value byte) bool {
	switch {
	case value >= 'a' && value <= 'z', value >= 'A' && value <= 'Z', value >= '0' && value <= '9':
		return true
	case value == '_', value == '-':
		return true
	default:
		return false
	}
}

func joinedContents(files []repo.File) string {
	var joined strings.Builder
	for _, file := range files {
		joined.WriteString(file.Content)
		joined.WriteString("\n")
	}
	return joined.String()
}

func preparedCommandFiles(filesByPath map[string]repo.File) []repo.File {
	files := make([]repo.File, 0, len(filesByPath))
	for _, file := range filesByPath {
		file.Content = stripCommentLines(file.Content)
		file.Content = joinShellContinuations(file.Content)
		if strings.TrimSpace(file.Content) != "" {
			files = append(files, file)
		}
	}
	return files
}

// commandSections returns a file's command content. Workflow files are
// already reduced to their binding run scripts during preparation.
func commandSections(file repo.File) []string {
	return []string{file.Content}
}

// workflowBlockRunContent extracts a workflow block's run scripts. Blocks
// from parsed workflows are already bare run text with no run: markers and
// pass through with their context lines stripped; scoped step fragments
// still need extraction.
func workflowBlockRunContent(block string) string {
	if strings.Contains(block, "run:") {
		return workflowRunContent(block)
	}
	lines := strings.Split(block, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "working-directory:") || strings.HasPrefix(trimmed, "uses: ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func workflowCommandSections(content string) []string {
	blocks := workflowCommandBlocks(content)
	sections := make([]string, 0, len(blocks))
	for _, block := range blocks {
		runContent := workflowRunContent(block)
		if strings.TrimSpace(runContent) != "" {
			sections = append(sections, runContent)
		}
	}
	return sections
}

func workflowCommandBlocks(content string) []string {
	if strings.Contains(content, workflowStepBoundary) {
		return splitNonEmpty(content, workflowStepBoundary)
	}
	return workflowStepBlocks(content)
}

func workflowRunContent(block string) string {
	scan := workflowRunScan{}
	for _, line := range strings.Split(block, "\n") {
		scan.visitLine(line)
	}
	return scan.content()
}

type workflowRunScan struct {
	kept          []string
	foldedLines   []string
	inRunBlock    bool
	inFoldedBlock bool
	runIndent     int
	contentIndent int
}

func (s *workflowRunScan) visitLine(line string) {
	trimmed := strings.TrimSpace(line)
	if s.inRunBlock {
		if s.recordRunBlockLine(line, trimmed) {
			return
		}
		s.endRunBlock()
	}
	runLine, ok := workflowRunLine(trimmed)
	if ok {
		s.startRun(line, runLine)
		return
	}
}

func (s *workflowRunScan) startRun(line, runLine string) {
	s.flushFolded()
	folded, block := workflowRunBlockScalar(runLine)
	if block {
		s.inRunBlock = true
		s.inFoldedBlock = folded
		s.runIndent = leadingSpaceCount(line)
		s.contentIndent = 0
		return
	}
	s.kept = append(s.kept, runLine)
	s.endRunBlock()
}

func workflowRunBlockScalar(value string) (folded bool, ok bool) {
	if len(value) == 0 {
		return false, false
	}
	switch value[0] {
	case '|':
		folded = false
	case '>':
		folded = true
	default:
		return false, false
	}
	for _, indicator := range value[1:] {
		if !isWorkflowBlockScalarIndicator(indicator) {
			return false, false
		}
	}
	return folded, true
}

func isWorkflowBlockScalarIndicator(indicator rune) bool {
	return indicator == '-' || indicator == '+' || (indicator >= '1' && indicator <= '9')
}

func (s *workflowRunScan) recordRunBlockLine(line, trimmed string) bool {
	if trimmed == "" {
		return true
	}
	indent := leadingSpaceCount(line)
	if s.contentIndent == 0 {
		if indent <= s.runIndent {
			return false
		}
		s.contentIndent = indent
	}
	if indent < s.contentIndent {
		return false
	}
	s.recordRunLine(trimmed)
	return true
}

func (s *workflowRunScan) recordRunLine(line string) {
	if s.inFoldedBlock {
		if line != "" {
			s.foldedLines = append(s.foldedLines, line)
		}
		return
	}
	s.kept = append(s.kept, line)
}

func (s *workflowRunScan) endRunBlock() {
	s.inRunBlock = false
	s.inFoldedBlock = false
	s.runIndent = 0
	s.contentIndent = 0
}

func (s *workflowRunScan) content() string {
	s.flushFolded()
	return strings.Join(s.kept, "\n")
}

func (s *workflowRunScan) flushFolded() {
	if len(s.foldedLines) == 0 {
		return
	}
	s.kept = append(s.kept, strings.TrimSpace(strings.Join(s.foldedLines, " ")))
	s.foldedLines = s.foldedLines[:0]
}

func leadingSpaceCount(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func workflowRunLine(trimmed string) (string, bool) {
	switch {
	case strings.HasPrefix(trimmed, "- run:"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "- run:")), true
	case strings.HasPrefix(trimmed, "run:"):
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "run:")), true
	default:
		return "", false
	}
}

func splitNonEmpty(content string, separator string) []string {
	parts := strings.Split(content, separator)
	sections := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			sections = append(sections, part)
		}
	}
	return sections
}

func isScriptPath(filePath string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))
	return strings.HasPrefix(normalized, "scripts/") || strings.Contains(normalized, "/scripts/")
}

func stripCommentLines(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		beforeComment, _, _ := strings.Cut(line, "#")
		if strings.TrimSpace(beforeComment) == "" {
			continue
		}
		kept = append(kept, beforeComment)
	}
	return strings.Join(kept, "\n")
}

func joinShellContinuations(content string) string {
	content = strings.ReplaceAll(content, "\\\r\n", " ")
	return strings.ReplaceAll(content, "\\\n", " ")
}

func hasRunnableCommandLine(snapshot repo.Snapshot, match func([]string) bool) bool {
	for _, file := range commandFiles(snapshot) {
		for _, content := range commandSections(file) {
			if contentHasCommandLine(content, match) {
				return true
			}
		}
	}
	return false
}

func lineHasGoSubcommandAllPackages(tokens []string, subcommand string) bool {
	for i := 0; i < len(tokens); i++ {
		if cleanCommandToken(tokens[i]) != "go" || !isCommandToken(tokens, i) {
			continue
		}
		commandIndex := goCommandIndex(tokens, i+1)
		if commandIndex == -1 || cleanCommandToken(tokens[commandIndex]) != subcommand {
			continue
		}
		if hasArgumentBeforeSeparator(tokens[commandIndex+1:], "./...") {
			return true
		}
	}
	return false
}

func lineHasGolangCICommand(tokens []string) bool {
	for i, token := range tokens {
		token = cleanCommandToken(token)
		if isToolBinaryToken(token, "golangci-lint") && isCommandToken(tokens, i) {
			return hasArgumentBeforeSeparator(tokens[i+1:], "run")
		}
		if isGoToolPackageToken(token, "github.com/golangci/golangci-lint") && isGoRunPackage(tokens, i) {
			return hasArgumentBeforeSeparator(tokens[i+1:], "run")
		}
	}
	return false
}

func lineHasGoToolCoverCommand(tokens []string) bool {
	for i := 0; i < len(tokens); i++ {
		if cleanCommandToken(tokens[i]) != "go" || !isCommandToken(tokens, i) {
			continue
		}
		commandIndex := goCommandIndex(tokens, i+1)
		if commandIndex == -1 || cleanCommandToken(tokens[commandIndex]) != "tool" {
			continue
		}
		if hasArgumentBeforeSeparator(tokens[commandIndex+1:], "cover") {
			return true
		}
	}
	return false
}

func lineHasGoTestCoverageProfileCommand(tokens []string) bool {
	for i := 0; i < len(tokens); i++ {
		if cleanCommandToken(tokens[i]) != "go" || !isCommandToken(tokens, i) {
			continue
		}
		commandIndex := goCommandIndex(tokens, i+1)
		if commandIndex == -1 || cleanCommandToken(tokens[commandIndex]) != "test" {
			continue
		}
		if hasCoverageProfileFlag(tokens[commandIndex+1:]) {
			return true
		}
	}
	return false
}

func lineHasGoCommandSignal(tokens []string) bool {
	for i := 0; i < len(tokens); i++ {
		if cleanCommandToken(tokens[i]) != "go" || !isCommandToken(tokens, i) {
			continue
		}
		commandIndex := goCommandIndex(tokens, i+1)
		if commandIndex == -1 {
			continue
		}
		switch cleanCommandToken(tokens[commandIndex]) {
		case "build", "mod", "run", "test", "tool", "vet":
			return true
		}
	}
	return false
}

func hasCoverageProfileFlag(tokens []string) bool {
	for i, token := range tokens {
		token = cleanCommandToken(token)
		if isShellSeparator(token) {
			return false
		}
		if strings.HasPrefix(token, "-coverprofile=") && strings.TrimPrefix(token, "-coverprofile=") != "" {
			return true
		}
		if token == "-coverprofile" {
			return hasFlagValue(tokens, i)
		}
	}
	return false
}

func hasArgumentBeforeSeparator(tokens []string, argument string) bool {
	for _, token := range tokens {
		token = cleanCommandToken(token)
		if isShellSeparator(token) {
			return false
		}
		if token == argument {
			return true
		}
	}
	return false
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-12T22:50:20+08:00","module_hash":"74380c1df0e61eb5c209cd7b10d18c383af904cb6f06c2928370df0ef99d0cfc","functions":[{"id":"func/commandFiles","name":"commandFiles","line":9,"end_line":11,"hash":"7e74eedb685cf2f0cce5014a93aa6b07f03f278760a4f38f0c3f4bc3a63c8032"},{"id":"func/commandFileMap","name":"commandFileMap","line":13,"end_line":18,"hash":"b777c1359f8b7a917a82b315ad40a602fd12e99da8ed5af831de931ab880ddcc"},{"id":"func/addBindingWorkflowFiles","name":"addBindingWorkflowFiles","line":22,"end_line":34,"hash":"e30c4899ee201d25b6a64d4e30f4dfc174b569d3c5e1bffd07452f86a434a4d0"},{"id":"func/addReachableCommandFiles","name":"addReachableCommandFiles","line":39,"end_line":46,"hash":"e55adea2954f680a38091ea752c347c7d1a450809990966456deaa399ddfddf4"},{"id":"func/commandFileCandidates","name":"commandFileCandidates","line":48,"end_line":65,"hash":"bab974389713fe3090d27c3a754f6b8bcd0d414482e34ad29f4b55b1bebbe1ad"},{"id":"func/reachableCandidates","name":"reachableCandidates","line":67,"end_line":75,"hash":"c8284b71d9039496cf3f2c809d2c582a4dabf644e79e225fe46c48e59b2e6331"},{"id":"func/commandFileReachable","name":"commandFileReachable","line":77,"end_line":83,"hash":"ddf588409e3ecffe0049741b93078d2057c659d65e6167fefe9480d8a31d6a21"},{"id":"func/runnerReference","name":"runnerReference","line":87,"end_line":98,"hash":"b4c246b3d2c69e588f160513a1b7692371278e41d4ae0c5d146d5727175d854d"},{"id":"func/pathBaseName","name":"pathBaseName","line":100,"end_line":106,"hash":"ab768faf6ba015bf4cbcf4caa3db7d72352bcdcb502daae8b7e8bd8dec765bcb"},{"id":"func/containsCommandWord","name":"containsCommandWord","line":108,"end_line":120,"hash":"14414b775beb662af2d8e626c0f4bff808aac8dfd16d53266204657515b41428"},{"id":"func/commandWordBoundary","name":"commandWordBoundary","line":122,"end_line":128,"hash":"7db3df3d3db8ba20a6e0b0552a1fe9d6021fcd21a4c950ae457d490b731b8dd4"},{"id":"func/isCommandWordByte","name":"isCommandWordByte","line":130,"end_line":139,"hash":"8a31915c659f539c35a3a3ccf445868f6bdb417ff3bb4348b7bd1c41350374f2"},{"id":"func/joinedContents","name":"joinedContents","line":141,"end_line":148,"hash":"2631ede8c37e01b69ec48524c5466ee8849f6297e1503aaf9c140f30a2f3f5a0"},{"id":"func/preparedCommandFiles","name":"preparedCommandFiles","line":150,"end_line":160,"hash":"8bda1a9bee63a6b581b619de26358de88a37cce9ce47a1c70bd3d2293d7fbbb9"},{"id":"func/commandSections","name":"commandSections","line":164,"end_line":166,"hash":"b55161f2bc182a347b8ed8024f80ea00dc194257e81d6086c89268ad2fb56c97"},{"id":"func/workflowBlockRunContent","name":"workflowBlockRunContent","line":172,"end_line":186,"hash":"fea4991aaa0982704caf881ada10aa9b387060f8ebd65f915ef315c804d9b397"},{"id":"func/workflowCommandSections","name":"workflowCommandSections","line":188,"end_line":198,"hash":"9e0131f56e8ba364fc4d0bd88b576f7c234a6620bbc8f4fb4c87af6a168f0a4f"},{"id":"func/workflowCommandBlocks","name":"workflowCommandBlocks","line":200,"end_line":205,"hash":"a0e3ace8dca30e20ca257eb61d24ddc3d9007a5edcf481f9071d035b616e8222"},{"id":"func/workflowRunContent","name":"workflowRunContent","line":207,"end_line":213,"hash":"ad761f7f7ca87764daaa55b233837ad14c08aa05b1b1e25cf30a8cf9429bad9a"},{"id":"func/workflowRunScan.visitLine","name":"workflowRunScan.visitLine","line":224,"end_line":237,"hash":"e3a9f97afb546d02d8170152e7ace5724d4a7f65e882db7806b5ea4913d3f321"},{"id":"func/workflowRunScan.startRun","name":"workflowRunScan.startRun","line":239,"end_line":251,"hash":"407bafaf3f8b4481f39c41c6a71936ca75b0cd0c6199eda8fa76d9a387c2ead1"},{"id":"func/workflowRunBlockScalar","name":"workflowRunBlockScalar","line":253,"end_line":271,"hash":"9d1dd475a5bc498aeaf8f96df286b105f3fb9a126d3a095768576cbb6def4952"},{"id":"func/isWorkflowBlockScalarIndicator","name":"isWorkflowBlockScalarIndicator","line":273,"end_line":275,"hash":"37dd5bf19a253a27b2b61cbedfefaac5205020f3df9ea448b82e23e140cc0120"},{"id":"func/workflowRunScan.recordRunBlockLine","name":"workflowRunScan.recordRunBlockLine","line":277,"end_line":293,"hash":"c43a7b4b47fd58d8e6ba2ef3d171424dc2e87d66407952dee5c6d35b451d8c45"},{"id":"func/workflowRunScan.recordRunLine","name":"workflowRunScan.recordRunLine","line":295,"end_line":303,"hash":"926124c084a6cc4397473f4defee1bcf0c17fc27d7d827010d9212a61f0695df"},{"id":"func/workflowRunScan.endRunBlock","name":"workflowRunScan.endRunBlock","line":305,"end_line":310,"hash":"218ccc551fc591e04ffeb623c45350e0c289135a01ac5d46784636330e9264af"},{"id":"func/workflowRunScan.content","name":"workflowRunScan.content","line":312,"end_line":315,"hash":"fea25fa4721188e9d3494f861a93e64825957db288847c9eb169e53298b20440"},{"id":"func/workflowRunScan.flushFolded","name":"workflowRunScan.flushFolded","line":317,"end_line":323,"hash":"44e94586072d07a6c69426c7eb87c133059050e953bc0bdc2661139b212fab81"},{"id":"func/leadingSpaceCount","name":"leadingSpaceCount","line":325,"end_line":327,"hash":"da65518d712e83c303cf05ccf348a5651c6eb11bda928cc8d4580edc2c409e59"},{"id":"func/workflowRunLine","name":"workflowRunLine","line":329,"end_line":338,"hash":"5219e70e8c517a2572228e0d218d1e88dfb5c761c7ac7dd64b2995b4be313618"},{"id":"func/splitNonEmpty","name":"splitNonEmpty","line":340,"end_line":349,"hash":"c21a5e9245da9d07d00baa61891310d578d631000a12a3521f600639f8812e41"},{"id":"func/isScriptPath","name":"isScriptPath","line":351,"end_line":354,"hash":"a6d07c8976f69a2dd2d55ec06b0b207220fb2231602d8b44541e8c960470b1d5"},{"id":"func/stripCommentLines","name":"stripCommentLines","line":356,"end_line":367,"hash":"df9c425a823d060f9a41d88d5b07007dc3513566b352f881396a1582f56ebc94"},{"id":"func/joinShellContinuations","name":"joinShellContinuations","line":369,"end_line":372,"hash":"9ff3276b8284cabc7109784ac9215013e1aa6bf08803cef49529c352a1e189be"},{"id":"func/hasRunnableCommandLine","name":"hasRunnableCommandLine","line":374,"end_line":383,"hash":"151d07e550e3be1d1794270729a309304c401017000bd17ffe593a2d0ec53384"},{"id":"func/lineHasGoSubcommandAllPackages","name":"lineHasGoSubcommandAllPackages","line":385,"end_line":399,"hash":"2c7202ebc3989115ddbe2dba7c418eb5113faa1820648e683898025a51646511"},{"id":"func/lineHasGolangCICommand","name":"lineHasGolangCICommand","line":401,"end_line":412,"hash":"43117172e4ab8dbf1379ac52316a6816959c2a4d308daec8ee2c5dc84ef5abb2"},{"id":"func/lineHasGoToolCoverCommand","name":"lineHasGoToolCoverCommand","line":414,"end_line":428,"hash":"30f46d564f91e3f991561b8ebf0c3518c83c51b1587867b802f9bc2e1c64c1f1"},{"id":"func/lineHasGoTestCoverageProfileCommand","name":"lineHasGoTestCoverageProfileCommand","line":430,"end_line":444,"hash":"5fbb1ea2887c14fcc00bad8eeee75e61a393928f33f2d0bbc0856169ae328826"},{"id":"func/lineHasGoCommandSignal","name":"lineHasGoCommandSignal","line":446,"end_line":461,"hash":"fb435d8b94139192ccdac106d85e595bb734224a43c6941465197ddadcb68efa"},{"id":"func/hasCoverageProfileFlag","name":"hasCoverageProfileFlag","line":463,"end_line":477,"hash":"041b28844db96a29eb34b53a4b9cf6ff4587fe53968fac959b68fc8087018c62"},{"id":"func/hasArgumentBeforeSeparator","name":"hasArgumentBeforeSeparator","line":479,"end_line":490,"hash":"bf1e1dfdf6420b2daee171c4871cbb70ea22da3c74d43b50b78f8c415b9ecc88"}]}
// mutate4go-manifest-end
