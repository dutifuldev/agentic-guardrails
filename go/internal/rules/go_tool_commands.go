package rules

import (
	"path"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/gotools"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

func contentHasGoToolCommand(content string, tool gotools.Tool) bool {
	for _, tokens := range commandTokensByLine(content) {
		if lineHasGoToolCommand(tokens, tool) {
			return true
		}
	}
	return false
}

func lineHasGoToolCommand(tokens []string, tool gotools.Tool) bool {
	for i, token := range tokens {
		token = cleanCommandToken(token)
		if isGoToolPackageToken(token, tool.Package) {
			if isGoRunPackage(tokens, i) {
				return true
			}
			continue
		}
		if isToolBinaryToken(token, tool.Binary) {
			return isCommandToken(tokens, i)
		}
	}
	return false
}

func contentHasSlophammerGoCommand(content string, subcommand string, requiredFlag string) bool {
	return contentHasCommandLine(content, func(tokens []string) bool {
		return lineHasSlophammerGoCommand(tokens, subcommand, requiredFlag)
	})
}

func fileHasConfigBackedSlophammerGoCommand(file repo.File, subcommand string) bool {
	if isWorkflowFilePath(file.Path) {
		for _, block := range workflowCommandBlocks(file.Content) {
			configRootPath := workflowBlockConfigRootPath(block)
			if contentHasConfigBackedSlophammerGoCommand(workflowBlockRunContent(block), subcommand, configRootPath) {
				return true
			}
		}
		return false
	}
	for _, content := range commandSections(file) {
		if contentHasConfigBackedSlophammerGoCommand(content, subcommand, ".") {
			return true
		}
	}
	return false
}

func fileHasConfigBackedSlophammerGoCheckExecuteCommand(file repo.File) bool {
	if isWorkflowFilePath(file.Path) {
		for _, block := range workflowCommandBlocks(file.Content) {
			configRootPath := workflowBlockConfigRootPath(block)
			if contentHasConfigBackedSlophammerGoCheckExecuteCommand(workflowBlockRunContent(block), configRootPath) {
				return true
			}
		}
		return false
	}
	for _, content := range commandSections(file) {
		if contentHasConfigBackedSlophammerGoCheckExecuteCommand(content, ".") {
			return true
		}
	}
	return false
}

func contentHasConfigBackedSlophammerGoCommand(content string, subcommand string, configRootPath string) bool {
	matcher := configBackedSlophammerGoCommandMatcher{
		subcommand:     subcommand,
		configRootPath: configRootPath,
	}
	return contentHasCommandLine(content, matcher.match)
}

func contentHasConfigBackedSlophammerGoCheckExecuteCommand(content string, configRootPath string) bool {
	return contentHasCommandLine(content, func(tokens []string) bool {
		return lineHasConfigBackedSlophammerGoCheckExecuteCommand(tokens, configRootPath)
	})
}

type configBackedSlophammerGoCommandMatcher struct {
	subcommand     string
	configRootPath string
}

func (m configBackedSlophammerGoCommandMatcher) match(tokens []string) bool {
	return lineHasConfigBackedSlophammerGoCommand(tokens, m.subcommand, m.configRootPath)
}

func contentHasCommandLine(content string, match func([]string) bool) bool {
	for _, tokens := range commandTokensByLine(content) {
		if match(tokens) {
			return true
		}
	}
	return false
}

func lineHasSlophammerGoCommand(tokens []string, subcommand string, requiredFlag string) bool {
	for i := 0; i < len(tokens); i++ {
		argsStart, ok := slophammerGoCommandArgsStart(tokens, i, subcommand)
		if ok && lineHasRequiredFlag(tokens[argsStart:], requiredFlag) {
			return true
		}
	}
	return false
}

func lineHasConfigBackedSlophammerGoCommand(tokens []string, subcommand string, configRootPath string) bool {
	for i := 0; i < len(tokens); i++ {
		argsStart, ok := slophammerGoCommandArgsStart(tokens, i, subcommand)
		if !ok {
			continue
		}
		if lineHasConfigRootArgument(tokens[:i], tokens[argsStart:], configRootPath) {
			return true
		}
	}
	return false
}

func lineHasConfigBackedSlophammerGoCheckExecuteCommand(tokens []string, configRootPath string) bool {
	for i := 0; i < len(tokens); i++ {
		argsStart, ok := slophammerGoCommandArgsStart(tokens, i, "check")
		if !ok {
			continue
		}
		args := tokens[argsStart:]
		if !lineHasBooleanFlag(args, "--execute") {
			continue
		}
		if lineHasConfigRootArgument(tokens[:i], args, configRootPath) {
			return true
		}
	}
	return false
}

func slophammerGoCommandArgsStart(tokens []string, commandIndex int, subcommand string) (int, bool) {
	token := cleanCommandToken(tokens[commandIndex])
	if !isSlophammerCommandToken(token) {
		return 0, false
	}
	if !isCommandToken(tokens, commandIndex) && !isGoRunPackage(tokens, commandIndex) {
		return 0, false
	}
	argsStart, ok := directSlophammerGoArgsStart(tokens, commandIndex, subcommand)
	if !ok {
		argsStart, ok = legacySlophammerGoArgsStart(tokens, commandIndex, subcommand)
	}
	if !ok {
		return 0, false
	}
	if subcommand == "mutate" && nonExecutingMutationFlagInArgs(tokens, argsStart) {
		return 0, false
	}
	return argsStart, true
}

// A scan only counts mutation sites and --update-manifest only rewrites
// the embedded manifest; neither runs a mutant, so neither can fail on a
// survivor. Only the matched command's own arguments are inspected: the
// check stops at the next shell separator, so flags of a later command
// never discredit it.
func nonExecutingMutationFlagInArgs(tokens []string, argsStart int) bool {
	for _, token := range tokens[argsStart:] {
		if shellSeparatorToken(token) {
			return false
		}
		if nonExecutingMutationFlagToken(cleanCommandToken(token)) {
			return true
		}
	}
	return false
}

// The mutate CLI rejects value-form spellings such as --scan=false as
// unknown options, so any spelling of these flags marks a command that
// either skips the gate or does not run at all.
var nonExecutingMutationFlags = []string{"--scan", "--update-manifest"}

func nonExecutingMutationFlagToken(token string) bool {
	for _, flag := range nonExecutingMutationFlags {
		if token == flag || strings.HasPrefix(token, flag+"=") {
			return true
		}
	}
	return false
}

func shellSeparatorToken(token string) bool {
	switch token {
	case "&&", "||", ";", "|":
		return true
	default:
		return false
	}
}

func directSlophammerGoArgsStart(tokens []string, commandIndex int, subcommand string) (int, bool) {
	if commandIndex+1 >= len(tokens) || cleanCommandToken(tokens[commandIndex+1]) != subcommand {
		return 0, false
	}
	return commandIndex + 2, true
}

func legacySlophammerGoArgsStart(tokens []string, commandIndex int, subcommand string) (int, bool) {
	if commandIndex+2 >= len(tokens) ||
		cleanCommandToken(tokens[commandIndex+1]) != "go" ||
		cleanCommandToken(tokens[commandIndex+2]) != subcommand {
		return 0, false
	}
	return commandIndex + 3, true
}

func lineHasConfigRootArgument(prefix []string, tokens []string, configRootPath string) bool {
	if workingDirectory, ok := priorCDWorkingDirectory(prefix); ok {
		configRootPath = configRootPathForWorkingDirectory(workingDirectory)
	}
	if token, ok := firstSlophammerGoPathArgument(tokens); ok {
		return pathIsConfigRootArgument(token, configRootPath)
	}
	return path.Clean(configRootPath) == "."
}

func firstSlophammerGoPathArgument(tokens []string) (string, bool) {
	for i := 0; i < len(tokens); i++ {
		token := cleanCommandToken(tokens[i])
		switch {
		case token == "":
			continue
		case isShellSeparator(token):
			return "", false
		case strings.HasPrefix(token, "-"):
			if slophammerGoFlagConsumesNext(token) {
				i++
			}
		default:
			return token, true
		}
	}
	return "", false
}

func slophammerGoFlagConsumesNext(token string) bool {
	return slophammerGoFlagNeedsValue(token) && !strings.Contains(token, "=")
}

func priorCDWorkingDirectory(tokens []string) (string, bool) {
	workingDirectory := ""
	for i := 0; i+1 < len(tokens); i++ {
		if cleanCommandToken(tokens[i]) != "cd" || !isCommandToken(tokens, i) {
			continue
		}
		next := cleanCommandToken(tokens[i+1])
		if next == "" || strings.HasPrefix(next, "-") || isShellSeparator(next) {
			continue
		}
		workingDirectory = next
	}
	return workingDirectory, workingDirectory != ""
}

func slophammerGoFlagNeedsValue(token string) bool {
	flag, _, _ := strings.Cut(token, "=")
	switch flag {
	case "--coverage-profile", "--format", "--max-candidates", "--max-score", "--profile", "--target":
		return true
	default:
		return false
	}
}

func pathIsConfigRootArgument(token string, configRootPath string) bool {
	cleaned := path.Clean(strings.ReplaceAll(token, "\\", "/"))
	return cleaned == path.Clean(configRootPath)
}

func workflowBlockConfigRootPath(block string) string {
	workingDirectory := ""
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "working-directory:") {
			continue
		}
		workingDirectory = strings.TrimSpace(strings.TrimPrefix(trimmed, "working-directory:"))
	}
	if workingDirectory == "" {
		return "."
	}
	return configRootPathForWorkingDirectory(workingDirectory)
}

