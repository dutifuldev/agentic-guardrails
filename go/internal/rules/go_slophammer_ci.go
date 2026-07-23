package rules

import (
	"context"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

// slophammerInvocationWindow bounds how far after a checker binary name the
// check subcommand may appear and still count as one invocation.
const slophammerInvocationWindow = 160

// slophammerCIRule enforces that config without enforcement is decoration:
// when slophammer.yml is present, binding CI evidence must invoke a
// Slophammer checker.
type slophammerCIRule struct {
	definition Definition
}

func newSlophammerCIRule(definition Definition) Rule {
	return slophammerCIRule{definition: definition}
}

func (r slophammerCIRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r slophammerCIRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	if !hasModuleLocalSlophammerConfig(snapshot, "") {
		return nil
	}
	if slophammerInvocation(commandEvidenceText(snapshot)) {
		return nil
	}
	return []Finding{finding(r.definition)}
}

// commandEvidenceText joins the binding command evidence: workflow run
// scripts plus the scripts and runner files they reach.
func commandEvidenceText(snapshot repo.Snapshot) string {
	var evidence strings.Builder
	for _, file := range commandFiles(snapshot) {
		evidence.WriteString(file.Content)
		evidence.WriteString("\n")
	}
	return strings.ToLower(evidence.String())
}

func slophammerInvocation(evidence string) bool {
	if strings.Contains(evidence, "uses: osolmaz/slophammer@") {
		return true
	}
	for _, binary := range []string{"slophammer-go", "slophammer-ts", "slophammer-rs", "slophammer-py"} {
		if invocationWithCheck(evidence, binary) {
			return true
		}
	}
	return false
}

func invocationWithCheck(evidence string, binary string) bool {
	for index := strings.Index(evidence, binary); index >= 0; {
		end := min(index+slophammerInvocationWindow, len(evidence))
		if strings.Contains(evidence[index:end], " check") {
			return true
		}
		next := strings.Index(evidence[index+1:], binary)
		if next < 0 {
			return false
		}
		index += 1 + next
	}
	return false
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-07-23T04:07:44Z","module_hash":"4c25881922ec990d04ba3a275dd6bf3d2b44de1dc2101ef7398edea04e567174","functions":[{"id":"func/newSlophammerCIRule","name":"newSlophammerCIRule","line":21,"end_line":23,"hash":"9d62b88a7d012b91e9fa0108ddf5f14a268f21c3197a1baf08be516fb378d0cb"},{"id":"func/slophammerCIRule.Metadata","name":"slophammerCIRule.Metadata","line":25,"end_line":27,"hash":"7dfb8bdac3dd34da963912b121e08a32f365515262dbd33ed1423fa6593f54f7"},{"id":"func/slophammerCIRule.Check","name":"slophammerCIRule.Check","line":29,"end_line":37,"hash":"08bbdc558f45206a9cad4579410628143d622c868f4bc31076ad4915122cf1e8"},{"id":"func/commandEvidenceText","name":"commandEvidenceText","line":41,"end_line":48,"hash":"d418383842b5144fbef4343e2a79a23b18eebca89293f39077a6b2373bd174d6"},{"id":"func/slophammerInvocation","name":"slophammerInvocation","line":50,"end_line":60,"hash":"34879a3be9a0e139eb5eaffb44085ed6a91b4d4cef2839ba09729beb515dfd19"},{"id":"func/invocationWithCheck","name":"invocationWithCheck","line":62,"end_line":75,"hash":"074d8b74f88e15193f6d434e002bc1ff133a05460e475322c1731a50688a1b92"}]}
// mutate4go-manifest-end
