package rules

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

// nolintDirective is assembled from two literals so this production file does
// not itself contain the marker it scans for.
const nolintDirective = "//" + "nolint"

var generatedGoFilePattern = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

var suppressionExemptSegments = map[string]struct{}{
	"fixtures":  {},
	"templates": {},
	"testdata":  {},
	"vendor":    {},
	"scripts":   {},
	"generated": {},
}

// goSuppressionsRule flags bare nolint directives in production Go code:
// suppressions must carry a trailing explanation comment in the nolintlint
// style or sit under a preceding comment line.
type goSuppressionsRule struct {
	definition Definition
}

func newGoSuppressionsRule(definition Definition) Rule {
	return goSuppressionsRule{definition: definition}
}

func (r goSuppressionsRule) Metadata() Metadata {
	return r.definition.Metadata()
}

func (r goSuppressionsRule) Check(_ context.Context, snapshot repo.Snapshot) []Finding {
	findings := make([]Finding, 0)
	for _, file := range snapshot.FilesWithSuffix(".go") {
		if !productionGoSuppressionPath(file.Path) || generatedGoFile(file.Content) {
			continue
		}
		if line, ok := bareSuppressionLine(file.Content); ok {
			findings = append(findings, r.suppressionFinding(file.Path, line))
		}
	}
	return findings
}

func (r goSuppressionsRule) suppressionFinding(filePath string, line int) Finding {
	return Finding{
		RuleID:   r.definition.ID,
		Severity: r.definition.Severity,
		Path:     filePath,
		Message:  fmt.Sprintf("%s (line %d)", r.definition.Message, line),
	}
}

func productionGoSuppressionPath(filePath string) bool {
	return !strings.HasSuffix(filePath, "_test.go") &&
		!pathHasAnySegment(filePath, suppressionExemptSegments)
}

func generatedGoFile(content string) bool {
	return generatedGoFilePattern.MatchString(content)
}

// bareSuppressionLine returns the first line carrying a nolint directive
// with neither a trailing explanation nor a preceding comment line.
func bareSuppressionLine(content string) (int, bool) {
	previousLineIsComment := false
	for index, line := range strings.Split(content, "\n") {
		if bareNolintLine(line, previousLineIsComment) {
			return index + 1, true
		}
		previousLineIsComment = strings.HasPrefix(strings.TrimSpace(line), "//")
	}
	return 0, false
}

func bareNolintLine(line string, previousLineIsComment bool) bool {
	comment := lineCommentText(line)
	directive := strings.Index(comment, nolintDirective)
	if directive < 0 || previousLineIsComment {
		return false
	}
	return !nolintExplanation(comment[directive+len(nolintDirective):])
}

// lineCommentText returns a line's comment, starting at the first // that
// sits outside string and rune literals, so directives quoted in code are
// not findings. Lines inside multi-line raw strings are beyond a line-based
// scan and stay visible.
func lineCommentText(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		character := line[i]
		if quote != 0 {
			quote, i = insideGoQuote(quote, character, i)
			continue
		}
		if goQuoteCharacter(character) {
			quote = character
			continue
		}
		if strings.HasPrefix(line[i:], "//") {
			return line[i:]
		}
	}
	return ""
}

func insideGoQuote(quote byte, character byte, index int) (byte, int) {
	if character == '\\' && quote != '`' {
		return quote, index + 1
	}
	if character == quote {
		return 0, index
	}
	return quote, index
}

func goQuoteCharacter(character byte) bool {
	return character == '"' || character == '\'' || character == '`'
}

// nolintExplanation reports whether the text after a nolint directive carries
// a non-empty trailing comment, the nolintlint reason convention.
func nolintExplanation(rest string) bool {
	comment := strings.Index(rest, "//")
	if comment < 0 {
		return false
	}
	return strings.TrimSpace(rest[comment+2:]) != ""
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-06-12T22:50:27+08:00","module_hash":"998f94358279a13545dc4e18efad912ade518e4e84ab4b095bf859d7e0e199ba","functions":[{"id":"func/newGoSuppressionsRule","name":"newGoSuppressionsRule","line":34,"end_line":36,"hash":"6db7e7afa6cc78685c123fda60db9a631f5fbf07aa933050a62ac3f5cc7f9b64"},{"id":"func/goSuppressionsRule.Metadata","name":"goSuppressionsRule.Metadata","line":38,"end_line":40,"hash":"0c7f0aaec0a967342a9b46e6fbcb12559ac089f82ce88b613c52f040f73cb818"},{"id":"func/goSuppressionsRule.Check","name":"goSuppressionsRule.Check","line":42,"end_line":53,"hash":"b4ea913faa398d0f8a829b75340f0183506ca7ead9eb91d396389a5889b0f1d4"},{"id":"func/goSuppressionsRule.suppressionFinding","name":"goSuppressionsRule.suppressionFinding","line":55,"end_line":62,"hash":"cf791b16dbb2c3725d6892c21dd6627b3a996a616e8c0a4ba614fa5c7647fcbe"},{"id":"func/productionGoSuppressionPath","name":"productionGoSuppressionPath","line":64,"end_line":67,"hash":"c698aefbae593a9f362e50cc7377e1132eeb9ad8170f858e5258261e6d9fe29c"},{"id":"func/generatedGoFile","name":"generatedGoFile","line":69,"end_line":71,"hash":"ff397494c250bb43ec7f44b8332a916c35a9b98d9b76e51b9a8f0fe48a1161af"},{"id":"func/bareSuppressionLine","name":"bareSuppressionLine","line":75,"end_line":84,"hash":"1e38ef4f31961d27a633e2a11bb763767698c847db4f77983bb2ecd3032c7fd6"},{"id":"func/bareNolintLine","name":"bareNolintLine","line":86,"end_line":93,"hash":"488f11f4d4676b416293e658dcf11fd077c41a26c84db9943c020ac72d142d7f"},{"id":"func/lineCommentText","name":"lineCommentText","line":99,"end_line":116,"hash":"b6833aac1e1249523d28750bfd1ad8ff27fb3be56664a803227f893dc7a69f50"},{"id":"func/insideGoQuote","name":"insideGoQuote","line":118,"end_line":126,"hash":"2f54e8266443376ffb312fcea24e9ef6127502e4b5318da001395e7d5dd85826"},{"id":"func/goQuoteCharacter","name":"goQuoteCharacter","line":128,"end_line":130,"hash":"0a7ff96a0bfefcd3feb26aef401218996774b35e9d226a27d158a1e52f1a497c"},{"id":"func/nolintExplanation","name":"nolintExplanation","line":134,"end_line":140,"hash":"76b77e40d18f887305d4c8f9aa57b605d137df9961b6eb13673acc6a8b2a5c38"}]}
// mutate4go-manifest-end