func configRootPathForWorkingDirectory(workingDirectory string) string {
	cleaned := path.Clean(strings.ReplaceAll(cleanCommandToken(workingDirectory), "\\", "/"))
	if cleaned == "." || cleaned == "/" {
		return "."
	}
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	parents := make([]string, 0, len(parts))
	for range parts {
		parents = append(parents, "..")
	}
	return path.Join(parents...)
}

func commandTokensByLine(content string) [][]string {
	lines := strings.Split(content, "\n")
	tokenLines := make([][]string, 0, len(lines))
	for _, line := range lines {
		tokens := commandTokens(line)
		if len(tokens) > 0 {
			tokenLines = append(tokenLines, tokens)
		}
	}
	return tokenLines
}

func commandTokens(line string) []string {
	var normalized strings.Builder
	var quote rune
	for _, r := range line {
		switch {
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case quote == r:
			quote = 0
		case quote == 0 && r == ';':
			normalized.WriteString(" ; ")
			continue
		}
		normalized.WriteRune(r)
	}
	return strings.Fields(strings.ReplaceAll(normalized.String(), "$(", " ; "))
}

func isGoRunPackage(tokens []string, packageIndex int) bool {
	for goIndex := 0; goIndex < packageIndex; goIndex++ {
		if cleanCommandToken(tokens[goIndex]) != "go" || !isCommandToken(tokens, goIndex) {
			continue
		}
		commandIndex := goCommandIndex(tokens, goIndex+1)
		if commandIndex == -1 || cleanCommandToken(tokens[commandIndex]) != "run" {
			continue
		}
		if goRunPackageIndex(tokens, commandIndex+1) == packageIndex {
			return true
		}
	}
	return false
}

