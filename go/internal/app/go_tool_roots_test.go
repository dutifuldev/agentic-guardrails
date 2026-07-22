package app

import (
	"reflect"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
	"github.com/osolmaz/slophammer/go/internal/toolchecks"
)

func TestGoToolRootsUseNestedModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"fixtures/repos/go-clean/go.mod": {Path: "fixtures/repos/go-clean/go.mod"},
		"go/go.mod":                      {Path: "go/go.mod"},
		"templates/go/go.mod":            {Path: "templates/go/go.mod"},
	})

	got := goToolRoots("..", snapshot)
	want := []string{"../go"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("goToolRoots = %#v, want %#v", got, want)
	}
}

func TestDryOptionsForModuleExpandsProductionFiles(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod":                          {Path: "go/go.mod"},
		"go/cmd/slophammer/main.go":          {Path: "go/cmd/slophammer/main.go"},
		"go/internal/app/app.go":             {Path: "go/internal/app/app.go"},
		"go/internal/app/app_test.go":        {Path: "go/internal/app/app_test.go"},
		"go/internal/app/generated/model.go": {Path: "go/internal/app/generated/model.go"},
		"go/internal/app/testdata/input.go":  {Path: "go/internal/app/testdata/input.go"},
		"fixtures/repos/go-clean/main.go":    {Path: "fixtures/repos/go-clean/main.go"},
		"templates/go/main.go":               {Path: "templates/go/main.go"},
	})

	got, ok := dryOptionsForModule(toolchecks.DryOptions{
		Root:    "..",
		Paths:   []string{"go/cmd", "go/internal"},
		Exclude: []string{"**/*_test.go", "**/generated/**", "**/testdata/**", "fixtures/**", "templates/**"},
	}, snapshot, "go")
	want := toolchecks.DryOptions{
		Root:    "../go",
		Paths:   []string{"cmd/slophammer/main.go", "internal/app/app.go"},
		Exclude: []string{"**/*_test.go", "**/generated/**", "**/testdata/**", "fixtures/**", "templates/**"},
	}

	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dryOptionsForModule = %#v, want %#v", got, want)
	}
}

func TestDryOptionsForModuleSkipsNestedModuleFiles(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":          {Path: "go.mod"},
		"main.go":         {Path: "main.go"},
		"tools/go.mod":    {Path: "tools/go.mod"},
		"tools/main.go":   {Path: "tools/main.go"},
		"tools/helper.go": {Path: "tools/helper.go"},
	})

	parent, ok := dryOptionsForModule(toolchecks.DryOptions{
		Root:    ".",
		Exclude: []string{"**/*_test.go"},
	}, snapshot, ".")
	if !ok {
		t.Fatal("parent ok = false, want true")
	}
	if !reflect.DeepEqual(parent.Paths, []string{"main.go"}) {
		t.Fatalf("parent paths = %#v, want only parent module files", parent.Paths)
	}

	child, ok := dryOptionsForModule(toolchecks.DryOptions{
		Root:    ".",
		Exclude: []string{"**/*_test.go"},
	}, snapshot, "tools")
	if !ok {
		t.Fatal("child ok = false, want true")
	}
	if !reflect.DeepEqual(child.Paths, []string{"helper.go", "main.go"}) {
		t.Fatalf("child paths = %#v, want child module files", child.Paths)
	}
}

func TestDryOptionsForModuleKeepsDotIncludePath(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go.mod":      {Path: "go.mod"},
		"main.go":     {Path: "main.go"},
		"go/go.mod":   {Path: "go/go.mod"},
		"go/main.go":  {Path: "go/main.go"},
		"go/other.go": {Path: "go/other.go"},
	})

	got, ok := dryOptionsForModule(toolchecks.DryOptions{
		Root:  ".",
		Paths: []string{"."},
	}, snapshot, ".")

	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !reflect.DeepEqual(got.Paths, []string{"main.go"}) {
		t.Fatalf("paths = %#v, want whole module", got.Paths)
	}

	nested, ok := dryOptionsForModule(toolchecks.DryOptions{
		Root:  ".",
		Paths: []string{"."},
	}, snapshot, "go")
	if !ok {
		t.Fatal("nested ok = false, want true")
	}
	if !reflect.DeepEqual(nested.Paths, []string{"main.go", "other.go"}) {
		t.Fatalf("nested paths = %#v, want whole nested module", nested.Paths)
	}
}

