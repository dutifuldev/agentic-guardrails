package rules

import (
	"context"
	"fmt"
	"sort"

	"github.com/osolmaz/slophammer/go/internal/agents"
	"github.com/osolmaz/slophammer/go/internal/config"
	"github.com/osolmaz/slophammer/go/internal/repo"
)

type Severity string

const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
)

type Finding struct {
	RuleID    string   `json:"rule_id"`
	Severity  Severity `json:"severity"`
	Path      string   `json:"path"`
	Message   string   `json:"message"`
	Baselined bool     `json:"baselined,omitempty"`
}

type Report struct {
	OK       bool           `json:"ok"`
	Findings []Finding      `json:"findings"`
	Scope    *ScopeCoverage `json:"scope,omitempty"`
}

// ScopeCoverage reports how much of the production surface the configured
// scope covers, so a narrowed scope is visible instead of silent.
type ScopeCoverage struct {
	Scanned         int `json:"scanned"`
	ProductionFiles int `json:"production_files"`
}

type Definition struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Severity    Severity `json:"severity"`
	Path        string   `json:"path"`
	Message     string   `json:"message"`
	Description string   `json:"description"`
	Tool        string   `json:"tool,omitempty"`
	Status      string   `json:"status"`
}

type Metadata struct {
	ID          string
	Severity    Severity
	Description string
}

type Rule interface {
	Metadata() Metadata
	Check(context.Context, repo.Snapshot) []Finding
}

const (
	ReadmeRequiredRuleID          = "repo.readme-required"
	AgentsRequiredRuleID          = "repo.agents-required"
	AgentsEmptyRuleID             = "repo.agents-empty"
	AgentsCommandsRequiredRuleID  = "repo.agents-commands-required"
	AgentsCommandInvalidRuleID    = "repo.agents-command-invalid"
	AgentsScopeRequiredRuleID     = "repo.agents-scope-required"
	AgentsStaleRuleID             = "repo.agents-stale"
	CIRequiredRuleID              = "repo.ci-required"
	SlophammerCIRequiredRuleID    = "repo.slophammer-ci-required"
	GoModuleRequiredRuleID        = "go.module-required"
	GoTestsRequiredRuleID         = "go.tests-required"
	GoVetRequiredRuleID           = "go.vet-required"
	GoLintRequiredRuleID          = "go.lint-required"
	GoCoverageRequiredRuleID      = "go.coverage-required"
	GoComplexityRequiredRuleID    = "go.complexity-required"
	GoDryRequiredRuleID           = "go.dry-required"
	GoCRAPRequiredRuleID          = "go.crap-required"
	GoMutationRequiredRuleID      = "go.mutation-required"
	GoDependencyBoundariesRuleID  = "go.dependency-boundaries-required"
	GoScopeIncompleteRuleID       = "go.scope-incomplete"
	GoSuppressionsJustifiedRuleID = "go.suppressions-justified"
)