func goCommandIndex(tokens []string, start int) int {
	return goArgumentIndex(tokens, start, goGlobalFlagNeedsValue)
}

func goGlobalFlagNeedsValue(token string) bool {
	flag, _, _ := strings.Cut(token, "=")
	return flag == "-C"
}

func goRunPackageIndex(tokens []string, start int) int {
	return goArgumentIndex(tokens, start, goRunFlagNeedsValue)
}

func goArgumentIndex(tokens []string, start int, flagNeedsValue func(string) bool) int {
	for i := start; i < len(tokens); i++ {
		token := cleanCommandToken(tokens[i])
		if token == "" {
			continue
		}
		if isShellSeparator(token) {
			return -1
		}
		if strings.HasPrefix(token, "-") {
			if flagNeedsValue(token) && !strings.Contains(token, "=") {
				i++
			}
			continue
		}
		return i
	}
	return -1
}

func goRunFlagNeedsValue(token string) bool {
	flag, _, _ := strings.Cut(token, "=")
	switch flag {
	case "-asmflags", "-exec", "-gcflags", "-ldflags", "-mod", "-overlay", "-p", "-pkgdir", "-tags", "-toolexec":
		return true
	default:
		return false
	}
}

func isCommandToken(tokens []string, commandIndex int) bool {
	if commandIndex == 0 {
		return true
	}
	previous := cleanCommandToken(tokens[commandIndex-1])
	return isShellSeparator(previous) || hasCommandPrefix(tokens, commandIndex)
}

func hasCommandPrefix(tokens []string, commandIndex int) bool {
	prefixIndex := commandIndex - 1
	for prefixIndex >= 0 && isEnvAssignmentToken(cleanCommandToken(tokens[prefixIndex])) {
		prefixIndex--
	}
	if prefixIndex < 0 {
		return true
	}
	prefix := cleanCommandToken(tokens[prefixIndex])
	if isShellSeparator(prefix) {
		return true
	}
	return prefix == "env" && isCommandToken(tokens, prefixIndex)
}

