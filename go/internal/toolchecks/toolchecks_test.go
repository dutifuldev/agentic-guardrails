package toolchecks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/gotools"
)

func TestCheckDryRunsNativeEngineAndEnforcesCandidateBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "left.go", duplicateSource("Left"))
	writeFile(t, root, "right.go", duplicateSource("Right"))
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{Root: root, MaximumCandidates: 0, MaximumSet: true}, &out, &errOut, &fakeRunner{})

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "DRY candidates:") || !strings.Contains(out.String(), "maximum: 0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckDryHonorsExplicitZeroBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "left.go", duplicateSource("Left"))
	writeFile(t, root, "right.go", duplicateSource("Right"))
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{Root: root, MaximumCandidates: 0, MaximumSet: true}, &out, &errOut, &fakeRunner{})

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "maximum: 0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckDryPassesConfiguredPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "cmd/main.go", duplicateSource("Left"))
	writeFile(t, root, "internal/app/app.go", duplicateSource("Right"))
	writeFile(t, root, "ignored/ignored.go", duplicateSource("Ignored"))
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{
		Root:              root,
		MaximumCandidates: 0,
		MaximumSet:        true,
		Paths:             []string{"cmd/main.go", "internal/app/app.go"},
	}, &out, &errOut, &fakeRunner{})

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "maximum: 0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckDryHonorsExplicitDisabledEngines(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "left.go", duplicateSource("Left"))
	writeFile(t, root, "right.go", duplicateSource("Right"))
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{
		Root:               root,
		MaximumCandidates:  0,
		MaximumSet:         true,
		StructuralEnabled:  false,
		StructuralSet:      true,
		CopiedBlockEnabled: false,
		CopiedBlockSet:     true,
	}, &out, &errOut, &fakeRunner{})

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "DRY candidates: 0; maximum: 0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckDryCanRenderJSONReport(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "left.go", duplicateSource("Left"))
	writeFile(t, root, "right.go", duplicateSource("Right"))
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{Root: root, MaximumCandidates: 999, MaximumSet: true, Format: "json"}, &out, &errOut, &fakeRunner{})

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"findings"`) || !strings.Contains(out.String(), `"structural-function"`) {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckDryCanRenderTextReport(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "left.go", duplicateSource("Left"))
	writeFile(t, root, "right.go", duplicateSource("Right"))
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{Root: root, MaximumCandidates: 999, MaximumSet: true, Format: "text"}, &out, &errOut, &fakeRunner{})

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Structural function findings:") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckDryReportsScanErrors(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckDry(context.Background(), DryOptions{Root: filepath.Join(t.TempDir(), "missing")}, &out, &errOut, &fakeRunner{})

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "dry check failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCountDRYCandidatesAcceptsNativeAndLegacyReports(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report string
		want   int
	}{
		{name: "native", report: `{"findings":[{},{}]}`, want: 2},
		{name: "legacy", report: `{"candidates":[{}]}`, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CountDRYCandidates([]byte(tc.report))
			if err != nil {
				t.Fatalf("CountDRYCandidates returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("CountDRYCandidates = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCountDRYCandidatesRejectsUnknownShape(t *testing.T) {
	if _, err := CountDRYCandidates([]byte(`{"groups":[]}`)); err == nil {
		t.Fatal("CountDRYCandidates returned nil error")
	}
}

func TestCheckCRAPRunsCRAP4GoAndReportsViolations(t *testing.T) {
	runner := &fakeRunner{output: []byte("pkg.Func 1 2 3 30.1\npkg.OK 1 2 3 30.0\n")}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:         "/repo",
		MaximumScore: 30,
		MaximumSet:   true,
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	wantArgs := gotools.CRAP4Go.GoRunArgs(gotools.Latest)
	if runnerCall := runner.call; runnerCall.dir != "/repo" || runnerCall.name != "go" || !reflect.DeepEqual(runnerCall.args, wantArgs) {
		t.Fatalf("call = %#v, want dir=/repo name=go args=%#v", runnerCall, wantArgs)
	}
	if !strings.Contains(errOut.String(), "CRAP score 30.1 exceeds maximum 30.0 for pkg.Func") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithTargetsUsesCrossPackageCoverage(t *testing.T) {
	runner := &scriptedRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:         "/repo",
		MaximumScore: 8,
		MaximumSet:   true,
		Targets:      []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if len(runner.calls) != 7 {
		t.Fatalf("calls = %#v, want 7 calls", runner.calls)
	}
	if got := strings.Join(runner.calls[1].args, " "); got != "list ./internal/service" {
		t.Fatalf("go list target packages args = %q", got)
	}
	if got := strings.Join(runner.calls[3].args, " "); !strings.Contains(got, "gocyclo") || !strings.Contains(got, "internal/service/service.go") {
		t.Fatalf("gocyclo args = %q", got)
	}
	if !strings.Contains(errOut.String(), "CRAP score 16.0 exceeds maximum 8.0 for service.Run") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithTargetsUsesSuppliedCoverageProfile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 1 1\n")
	runner := &scriptedRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    8,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr = %q", code, errOut.String())
	}
	if len(runner.calls) != 4 {
		t.Fatalf("calls = %#v, want 4 calls", runner.calls)
	}
	for _, call := range runner.calls {
		got := strings.Join(call.args, " ")
		if strings.HasPrefix(got, "test ") || strings.HasPrefix(got, "list ./") {
			t.Fatalf("unexpected coverage generation/list call: %#v", call)
		}
	}
	if got := strings.Join(runner.calls[2].args, " "); !strings.HasPrefix(got, "tool cover -func=") || !filepath.IsAbs(strings.TrimPrefix(got, "tool cover -func=")) {
		t.Fatalf("cover args = %q, want absolute supplied profile", got)
	}
}

func TestCheckCRAPWithSuppliedCoverageProfileRejectsMissingAnalyzedFunctionCoverage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 1 1\n"+
		"example.test/backend/internal/other/other.go:8.1,8.2 1 1\n")
	runner := &scriptedRunner{
		coverOutput:         "example.test/backend/internal/service/service.go:12:\tRun\t90.0%\ntotal:\t(statements)\t90.0%\n",
		complexityOutput:    "8 service Run internal/service/service.go:12:1\n9 other Risk internal/other/other.go:7:1\n",
		complexityOutputSet: true,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    8,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go", "internal/other/other.go"},
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "does not include coverage for analyzed function other.Risk") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithSuppliedCoverageProfileUsesRawCoverageForFunctionLiterals(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nvar closeDB = func(db DB) error { return db.Close() }\n\ntype DB interface { Close() error }\n")
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:3.33,3.54 1 1\n")
	runner := &scriptedRunner{
		coverOutput:         "total:\t(statements)\t0.0%\n",
		complexityOutput:    "1 service closeDB internal/service/service.go:3:15\n",
		complexityOutputSet: true,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    1,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "CRAP scores meet maximum 1.0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckCRAPWithSuppliedCoverageProfileUsesFullRawCoverageForMultilineFunctionLiterals(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nvar score = func(ok bool) int {\n\tif ok {\n\t\treturn 1\n\t}\n\treturn 0\n}\n")
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:3.32,4.8 1 1\n"+
		"example.test/backend/internal/service/service.go:4.8,6.3 1 1\n"+
		"example.test/backend/internal/service/service.go:7.2,7.10 1 0\n")
	runner := &scriptedRunner{
		coverOutput:         "total:\t(statements)\t0.0%\n",
		complexityOutput:    "2 service score internal/service/service.go:3:13\n",
		complexityOutputSet: true,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    2,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stdout = %q stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "CRAP score 2.1 exceeds maximum 2.0 for service.score") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithSuppliedCoverageProfileKeepsSameLineFunctionLiteralsSeparate(t *testing.T) {
	root := t.TempDir()
	sourceLine := "var first = func(ok bool) int { if ok { return 1 }; return 0 }; var second = func(ok bool) int { if ok { return 2 }; return 0 }"
	firstColumn := oneBasedColumn(t, sourceLine, "func(ok bool) int { if ok { return 1 }")
	secondColumn := oneBasedColumn(t, sourceLine, "func(ok bool) int { if ok { return 2 }")
	writeFile(t, root, "internal/service/service.go", "package service\n\n"+sourceLine+"\n")
	writeFile(t, root, "coverage.out", fmt.Sprintf("mode: count\n"+
		"example.test/backend/internal/service/service.go:3.%d,3.%d 10 0\n"+
		"example.test/backend/internal/service/service.go:3.%d,3.%d 10 1\n",
		firstColumn,
		oneBasedColumn(t, sourceLine, "}; var second")+1,
		secondColumn,
		len(sourceLine)+1))
	runner := &scriptedRunner{
		coverOutput:         "total:\t(statements)\t0.0%\n",
		complexityOutput:    fmt.Sprintf("3 service first internal/service/service.go:3:%d\n3 service second internal/service/service.go:3:%d\n", firstColumn, secondColumn),
		complexityOutputSet: true,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    5,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stdout = %q stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "CRAP score 12.0 exceeds maximum 5.0 for service.first") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithSuppliedCoverageProfileAcceptsZeroStatementFunctionLiteral(t *testing.T) {
	root := t.TempDir()
	sourceLine := "var hook = func() {}"
	hookColumn := oneBasedColumn(t, sourceLine, "func()")
	writeFile(t, root, "internal/service/service.go", "package service\n\n"+sourceLine+"\n")
	writeFile(t, root, "coverage.out", fmt.Sprintf("mode: count\n"+
		"example.test/backend/internal/service/service.go:3.%d,3.%d 0 1\n",
		hookColumn,
		len(sourceLine)+1))
	runner := &scriptedRunner{
		coverOutput:         "total:\t(statements)\t0.0%\n",
		complexityOutput:    fmt.Sprintf("1 service hook internal/service/service.go:3:%d\n", hookColumn),
		complexityOutputSet: true,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    2,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stdout = %q stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "CRAP scores meet maximum 2.0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckCRAPWithGeneratedCoverageUsesFullRawCoverageForMultilineFunctionLiterals(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nvar score = func(ok bool) int {\n\tif ok {\n\t\treturn 1\n\t}\n\treturn 0\n}\n")
	runner := &generatedLiteralCoverageRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:         root,
		MaximumScore: 2,
		MaximumSet:   true,
		Targets:      []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stdout = %q stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "CRAP score 2.1 exceeds maximum 2.0 for service.score") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithGeneratedCoverageRejectsUnmappedNonLiteralFunction(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nfunc Run() { if true { return } }\n")
	runner := &generatedUnmappedFunctionCoverageRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:         root,
		MaximumScore: 8,
		MaximumSet:   true,
		Targets:      []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2; stdout = %q stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "does not include coverage for analyzed function service.Run") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithGeneratedCoverageIgnoresBuildConstrainedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nfunc Run() int { return 1 }\n")
	writeFile(t, root, "internal/service/windows.go", "//go:build windows\n\npackage service\n\nfunc WindowsOnly() int { return 1 }\n")
	runner := &generatedBuildConstrainedCoverageRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:         root,
		MaximumScore: 8,
		MaximumSet:   true,
		Targets:      []string{"internal/service/service.go", "internal/service/windows.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stdout = %q stderr = %q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "CRAP scores meet maximum 8.0") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestParseCoveragePositionRejectsInvalidPositions(t *testing.T) {
	for _, position := range []string{
		"",
		"12",
		".4",
		"12.",
		"nope.4",
		"0.4",
		"12.nope",
		"12.0",
	} {
		t.Run(position, func(t *testing.T) {
			if _, ok := parseCoveragePosition(position); ok {
				t.Fatalf("parseCoveragePosition(%q) succeeded", position)
			}
		})
	}
}

func TestParseCoveragePositionAcceptsLineAndColumn(t *testing.T) {
	got, ok := parseCoveragePosition("12.4")
	if !ok {
		t.Fatal("parseCoveragePosition returned false")
	}
	want := sourcePosition{line: 12, column: 4}
	if got != want {
		t.Fatalf("parseCoveragePosition = %#v, want %#v", got, want)
	}
}

func TestCheckCRAPWithoutTargetsUsesSuppliedCoverageProfile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 1 1\n")
	runner := &scriptedRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    8,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr = %q", code, errOut.String())
	}
	for _, call := range runner.calls {
		got := strings.Join(call.args, " ")
		if strings.Contains(got, "crap4go") || strings.HasPrefix(got, "test ") {
			t.Fatalf("unexpected CRAP fallback or coverage generation call: %#v", call)
		}
	}
	if got := strings.Join(runner.calls[1].args, " "); !strings.Contains(got, "gocyclo") || !strings.Contains(got, " .") {
		t.Fatalf("gocyclo args = %q", got)
	}
}

func TestCheckCRAPWithSuppliedCoverageProfileRejectsUnmappedNoFunctionTargets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n")
	runner := &scriptedRunner{coverOutput: "total:\t(statements)\t0.0%\n", complexityOutputSet: true}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    8,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/constants.go"},
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "does not include files for module") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCRAPWithNoFunctionsRejectsUnrelatedSuppliedCoverageProfile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/other/other.go:7.1,7.2 1 1\n")
	runner := &scriptedRunner{
		coverOutput:         "example.test/backend/internal/other/other.go:7:\tOther\t90.0%\ntotal:\t(statements)\t90.0%\n",
		complexityOutputSet: true,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{
		Root:            root,
		MaximumScore:    8,
		MaximumSet:      true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/constants.go"},
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "does not include configured Go scope file internal/service/constants.go") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoverageEnforcesConfiguredThreshold(t *testing.T) {
	runner := &scriptedRunner{coverageTotal: "84.9%"}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:         "/repo",
		Threshold:    85,
		ThresholdSet: true,
		Targets:      []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("calls = %#v, want 5 calls", runner.calls)
	}
	if got := strings.Join(runner.calls[1].args, " "); got != "list ./internal/service" {
		t.Fatalf("go list target packages args = %q", got)
	}
	if !strings.Contains(errOut.String(), "coverage 84.9% is below required 85.0%") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoverageUsesSuppliedCoverageProfile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 17 1\n"+
		"example.test/backend/internal/service/service.go:13.1,13.2 3 0\n")
	runner := &scriptedRunner{coverageTotal: "85.0%"}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls = %#v, want 3 calls", runner.calls)
	}
	for _, call := range runner.calls {
		got := strings.Join(call.args, " ")
		if strings.HasPrefix(got, "test ") || strings.HasPrefix(got, "list ./") {
			t.Fatalf("unexpected coverage generation/list call: %#v", call)
		}
	}
	if !strings.Contains(out.String(), "coverage 85.0% meets required 85.0%") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckCoverageAcceptsSuppliedCoverageProfileWithExtraPackages(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 17 1\n"+
		"example.test/backend/internal/service/service.go:13.1,13.2 3 0\n"+
		"example.test/backend/internal/other/other.go:7.1,7.2 10 1\n")
	runner := &scriptedRunner{
		coverageTotal: "85.0%",
		coverOutput: "example.test/backend/internal/service/service.go:12:\tRun\t85.0%\n" +
			"example.test/backend/internal/other/other.go:7:\tOther\t90.0%\n" +
			"total:\t(statements)\t85.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
}

func TestCheckCoverageWithSuppliedProfileUsesScopedTotal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 10 0\n"+
		"example.test/backend/internal/other/other.go:7.1,7.2 90 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:12:\tRun\t0.0%\n" +
			"example.test/backend/internal/other/other.go:7:\tOther\t100.0%\n" +
			"total:\t(statements)\t90.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "coverage 0.0% is below required 85.0%") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoverageWithUnscopedSuppliedProfileUsesCurrentPackageTotal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nfunc Run() int { return 1 }\n")
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:3.16,3.27 10 0\n"+
		"example.test/backend/internal/stale/stale.go:7.1,7.2 90 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:3:\tRun\t0.0%\n" +
			"example.test/backend/internal/stale/stale.go:7:\tStale\t100.0%\n" +
			"total:\t(statements)\t90.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
	}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "coverage 0.0% is below required 85.0%") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoverageWithSuppliedProfileMergesDuplicateBlocks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 1 1\n"+
		"example.test/backend/internal/service/service.go:12.2,12.3 1 0\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 1 0\n"+
		"example.test/backend/internal/service/service.go:12.2,12.3 1 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:12:\tRun\t100.0%\n" +
			"total:\t(statements)\t100.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       100,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "coverage 100.0% meets required 100.0%") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckCoverageWithSuppliedProfileAcceptsTotalOnlyCoverOutput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/init.go:2.20,2.32 1 1\n")
	runner := &scriptedRunner{coverOutput: "total:\t(statements)\t100.0%\n"}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       100,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/init.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "coverage 100.0% meets required 100.0%") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckCoverageRejectsSuppliedCoverageProfileMissingScopedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:12.1,12.2 1 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:12:\tRun\t100.0%\n" +
			"total:\t(statements)\t100.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go", "internal/other/other.go"},
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "does not include configured Go scope file internal/other/other.go") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoverageRejectsUnscopedSuppliedCoverageProfileMissingPackageFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nfunc Run() int { return 1 }\n")
	writeFile(t, root, "internal/other/other.go", "package other\n\nfunc Other() int { return 1 }\n")
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:3.16,3.27 1 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:3:\tRun\t100.0%\n" +
			"total:\t(statements)\t100.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "does not include configured Go scope file internal/other/other.go") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoverageAllowsMissingDeclarationOnlyScopedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nfunc Run() int { return 1 }\n")
	writeFile(t, root, "internal/service/types.go", "package service\n\ntype Config struct{ Enabled bool }\nconst Name = \"service\"\n")
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:3.16,3.27 1 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:3:\tRun\t100.0%\n" +
			"total:\t(statements)\t100.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       100,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go", "internal/service/types.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
}

func TestCheckCoverageAllowsMissingTestScopedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/service/service.go", "package service\n\nfunc Run() int { return 1 }\n")
	writeFile(t, root, "internal/service/service_test.go", "package service\n\nfunc TestRun() { _ = Run() }\n")
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/service/service.go:3.16,3.27 1 1\n")
	runner := &scriptedRunner{
		coverOutput: "example.test/backend/internal/service/service.go:3:\tRun\t100.0%\n" +
			"total:\t(statements)\t100.0%\n",
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       100,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go", "internal/service/service_test.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
}

func TestCheckCoverageRejectsSuppliedCoverageProfileOutsideScope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "coverage.out", "mode: count\n"+
		"example.test/backend/internal/other/other.go:7.1,7.2 10 1\n")
	runner := &scriptedRunner{coverOutput: "example.test/backend/internal/other/other.go:7:\tOther\t90.0%\ntotal:\t(statements)\t90.0%\n"}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:            root,
		Threshold:       85,
		ThresholdSet:    true,
		CoverageProfile: "coverage.out",
		Targets:         []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "does not include configured Go scope file internal/service/service.go") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckCoveragePassesWhenTotalMeetsThreshold(t *testing.T) {
	runner := &scriptedRunner{coverageTotal: "85.0%"}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCoverage(context.Background(), CoverageOptions{
		Root:         "/repo",
		Threshold:    85,
		ThresholdSet: true,
		Targets:      []string{"internal/service/service.go"},
	}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "coverage 85.0% meets required 85.0%") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestCheckCRAPHonorsExplicitZeroLimit(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckCRAP(context.Background(), CRAPOptions{MaximumScore: 0, MaximumSet: true}, &out, &errOut, &fakeRunner{
		output: []byte("pkg.Func 1 2 3 0.1\n"),
	})

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "CRAP score 0.1 exceeds maximum 0.0 for pkg.Func") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestCheckMutationRunsMutate4GoScan(t *testing.T) {
	runner := &fakeRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: "/repo", Target: "main.go", Scan: true}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	wantArgs := gotools.Mutate4Go.GoRunArgs(gotools.Latest, "main.go", "--scan")
	if runnerCall := runner.call; runnerCall.dir != "/repo" || runnerCall.name != "go" || !reflect.DeepEqual(runnerCall.args, wantArgs) {
		t.Fatalf("call = %#v, want dir=/repo name=go args=%#v", runnerCall, wantArgs)
	}
}

func TestCheckMutationFailsOnSurvivors(t *testing.T) {
	runner := &fakeRunner{output: []byte("Mutation Report\n===============\nKilled: 4\nSurvived: 3\nUncovered: 0\n")}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: "/repo", Target: "main.go"}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("3 mutant(s) survived")) {
		t.Fatalf("errOut = %q, want survivor message", errOut.String())
	}
}

func TestCheckMutationFailsOnUncoveredChangedSites(t *testing.T) {
	runner := &fakeRunner{output: []byte(
		"Changed mutation sites: 3\nSelected mutation sites: 0\n" +
			"Mutation Report\nKilled: 0\nSurvived: 0\nUncovered: 3\n")}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: "/repo", Target: "main.go"}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("no test coverage")) {
		t.Fatalf("errOut = %q, want uncovered-changed message", errOut.String())
	}
}

func TestCheckMutationScanModeIgnoresCounts(t *testing.T) {
	runner := &fakeRunner{output: []byte("Changed mutation sites: 9\nSelected mutation sites: 0\n")}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: "/repo", Target: "main.go", Scan: true}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0 for scan mode", code)
	}
}

func TestCheckMutationPassesWithoutSurvivors(t *testing.T) {
	runner := &fakeRunner{output: []byte("Killed: 5\nSurvived: 0\n")}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: "/repo", Target: "main.go"}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0; errOut=%s", code, errOut.String())
	}
}

func TestCheckMutationRunsConfiguredTargets(t *testing.T) {
	runner := &fakeRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: "/repo", Targets: []string{"a.go", "b.go"}, Scan: true}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %#v, want 2 calls", runner.calls)
	}
	wantFirst := gotools.Mutate4Go.GoRunArgs(gotools.Latest, "a.go", "--scan")
	wantSecond := gotools.Mutate4Go.GoRunArgs(gotools.Latest, "b.go", "--scan")
	if !reflect.DeepEqual(runner.calls[0].args, wantFirst) || !reflect.DeepEqual(runner.calls[1].args, wantSecond) {
		t.Fatalf("calls = %#v, want args %#v and %#v", runner.calls, wantFirst, wantSecond)
	}
}

func TestCheckMutationRequiresTarget(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{}, &out, &errOut, &fakeRunner{})

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "--target is required") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestToolFailureReturnsInfrastructureError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Target: "main.go"}, &out, &errOut, &fakeRunner{err: errors.New("boom")})

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "mutate4go failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

// stampingRunner rewrites the target file before answering, the way
// mutate4go rewrites in-source manifests during every run.
type stampingRunner struct {
	path    string
	stamped string
	output  []byte
	err     error
}

func (runner *stampingRunner) Run(_ context.Context, _ string, _ string, _ ...string) (CommandResult, error) {
	if err := os.WriteFile(runner.path, []byte(runner.stamped), 0o600); err != nil {
		panic(err)
	}
	return CommandResult{Stdout: runner.output}, runner.err
}

func stampedMutationRun(t *testing.T, output string, runErr error) (string, int, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "main.go", "package main // original manifest\n")
	runner := &stampingRunner{
		path:    filepath.Join(root, "main.go"),
		stamped: "package main // restamped manifest\n",
		output:  []byte(output),
		err:     runErr,
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := CheckMutation(context.Background(), MutationOptions{Root: root, Target: "main.go"}, &out, &errOut, runner)
	content, err := os.ReadFile(runner.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content), code, errOut.String()
}

func TestCheckMutationFailingRunRestoresManifests(t *testing.T) {
	content, code, stderr := stampedMutationRun(t,
		"Changed mutation sites: 2\nSelected mutation sites: 2\nKilled: 1\nSurvived: 1\n", nil)

	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(content, "original manifest") {
		t.Fatalf("content = %q, want survivors' manifest stamp reverted", content)
	}
}

func TestCheckMutationErroringRunRestoresManifests(t *testing.T) {
	content, code, _ := stampedMutationRun(t, "", errors.New("boom"))

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(content, "original manifest") {
		t.Fatalf("content = %q, want manifest stamp reverted", content)
	}
}

func TestCheckMutationPassingRunKeepsEarnedManifests(t *testing.T) {
	content, code, stderr := stampedMutationRun(t,
		"Changed mutation sites: 2\nSelected mutation sites: 2\nKilled: 2\nSurvived: 0\n", nil)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(content, "restamped manifest") {
		t.Fatalf("content = %q, want earned manifest stamp kept", content)
	}
}

func TestCheckMutationNoChangeRunDoesNotChurnManifests(t *testing.T) {
	content, code, stderr := stampedMutationRun(t,
		"Changed mutation sites: 0\nSelected mutation sites: 0\nKilled: 0\nSurvived: 0\n", nil)

	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(content, "original manifest") {
		t.Fatalf("content = %q, want tested_at churn reverted", content)
	}
}

func TestCheckMutationUncoveredChangeRunRestoresManifests(t *testing.T) {
	content, code, _ := stampedMutationRun(t,
		"Changed mutation sites: 3\nSelected mutation sites: 1\nKilled: 1\nSurvived: 0\n", nil)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(content, "original manifest") {
		t.Fatalf("content = %q, want manifest stamp reverted", content)
	}
}

func TestCheckMutationScanNeverMovesManifests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.go", "package main // original manifest\n")
	runner := &stampingRunner{
		path:    filepath.Join(root, "main.go"),
		stamped: "package main // restamped manifest\n",
		output:  []byte("Changed mutation sites: 5\nSelected mutation sites: 5\n"),
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: root, Target: "main.go", Scan: true}, &out, &errOut, runner)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	content, err := os.ReadFile(runner.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "original manifest") {
		t.Fatalf("content = %q, want scan to leave manifests alone", content)
	}
}

func TestCheckMutationSnapshotsDirectoryTargets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/main.go", "package pkg // original manifest\n")
	runner := &stampingRunner{
		path:    filepath.Join(root, "pkg", "main.go"),
		stamped: "package pkg // restamped manifest\n",
		output:  []byte("Killed: 0\nSurvived: 1\n"),
	}
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := CheckMutation(context.Background(), MutationOptions{Root: root, Target: "pkg"}, &out, &errOut, runner)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	content, err := os.ReadFile(runner.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "original manifest") {
		t.Fatalf("content = %q, want directory snapshot restored", content)
	}
}

type fakeRunner struct {
	call   fakeCall
	calls  []fakeCall
	output []byte
	stderr []byte
	err    error
}

func (runner *fakeRunner) Run(_ context.Context, dir string, name string, args ...string) (CommandResult, error) {
	runner.call = fakeCall{dir: dir, name: name, args: args}
	runner.calls = append(runner.calls, runner.call)
	return CommandResult{Stdout: runner.output, Stderr: runner.stderr}, runner.err
}

type generatedLiteralCoverageRunner struct{}

func (runner *generatedLiteralCoverageRunner) Run(_ context.Context, dir string, _ string, args ...string) (CommandResult, error) {
	command := strings.Join(args, " ")
	if result, ok := generatedCoverageListResult(command, dir); ok {
		return result, nil
	}
	switch {
	case strings.HasPrefix(command, "test "):
		profilePath := coverProfilePath(args)
		if profilePath == "" {
			return CommandResult{}, errors.New("missing coverprofile")
		}
		content := "mode: count\n" +
			"example.test/backend/internal/service/service.go:3.32,4.8 1 1\n" +
			"example.test/backend/internal/service/service.go:4.8,6.3 1 1\n" +
			"example.test/backend/internal/service/service.go:7.2,7.10 1 0\n"
		if err := os.WriteFile(profilePath, []byte(content), 0o600); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{}, nil
	case strings.Contains(command, "gocyclo"):
		return CommandResult{Stdout: []byte("2 service score internal/service/service.go:3:13\n")}, nil
	case strings.HasPrefix(command, "tool cover -func="):
		return CommandResult{Stdout: []byte("total:\t(statements)\t0.0%\n")}, nil
	default:
		return CommandResult{}, errors.New("unexpected command: " + command)
	}
}

type generatedUnmappedFunctionCoverageRunner struct{}

func (runner *generatedUnmappedFunctionCoverageRunner) Run(_ context.Context, dir string, _ string, args ...string) (CommandResult, error) {
	command := strings.Join(args, " ")
	if result, ok := generatedCoverageListResult(command, dir); ok {
		return result, nil
	}
	switch {
	case strings.HasPrefix(command, "test "):
		profilePath := coverProfilePath(args)
		if profilePath == "" {
			return CommandResult{}, errors.New("missing coverprofile")
		}
		if err := os.WriteFile(profilePath, []byte("mode: count\n"), 0o600); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{}, nil
	case strings.Contains(command, "gocyclo"):
		return CommandResult{Stdout: []byte("2 service Run internal/service/service.go:3:1\n")}, nil
	case strings.HasPrefix(command, "tool cover -func="):
		return CommandResult{Stdout: []byte("total:\t(statements)\t0.0%\n")}, nil
	default:
		return CommandResult{}, errors.New("unexpected command: " + command)
	}
}

type generatedBuildConstrainedCoverageRunner struct{}

func (runner *generatedBuildConstrainedCoverageRunner) Run(_ context.Context, dir string, _ string, args ...string) (CommandResult, error) {
	command := strings.Join(args, " ")
	if result, ok := generatedCoverageListResult(command, dir); ok {
		return result, nil
	}
	switch {
	case strings.HasPrefix(command, "test "):
		profilePath := coverProfilePath(args)
		if profilePath == "" {
			return CommandResult{}, errors.New("missing coverprofile")
		}
		content := "mode: count\n" +
			"example.test/backend/internal/service/service.go:3.16,3.30 1 1\n"
		if err := os.WriteFile(profilePath, []byte(content), 0o600); err != nil {
			return CommandResult{}, err
		}
		return CommandResult{}, nil
	case strings.Contains(command, "gocyclo"):
		return CommandResult{Stdout: []byte("1 service Run internal/service/service.go:3:1\n1 service WindowsOnly internal/service/windows.go:5:1\n")}, nil
	case strings.HasPrefix(command, "tool cover -func="):
		return CommandResult{Stdout: []byte("example.test/backend/internal/service/service.go:3:\tRun\t100.0%\ntotal:\t(statements)\t100.0%\n")}, nil
	default:
		return CommandResult{}, errors.New("unexpected command: " + command)
	}
}

func generatedCoverageListResult(command string, dir string) (CommandResult, bool) {
	switch {
	case command == "list -m":
		return CommandResult{Stdout: []byte("example.test/backend\n")}, true
	case command == "list ./internal/service":
		return CommandResult{Stdout: []byte("example.test/backend/internal/service\n")}, true
	case command == "list ./...":
		return CommandResult{Stdout: []byte("example.test/backend/internal/service\n")}, true
	case strings.HasPrefix(command, "list -f "):
		return CommandResult{Stdout: []byte(fmt.Sprintf("%s|service.go\n", filepath.Join(dir, "internal/service")))}, true
	default:
		return CommandResult{}, false
	}
}

func coverProfilePath(args []string) string {
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, "-coverprofile="); ok {
			return value
		}
	}
	return ""
}

type scriptedRunner struct {
	calls               []fakeCall
	coverageTotal       string
	coverOutput         string
	complexityOutput    string
	complexityOutputSet bool
}

func (runner *scriptedRunner) Run(_ context.Context, dir string, name string, args ...string) (CommandResult, error) {
	runner.calls = append(runner.calls, fakeCall{dir: dir, name: name, args: append([]string(nil), args...)})
	command := strings.Join(args, " ")
	if result, ok := runner.listResult(dir, command); ok {
		return result, nil
	}
	if strings.HasPrefix(command, "test ") {
		return CommandResult{}, nil
	}
	if strings.Contains(command, "gocyclo") {
		return runner.complexityResult(), nil
	}
	if strings.HasPrefix(command, "tool cover -func=") {
		return runner.coverResult(), nil
	}
	return CommandResult{}, errors.New("unexpected command: " + command)
}

func (runner *scriptedRunner) listResult(dir string, command string) (CommandResult, bool) {
	if strings.HasPrefix(command, "list -f ") {
		return CommandResult{Stdout: []byte(runner.goListFilesOutput(dir, command))}, true
	}
	switch command {
	case "list -m":
		return CommandResult{Stdout: []byte("example.test/backend\n")}, true
	case "list ./internal/service":
		return CommandResult{Stdout: []byte("example.test/backend/internal/service\n")}, true
	case "list ./...":
		return CommandResult{Stdout: []byte("example.test/backend/internal/service\nexample.test/backend/test/integration\n")}, true
	default:
		return CommandResult{}, false
	}
}

func (runner *scriptedRunner) goListFilesOutput(dir string, command string) string {
	if output := goListFilesOutputFromDisk(dir, command); output != "" {
		return output
	}
	var out strings.Builder
	if strings.Contains(command, "./internal/service") {
		_, _ = fmt.Fprintf(&out, "%s|service.go init.go types.go constants.go\n", filepath.Join(dir, "internal/service"))
	}
	if strings.Contains(command, "./internal/other") {
		_, _ = fmt.Fprintf(&out, "%s|other.go\n", filepath.Join(dir, "internal/other"))
	}
	if strings.Contains(command, "./...") {
		_, _ = fmt.Fprintf(&out, "%s|service.go\n", filepath.Join(dir, "internal/service"))
	}
	if strings.HasSuffix(command, " .") {
		_, _ = fmt.Fprintf(&out, "%s|service.go\n", dir)
	}
	return out.String()
}

func goListFilesOutputFromDisk(dir string, command string) string {
	if strings.Contains(command, "./...") {
		return goListAllPackageFilesFromDisk(dir)
	}
	var out strings.Builder
	for _, packageDir := range []string{"internal/service", "internal/other", "."} {
		pattern := "./" + packageDir
		if packageDir == "." {
			pattern = " ."
		}
		if strings.Contains(command, pattern) {
			out.WriteString(goListPackageFilesFromDisk(filepath.Join(dir, packageDir)))
		}
	}
	return out.String()
}

func goListAllPackageFilesFromDisk(dir string) string {
	var out strings.Builder
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		out.WriteString(goListPackageFilesFromDisk(path))
		return nil
	})
	return out.String()
}

func goListPackageFilesFromDisk(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	names := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("%s|%s\n", dir, strings.Join(names, " "))
}

func (runner *scriptedRunner) complexityResult() CommandResult {
	if runner.complexityOutputSet {
		return CommandResult{Stdout: []byte(runner.complexityOutput)}
	}
	return CommandResult{Stdout: []byte("8 service Run internal/service/service.go:12:1\n")}
}

func (runner *scriptedRunner) coverResult() CommandResult {
	if runner.coverOutput != "" {
		return CommandResult{Stdout: []byte(runner.coverOutput)}
	}
	total := runner.coverageTotal
	if total == "" {
		total = "50.0%"
	}
	return CommandResult{Stdout: []byte("example.test/backend/internal/service/service.go:12:\tRun\t50.0%\ntotal:\t(statements)\t" + total + "\n")}
}

type fakeCall struct {
	dir  string
	name string
	args []string
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func oneBasedColumn(t *testing.T, line string, needle string) int {
	t.Helper()
	index := strings.Index(line, needle)
	if index < 0 {
		t.Fatalf("line %q does not contain %q", line, needle)
	}
	return index + 1
}

func duplicateSource(name string) string {
	return `package sample

func ` + name + `(items []int) []int {
	var kept []int
	for _, item := range items {
		if item%2 == 0 {
			kept = append(kept, item+1)
		}
	}
	return kept
}
`
}