var defaultDefinitions = []Definition{
	{
		ID:          ReadmeRequiredRuleID,
		Title:       "README required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "README.md",
		Message:     "README.md is required",
		Description: "The target repo should have a README.md.",
		Status:      "implemented",
	},
	{
		ID:          AgentsRequiredRuleID,
		Title:       "Agent instructions required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "AGENTS.md",
		Message:     "AGENTS.md is required",
		Description: "The target repo should have an AGENTS.md.",
		Status:      "implemented",
	},
	{
		ID:          AgentsEmptyRuleID,
		Title:       "Useful agent instructions required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "AGENTS.md",
		Message:     "AGENTS.md must contain useful repository instructions",
		Description: "The root AGENTS.md must contain useful text after Markdown headings, comments, and whitespace are removed.",
		Status:      "implemented",
	},
	{
		ID:          AgentsCommandsRequiredRuleID,
		Title:       "Agent verification commands required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "AGENTS.md",
		Message:     "AGENTS.md must name a verification command supported by the repository",
		Description: "When verification commands can be derived from repository evidence, the governing AGENTS.md must name at least one of them.",
		Status:      "implemented",
	},
	{
		ID:          AgentsCommandInvalidRuleID,
		Title:       "Generated agent commands must be valid",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "AGENTS.md",
		Message:     "AGENTS.md contains generated commands without repository evidence",
		Description: "Commands in a Slophammer-managed AGENTS.md evidence block must still be supported by current repository evidence.",
		Status:      "implemented",
	},
	{
		ID:          AgentsScopeRequiredRuleID,
		Title:       "Package agent instructions required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "AGENTS.md",
		Message:     "Package instructions must name a verification command for this package",
		Description: "Each package boundary must be governed by an AGENTS.md that names a root umbrella command or a command derived for that package.",
		Status:      "implemented",
	},
	{
		ID:          AgentsStaleRuleID,
		Title:       "Generated agent evidence must be current",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        "AGENTS.md",
		Message:     "The Slophammer AGENTS.md evidence block is stale",
		Description: "A Slophammer-managed AGENTS.md evidence block must match the package and command facts derived from the current repository.",
		Status:      "implemented",
	},
	{
		ID:          CIRequiredRuleID,
		Title:       "CI workflow required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     ".github/workflows must contain at least one .yml or .yaml workflow",
		Description: "The target repo should have a CI workflow under .github/workflows.",
		Status:      "implemented",
	},
	{
		ID:          SlophammerCIRequiredRuleID,
		Title:       "Slophammer enforcement required",
		Category:    "repo",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     "CI must run a Slophammer checker when slophammer.yml is present",
		Description: "A repository that carries slophammer.yml must execute a Slophammer checker from binding CI evidence; config without enforcement is decoration.",
		Status:      "implemented",
	},
	{
		ID:          GoModuleRequiredRuleID,
		Title:       "Go module required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        "go.mod",
		Message:     "Go projects must include a go.mod file",
		Description: "Go projects should be Go modules.",
		Tool:        "go",
		Status:      "implemented",
	},
	{
		ID:          GoTestsRequiredRuleID,
		Title:       "Go tests required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     "Go projects must declare go test ./... in CI or scripts",
		Description: "Go projects should run go test against ./... in an inspectable workflow or script.",
		Tool:        "go test",
		Status:      "implemented",
	},
	{
		ID:          GoVetRequiredRuleID,
		Title:       "Go vet required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     "Go projects must declare go vet ./... in CI or scripts",
		Description: "Go projects should run go vet ./... in an inspectable workflow or script.",
		Tool:        "go vet",
		Status:      "implemented",
	},
	{
		ID:          GoLintRequiredRuleID,
		Title:       "Go lint required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".golangci.yml",
		Message:     "Go projects must configure and declare golangci-lint",
		Description: "Go projects should configure golangci-lint and declare a lint check in CI or scripts.",
		Tool:        "golangci-lint",
		Status:      "implemented",
	},
	{
		ID:          GoCoverageRequiredRuleID,
		Title:       "Go coverage gate required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        "scripts/check-go-coverage.sh",
		Message:     "Go projects must declare a coverage gate",
		Description: "Go projects should enforce coverage with go test coverage output, go tool cover, and a minimum threshold.",
		Tool:        "go test -coverprofile",
		Status:      "implemented",
	},
	{
		ID:          GoComplexityRequiredRuleID,
		Title:       "Go complexity linting required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".golangci.yml",
		Message:     "Go projects must enable a complexity linter",
		Description: "Go projects should enable a complexity linter through golangci-lint.",
		Tool:        "golangci-lint",
		Status:      "implemented",
	},
	{
		ID:          GoDryRequiredRuleID,
		Title:       "Go DRY check required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     "Go projects must declare a DRY check",
		Description: "Go projects should declare Slophammer's unified DRY check for structural and copied-block duplicate detection.",
		Tool:        "slophammer-go dry",
		Status:      "implemented",
	},
	{
		ID:          GoCRAPRequiredRuleID,
		Title:       "Go CRAP check required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     "Go projects must declare crap4go with a threshold",
		Description: "Go projects should declare crap4go with a threshold for complexity and coverage risk scoring.",
		Tool:        "crap4go",
		Status:      "implemented",
	},
	{
		ID:          GoMutationRequiredRuleID,
		Title:       "Go mutation check required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".github/workflows",
		Message:     "Go projects must declare mutate4go",
		Description: "Go projects should declare mutate4go in an inspectable workflow or script. Only executing invocations count: list, scan, check, dry-run, and manifest-only forms cannot fail on a surviving mutant and are not evidence. mutate4go exits zero even when mutants survive, so only the gating slophammer-go wrapper counts.",
		Tool:        "mutate4go",
		Status:      "implemented",
	},
	{
		ID:          GoDependencyBoundariesRuleID,
		Title:       "Go dependency boundaries required",
		Category:    "go",
		Severity:    SeverityError,
		Path:        "slophammer.yml",
		Message:     "Go projects must respect configured dependency boundaries",
		Description: "Go projects should keep imports inside the dependency boundaries declared in slophammer.yml.",
		Status:      "implemented",
	},
	{
		ID:          GoScopeIncompleteRuleID,
		Title:       "Go scope completeness",
		Category:    "go",
		Severity:    SeverityError,
		Path:        "slophammer.yml",
		Message:     "Configured Go scope must cover all production files or exclude them with reasons",
		Description: "Every production Go file must be in configured scope or covered by a conventional or reasoned exclude, so findings cannot be hidden by narrowing scope.",
		Status:      "implemented",
	},
	{
		ID:          GoSuppressionsJustifiedRuleID,
		Title:       "Go suppressions justified",
		Category:    "go",
		Severity:    SeverityError,
		Path:        ".",
		Message:     "nolint directives in production Go code must carry a reason",
		Description: "A //nolint directive in production scope must carry a trailing // reason comment; bare suppressions accumulate silently.",
		Status:      "implemented",
	},
}