func TestGoIncludeRoot(t *testing.T) {
	tests := []struct {
		name       string
		moduleRoot string
		include    string
		want       string
		wantOK     bool
	}{
		{name: "empty", moduleRoot: "go", include: "", wantOK: false},
		{name: "dot", moduleRoot: "go", include: ".", want: "go", wantOK: true},
		{name: "root module", moduleRoot: ".", include: "cmd", want: "cmd", wantOK: true},
		{name: "nested module exact", moduleRoot: "go", include: "go", want: "go", wantOK: true},
		{name: "nested module child", moduleRoot: "go", include: "go/internal", want: "go/internal", wantOK: true},
		{name: "outside nested module", moduleRoot: "go", include: "templates/go", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := goIncludeRoot(tt.moduleRoot, tt.include)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("goIncludeRoot = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestMutationOptionsForModulesTrimsConfiguredTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod": {Path: "go/go.mod"},
	})

	got := mutationOptionsForModules(toolchecks.MutationOptions{
		Root:    "..",
		Targets: []string{"go/internal/rules/rules.go"},
		Scan:    true,
	}, snapshot)
	want := []toolchecks.MutationOptions{
		{Root: "../go", Targets: []string{"internal/rules/rules.go"}, Scan: true},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutationOptionsForModules = %#v, want %#v", got, want)
	}
}

func TestTargetModuleRoot(t *testing.T) {
	moduleRoots := []string{".", "services/api", "services/api/tools"}
	tests := []struct {
		target string
		want   string
	}{
		{target: "main.go", want: "."},
		{target: "services/api", want: "services/api"},
		{target: "services/api/internal/app.go", want: "services/api"},
		{target: "services/api/tools/main.go", want: "services/api/tools"},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := targetModuleRoot(tt.target, moduleRoots); got != tt.want {
				t.Fatalf("targetModuleRoot = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTargetModuleRootKeepsResolvedRootFilesAtRepoRoot(t *testing.T) {
	if got := targetModuleRoot("cmd/main.go", []string{"go"}); got != "." {
		t.Fatalf("targetModuleRoot = %q, want .", got)
	}
}

func TestMutationOptionsForModulesKeepsRootFilesOutOfNestedModule(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo", map[string]repo.File{
		"go/go.mod": {Path: "go/go.mod"},
	})

	got := mutationOptionsForModules(toolchecks.MutationOptions{
		Root:    ".",
		Targets: []string{"main.go", "go/internal/rules/rules.go"},
		Scan:    true,
	}, snapshot)
	want := []toolchecks.MutationOptions{
		{Root: ".", Targets: []string{"main.go"}, Scan: true},
		{Root: "go", Targets: []string{"internal/rules/rules.go"}, Scan: true},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutationOptionsForModules = %#v, want %#v", got, want)
	}
}

func TestMutationOptionsForModulesKeepsRootModuleTargets(t *testing.T) {
	snapshot := repo.NewSnapshot("/repo/go", map[string]repo.File{
		"go.mod": {Path: "go.mod"},
	})

	got := mutationOptionsForModules(toolchecks.MutationOptions{
		Root:   ".",
		Target: "internal/rules/rules.go",
		Scan:   true,
	}, snapshot)
	want := []toolchecks.MutationOptions{
		{Root: ".", Targets: []string{"internal/rules/rules.go"}, Scan: true},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutationOptionsForModules = %#v, want %#v", got, want)
	}
}