func isEnvAssignmentToken(token string) bool {
	name, _, ok := strings.Cut(token, "=")
	return ok && shellNamePattern.MatchString(name)
}

func lineHasRequiredFlag(tokens []string, requiredFlag string) bool {
	if requiredFlag == "" {
		return true
	}
	for i, token := range tokens {
		if isShellSeparator(cleanCommandToken(token)) {
			return false
		}
		if cleanCommandToken(token) == requiredFlag {
			return hasFlagValue(tokens, i)
		}
	}
	return false
}

func lineHasBooleanFlag(tokens []string, flag string) bool {
	for _, token := range tokens {
		cleaned := cleanCommandToken(token)
		if isShellSeparator(cleaned) {
			return false
		}
		if cleaned == flag {
			return true
		}
	}
	return false
}

func hasFlagValue(tokens []string, flagIndex int) bool {
	if flagIndex+1 >= len(tokens) {
		return false
	}
	value := cleanCommandToken(tokens[flagIndex+1])
	return value != "" && !strings.HasPrefix(value, "-") && !isShellSeparator(value)
}

func isGoToolPackageToken(token string, packageNeedle string) bool {
	return strings.Contains(token, packageNeedle)
}

func isToolBinaryToken(token string, binaryName string) bool {
	return path.Base(token) == binaryName
}

func isSlophammerCommandToken(token string) bool {
	base := path.Base(stripGoModuleVersion(token))
	switch base {
	case "slophammer", "slophammer.exe", "slophammer-go", "slophammer-go.exe":
		return true
	default:
		return false
	}
}

func stripGoModuleVersion(token string) string {
	if before, _, ok := strings.Cut(token, "@"); ok {
		return before
	}
	return token
}

func cleanCommandToken(token string) string {
	return strings.Trim(token, " \t\r\n'\"")
}