func DefaultDefinitions() []Definition {
	return append([]Definition(nil), defaultDefinitions...)
}

func DefaultRules() []Rule {
	ruleSet := make([]Rule, 0, len(defaultDefinitions))
	for _, definition := range defaultDefinitions {
		ruleSet = append(ruleSet, ruleFromDefinition(definition))
	}
	return ruleSet
}

func Run(ctx context.Context, snapshot repo.Snapshot, ruleSet []Rule) Report {
	return RunWithConfig(ctx, snapshot, ruleSet, config.Config{})
}

func RunWithConfig(ctx context.Context, snapshot repo.Snapshot, ruleSet []Rule, cfg config.Config) Report {
	findings := make([]Finding, 0)
	for _, rule := range ruleSet {
		findings = append(findings, checkRule(ctx, rule, snapshot, cfg)...)
	}
	applyConfig(findings, cfg)
	return NewReport(findings)
}

func NewReport(findings []Finding) Report {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].RuleID == findings[j].RuleID {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].RuleID < findings[j].RuleID
	})
	return Report{OK: len(findings) == 0, Findings: findings}
}

type configuredRule interface {
	CheckWithConfig(context.Context, repo.Snapshot, config.Config) []Finding
}

func checkRule(ctx context.Context, rule Rule, snapshot repo.Snapshot, cfg config.Config) []Finding {
	if configured, ok := rule.(configuredRule); ok {
		return configured.CheckWithConfig(ctx, snapshot, cfg)
	}
	return rule.Check(ctx, snapshot)
}

func applyConfig(findings []Finding, cfg config.Config) {
	for i := range findings {
		findings[i].Severity = Severity(cfg.RuleSeverity(findings[i].RuleID, string(findings[i].Severity)))
	}
}

func Find(ruleSet []Rule, id string) (Metadata, bool) {
	for _, rule := range ruleSet {
		metadata := rule.Metadata()
		if metadata.ID == id {
			return metadata, true
		}
	}
	return Metadata{}, false
}

func Explain(ruleSet []Rule, id string) (string, bool) {
	metadata, ok := Find(ruleSet, id)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s\nseverity: %s\n\n%s\n", metadata.ID, metadata.Severity, metadata.Description), true
}

func (d Definition) Metadata() Metadata {
	return Metadata{
		ID:          d.ID,
		Severity:    d.Severity,
		Description: d.Description,
	}
}

func ruleFromDefinition(definition Definition) Rule {
	factory, ok := ruleFactories[definition.ID]
	if !ok {
		panic("missing rule implementation: " + definition.ID)
	}
	return factory(definition)
}

type ruleFactory func(Definition) Rule

var ruleFactories = map[string]ruleFactory{
	ReadmeRequiredRuleID:          newRequiredFileRule,
	AgentsRequiredRuleID:          newAgentRule,
	AgentsEmptyRuleID:             newAgentRule,
	AgentsCommandsRequiredRuleID:  newAgentRule,
	AgentsCommandInvalidRuleID:    newAgentRule,
	AgentsScopeRequiredRuleID:     newAgentRule,
	AgentsStaleRuleID:             newAgentRule,
	CIRequiredRuleID:              newCIRequiredRule,
	SlophammerCIRequiredRuleID:    newSlophammerCIRule,
	GoModuleRequiredRuleID:        newGoModuleRule,
	GoTestsRequiredRuleID:         newGoTestsRule,
	GoVetRequiredRuleID:           newGoVetRule,
	GoLintRequiredRuleID:          newGoLintRule,
	GoCoverageRequiredRuleID:      newGoCoverageRule,
	GoComplexityRequiredRuleID:    newGoComplexityRule,
	GoDryRequiredRuleID:           newGoDryRule,
	GoCRAPRequiredRuleID:          newGoCRAPRule,
	GoMutationRequiredRuleID:      newGoMutationRule,
	GoDependencyBoundariesRuleID:  newGoDependencyBoundariesRule,
	GoScopeIncompleteRuleID:       newGoScopeRule,
	GoSuppressionsJustifiedRuleID: newGoSuppressionsRule,
}

type agentRule struct {
	definition Definition
}

func newAgentRule(definition Definition) Rule {
	return agentRule{definition: definition}
}

func (r agentRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r agentRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	findings := []Finding{}
	for _, issue := range agents.Evaluate(snapshot) {
		if issue.RuleID != r.definition.ID {
			continue
		}
		findings = append(findings, Finding{
			RuleID:   issue.RuleID,
			Severity: r.definition.Severity,
			Path:     issue.Path,
			Message:  issue.Message,
		})
	}
	return findings
}

type requiredFileRule struct {
	definition Definition
}

func newRequiredFileRule(definition Definition) Rule {
	return requiredFileRule{definition: definition}
}

func (r requiredFileRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r requiredFileRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	if snapshot.HasFileFold(r.definition.Path) {
		return nil
	}
	return []Finding{{
		RuleID:   r.definition.ID,
		Severity: r.definition.Severity,
		Path:     r.definition.Path,
		Message:  r.definition.Message,
	}}
}

type ciRequiredRule struct {
	definition Definition
}

func newCIRequiredRule(definition Definition) Rule {
	return ciRequiredRule{definition: definition}
}