func isShellSeparator(token string) bool {
	switch token {
	case ";", "|", "||", "&", "&&":
		return true
	default:
		return false
	}
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-13T01:26:13+08:00","module_hash":"8c6bd197c0b3480990c7dce5536eb3439c990474cb2a5fc673cae0bb9f05ad1d","functions":[{"id":"func/contentHasGoToolCommand","name":"contentHasGoToolCommand","line":11,"end_line":18,"hash":"104429191b5008441c044633b923868d5ff295cb8bff2a5cb6ce8f21a269aa71"},{"id":"func/lineHasGoToolCommand","name":"lineHasGoToolCommand","line":20,"end_line":34,"hash":"0b487471731781d84807a80e88a3948b4d5daef5615bdc79827dd558dd261c0b"},{"id":"func/contentHasSlophammerGoCommand","name":"contentHasSlophammerGoCommand","line":36,"end_line":40,"hash":"f8fe9a87d3277c2226f0b42c21138287ebde497faf8a85a2fe0e5232a4dd071b"},{"id":"func/fileHasConfigBackedSlophammerGoCommand","name":"fileHasConfigBackedSlophammerGoCommand","line":42,"end_line":58,"hash":"f264c4493e9b0b87f01dfe7ec67ec4868eb2e726921e1f096022ea372d636e3c"},{"id":"func/fileHasConfigBackedSlophammerGoCheckExecuteCommand","name":"fileHasConfigBackedSlophammerGoCheckExecuteCommand","line":60,"end_line":76,"hash":"8c752202311ebb8521a4e3812d154e8bfcad3654a2756706c3d90374b038991c"},{"id":"func/contentHasConfigBackedSlophammerGoCommand","name":"contentHasConfigBackedSlophammerGoCommand","line":78,"end_line":84,"hash":"e63b1767b66982140fadb9c211c8092d427c6412d53b08d93c8209d8cf66df25"},{"id":"func/contentHasConfigBackedSlophammerGoCheckExecuteCommand","name":"contentHasConfigBackedSlophammerGoCheckExecuteCommand","line":86,"end_line":90,"hash":"3906afe0d879bc15e25cefbb3575262a68a22c4e180725737822b648fe5f0bc7"},{"id":"func/configBackedSlophammerGoCommandMatcher.match","name":"configBackedSlophammerGoCommandMatcher.match","line":97,"end_line":99,"hash":"51a71c93530e312d5527af28d2157d09e378223761430c7a9c12d6c7a19556c2"},{"id":"func/contentHasCommandLine","name":"contentHasCommandLine","line":101,"end_line":108,"hash":"8760eab5fd355e7bd0633a65512a64542c119e887c3bde2a0c3b7ab5e4e8b463"},{"id":"func/lineHasSlophammerGoCommand","name":"lineHasSlophammerGoCommand","line":110,"end_line":118,"hash":"5adad91cdadd08279dec3d84b7899ce8131ba9b1827c50af60065477b99eada8"},{"id":"func/lineHasConfigBackedSlophammerGoCommand","name":"lineHasConfigBackedSlophammerGoCommand","line":120,"end_line":131,"hash":"c9c7df763851052ecef119e5f8d686570393dfc540260f5af009057c8a3a27da"},{"id":"func/lineHasConfigBackedSlophammerGoCheckExecuteCommand","name":"lineHasConfigBackedSlophammerGoCheckExecuteCommand","line":133,"end_line":148,"hash":"b66ed019f8ec01e294c546c4e06cd2c068db02279090449dc5c001c4dc9fc999"},{"id":"func/slophammerGoCommandArgsStart","name":"slophammerGoCommandArgsStart","line":150,"end_line":169,"hash":"c8c8d6e44c65310181d7ee6e0dc17e4e46a666168a64ce6c7fbe2b0194ddb4de"},{"id":"func/nonExecutingMutationFlagInArgs","name":"nonExecutingMutationFlagInArgs","line":176,"end_line":186,"hash":"af5dfa1740a556247ee18d5ca82b801229f7a128251a861408e13cc1384b98bd"},{"id":"func/nonExecutingMutationFlagToken","name":"nonExecutingMutationFlagToken","line":193,"end_line":200,"hash":"df1e797aeca9dcf586108f4596596ec2e2de30f45c4f8fdb5b10bf30f5992ea0"},{"id":"func/shellSeparatorToken","name":"shellSeparatorToken","line":202,"end_line":209,"hash":"bf1e9bbaa105195344a0f3a09358b9480783161d8925584af8c034c25050bce5"},{"id":"func/directSlophammerGoArgsStart","name":"directSlophammerGoArgsStart","line":211,"end_line":216,"hash":"202a33c978d699f6365cd9ae04a90eec76f4e2fd0f42f6f0eb03e94464430652"},{"id":"func/legacySlophammerGoArgsStart","name":"legacySlophammerGoArgsStart","line":218,"end_line":225,"hash":"a559b3808f720923de30d24cbd94ef9bf98ffca6c8d52ad6e2121a806023253a"},{"id":"func/lineHasConfigRootArgument","name":"lineHasConfigRootArgument","line":227,"end_line":235,"hash":"724f64529281ebff6e90be515e721026eb254651260a0f35c83dd5937ecad4b5"},{"id":"func/firstSlophammerGoPathArgument","name":"firstSlophammerGoPathArgument","line":237,"end_line":254,"hash":"805f96c1e228fd7f208eedbaa559532e2a708643d7681390bb8974d2a2b1104b"},{"id":"func/slophammerGoFlagConsumesNext","name":"slophammerGoFlagConsumesNext","line":256,"end_line":258,"hash":"e20175d7d16d39877db384f744dca3a7172bdb56e96fe6c5a3e5f3eaee3a9e77"},{"id":"func/priorCDWorkingDirectory","name":"priorCDWorkingDirectory","line":260,"end_line":273,"hash":"0ead31f84d1b2d6a8eea8a00886f4eb30be085e92274fbd72cb130fd4211daf8"},{"id":"func/slophammerGoFlagNeedsValue","name":"slophammerGoFlagNeedsValue","line":275,"end_line":283,"hash":"bffa90a5f4eb5a6cb01696485b23faecdd4cbd5afecab339a7e8a6041d2dc6b8"},{"id":"func/pathIsConfigRootArgument","name":"pathIsConfigRootArgument","line":285,"end_line":288,"hash":"2c04bfe68cfd97eb4f3f6c04b53fc35ff9790adb40cd8cd3bb058b291f8b87d7"},{"id":"func/workflowBlockConfigRootPath","name":"workflowBlockConfigRootPath","line":290,"end_line":303,"hash":"d56affa7dec5bb1627e4ba5d24814151213746fb5f93f3760f3c7396676b9f49"},{"id":"func/configRootPathForWorkingDirectory","name":"configRootPathForWorkingDirectory","line":305,"end_line":316,"hash":"4dcdc3228d40dda80a853142e614aa0e79f37ed35e5913820f0f244cdf8f15d1"},{"id":"func/commandTokensByLine","name":"commandTokensByLine","line":318,"end_line":328,"hash":"473038f0426a24c6b4e363d959eede2e11c464546fcb63cfcd890bdf78202960"},{"id":"func/commandTokens","name":"commandTokens","line":330,"end_line":346,"hash":"f32fec70071f8663fa0419d71a03f5fb3366b435caec75882b32f349910fcf74"},{"id":"func/isGoRunPackage","name":"isGoRunPackage","line":348,"end_line":362,"hash":"ad3d5e36fe615c72533773df6e748424102f4325c76ec6aad1ea9e0a63389a33"},{"id":"func/goCommandIndex","name":"goCommandIndex","line":364,"end_line":366,"hash":"eb772c36d8fd0e0f5653b7df42df51af5f0006b66eb49846d823bcc3d8f184d0"},{"id":"func/goGlobalFlagNeedsValue","name":"goGlobalFlagNeedsValue","line":368,"end_line":371,"hash":"773b5ed6501aed46bc93ec5ec2d45a63fdd26e949e68c11efc801db9e804aeb4"},{"id":"func/goRunPackageIndex","name":"goRunPackageIndex","line":373,"end_line":375,"hash":"9783a968a8fed4a3d9d0c84a19aad85e923616c2883c87108265bf223bebcf26"},{"id":"func/goArgumentIndex","name":"goArgumentIndex","line":377,"end_line":395,"hash":"e898590f0a844d84ae742af52cf2bdceb2aa554f40ea6f5146eacb856f09d0b2"},{"id":"func/goRunFlagNeedsValue","name":"goRunFlagNeedsValue","line":397,"end_line":405,"hash":"873931bc6faa45419d8a2f4f4299de8a4311cc2bedb20aa10e82d1f1cb1070df"},{"id":"func/isCommandToken","name":"isCommandToken","line":407,"end_line":413,"hash":"a9b9fbcd907311524054882a43d5bad01f4151675c0dfb47d2a008cd47e52c33"},{"id":"func/hasCommandPrefix","name":"hasCommandPrefix","line":415,"end_line":428,"hash":"2e25b4507e788fc9ff74f12c192a9eb5284295d18b4daf19406b7f9aa9a596c2"},{"id":"func/isEnvAssignmentToken","name":"isEnvAssignmentToken","line":430,"end_line":433,"hash":"86d6bac4dc482b400b9a51d1ac7ca35b524e40ad5ae1c8485901158446c4dc9b"},{"id":"func/lineHasRequiredFlag","name":"lineHasRequiredFlag","line":435,"end_line":448,"hash":"02adf5dd9094d88475f1c363add5b996b1dd602d17cc4c9e019d26dd9d468061"},{"id":"func/lineHasBooleanFlag","name":"lineHasBooleanFlag","line":450,"end_line":461,"hash":"ecd862dc9a099e064ff2cfcb381dfba1649411181f06ae645760e9299caf7cdc"},{"id":"func/hasFlagValue","name":"hasFlagValue","line":463,"end_line":469,"hash":"b720a7659efb56c5cf4579893bc6b82ee9e04c16e9d904b492cd08ca6ca63d20"},{"id":"func/isGoToolPackageToken","name":"isGoToolPackageToken","line":471,"end_line":473,"hash":"ea241843ddfbe22a80886214806aabb44093e2242e059e18ce71ffb0ce4b52f6"},{"id":"func/isToolBinaryToken","name":"isToolBinaryToken","line":475,"end_line":477,"hash":"af955e9348f52943509c3c84bd4db4e73043284ddd80fa339594f9d3bd75bfbb"},{"id":"func/isSlophammerCommandToken","name":"isSlophammerCommandToken","line":479,"end_line":487,"hash":"e9b24403598f4df1548d1d838a5f3729036c92c0679b2ad5df726f4da7242935"},{"id":"func/stripGoModuleVersion","name":"stripGoModuleVersion","line":489,"end_line":494,"hash":"12076cdee75472d106e824e660cacc6773161b9d8b7faad7ba25d7b14ff2f65a"},{"id":"func/cleanCommandToken","name":"cleanCommandToken","line":496,"end_line":498,"hash":"639174e51127b3ec50424aa7adcc2cbf6779bbc33ea5bcce69003430f6dea969"},{"id":"func/isShellSeparator","name":"isShellSeparator","line":500,"end_line":507,"hash":"b6931917d87be3dbbe6fc1bcb3bfa1ad5906f2ca331864555c880e6711d97bb9"}]}
// mutate4go-manifest-end