func (r ciRequiredRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r ciRequiredRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	if len(snapshot.WorkflowFiles()) > 0 {
		return nil
	}
	return []Finding{{
		RuleID:   r.definition.ID,
		Severity: r.definition.Severity,
		Path:     r.definition.Path,
		Message:  r.definition.Message,
	}}
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-08-18T21:29:28+08:00","module_hash":"604891d574a7b185c1754f4ee7104b004ac91dd39152221865902d10c2cebb1b","functions":[{"id":"func/DefaultDefinitions","name":"DefaultDefinitions","line":310,"end_line":312,"hash":"e754bc5cc75ece4e9ee72ba2514a3850f2a34f65c8f46c89a6113ea9a8cc9061"},{"id":"func/DefaultRules","name":"DefaultRules","line":314,"end_line":320,"hash":"63d5944fd40adfa0092f01f22bf02bd7973e4c8fe3c692aba3c2ce4a0a51cb08"},{"id":"func/Run","name":"Run","line":322,"end_line":324,"hash":"201bffacb17470826ccc6805092cb1791c00bbcec39b5a9dad66f67a799a7e9b"},{"id":"func/RunWithConfig","name":"RunWithConfig","line":326,"end_line":333,"hash":"74f3a0830706a82191fa0ad98ec7579cdc1155fe0d39271793a2c1e11d07c553"},{"id":"func/NewReport","name":"NewReport","line":335,"end_line":343,"hash":"872697fdac44c6ebbd7a7d4ac842da690ca8b11bcaf78339b344a17541a2704b"},{"id":"func/checkRule","name":"checkRule","line":349,"end_line":354,"hash":"63f11c7481eb21f4e073b100ea9190c6dad9cba05233ad97a3963a790bceaf98"},{"id":"func/applyConfig","name":"applyConfig","line":356,"end_line":360,"hash":"9524b6bbc378b746791141e457986a27d27288babe23ab5b201fd0493438eb91"},{"id":"func/Find","name":"Find","line":362,"end_line":370,"hash":"b4ee100a152f8682a6ad68054b169ea9cb3e63fba5d7683b98c3bb574d91e3c9"},{"id":"func/Explain","name":"Explain","line":372,"end_line":378,"hash":"b08b49c8195e21fbe9535c00ced70a48fd667e5ec9a4690b9370d3d15aee420f"},{"id":"func/Definition.Metadata","name":"Definition.Metadata","line":380,"end_line":386,"hash":"5a46fdca7cc1a5c538a7008bcc5b57cfad7bf057036770958b9c7abd0f2e490d"},{"id":"func/ruleFromDefinition","name":"ruleFromDefinition","line":388,"end_line":394,"hash":"6a0a50c6bc1ac4e4b94f1599429b95a37948f51a56abde20fd0c760929f5ff56"},{"id":"func/newAgentRule","name":"newAgentRule","line":426,"end_line":428,"hash":"6773aafdb6c09822a54ad577b16698732ef888228590f0c8a183eb0e0da6f782"},{"id":"func/agentRule.Metadata","name":"agentRule.Metadata","line":430,"end_line":432,"hash":"f4f0de7c5e520743d5b4d319bda2fa5fbba07210ef32f0671159bead4b0c11bd"},{"id":"func/agentRule.Check","name":"agentRule.Check","line":434,"end_line":448,"hash":"7830114cb221c1737412bd655c22dfc6a0a7f576b8dd33c5159dea1efafc51fd"},{"id":"func/newRequiredFileRule","name":"newRequiredFileRule","line":454,"end_line":456,"hash":"bc56b2daed3f130677cdeacaa881e95fc63375aecd2ae0035106095ea34b62af"},{"id":"func/requiredFileRule.Metadata","name":"requiredFileRule.Metadata","line":458,"end_line":460,"hash":"de2ee6cb14b23b8e103cdb17a9d44953c9c5230f0871db556cb06b747d0af8a8"},{"id":"func/requiredFileRule.Check","name":"requiredFileRule.Check","line":462,"end_line":472,"hash":"29c3dad0b76772c496243ac9410cfbd7947ca2d3ec64f95eedf9ce5259ad2336"},{"id":"func/newCIRequiredRule","name":"newCIRequiredRule","line":478,"end_line":480,"hash":"9dbcd214469ba3535220d4031848fcf8223ab05889b787c467d0d70dabd2ebee"},{"id":"func/ciRequiredRule.Metadata","name":"ciRequiredRule.Metadata","line":482,"end_line":484,"hash":"c8eaafa0693baea8f9f1b22fd8b3dcb209677c99a6b8df1d441d5018d255b685"},{"id":"func/ciRequiredRule.Check","name":"ciRequiredRule.Check","line":486,"end_line":496,"hash":"5a4dab722a5dac5102c0f9569b526aa0b80ec86cbb7727a327edf6eaecd62b50"}]}
// mutate4go-manifest-end
