package agents

import (
	"slices"
	"strings"
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestRenderDerivesRunnerAndPackageFacts(t *testing.T) {
	snapshot := testSnapshot(map[string]string{
		"Makefile":                "check:\n\t@true\n",
		"Taskfile.yml":            "check:\n  cmds: [true]\n",
		"justfile":                "check:\n  true\n",
		"go/go.mod":               "module example.com/demo\n",
		"rust/Cargo.toml":         "[workspace]\nmembers = []\n",
		"rust/crate/Cargo.toml":   "[package]\nname = \"demo\"\n",
		"python/pyproject.toml":   "[dependency-groups]\ndev = [\"pytest\"]\n",
		"python/uv.lock":          "version = 1\n",
		"plain/pyproject.toml":    "[project]\nname = \"plain\"\n",
		"npm/package.json":        `{"scripts":{"test":"node --test"}}`,
		"pnpm/package.json":       `{"scripts":{"check":"true"}}`,
		"pnpm/pnpm-lock.yaml":     "lockfileVersion: 9\n",
		"yarn/package.json":       `{"scripts":{"build":"true"}}`,
		"yarn/yarn.lock":          "",
		"bun/package.json":        `{"scripts":{"test":"true"}}`,
		"bun/bun.lock":            "",
		"invalid/package.json":    "{",
		"no-scripts/package.json": `{"name":"empty"}`,
		"nested/Taskfile.yaml":    "check:\n",
		"ignored/Cargo.lock":      "",
		"fixtures/demo/go.mod":    "module example.com/ignored\n",
	})

	evidence := Derive(snapshot)
	if got := strings.Join(evidence.GlobalCommands, ","); got != "make check,task check,just check" {
		t.Fatalf("global commands = %q", got)
	}
	for _, command := range []string{
		"go test ./...",
		"cargo test --workspace",
		"cargo test",
		"uv run pytest",
		"npm test",
		"pnpm run check",
		"yarn run build",
		"bun test",
	} {
		if !slices.Contains(evidence.AllCommands, command) {
			t.Fatalf("all commands do not contain %q: %#v", command, evidence.AllCommands)
		}
	}
	rendered := Render(snapshot)
	if !strings.Contains(rendered, "```sh\nmake check\ntask check\njust check\n```") {
		t.Fatalf("rendered commands missing: %s", rendered)
	}
	if strings.Contains(rendered, "fixtures/demo") {
		t.Fatalf("rendered ignored fixture package: %s", rendered)
	}
	if !strings.Contains(rendered, "- `rust`: `Cargo.toml`") {
		t.Fatalf("rendered package missing: %s", rendered)
	}
}

func TestDeriveUsesWorkspacePackageManagers(t *testing.T) {
	lockSnapshot := testSnapshot(map[string]string{
		"pnpm-lock.yaml":            "lockfileVersion: 9\n",
		"packages/app/package.json": `{"scripts":{"check":"true"}}`,
	})
	if !slices.Contains(Derive(lockSnapshot).AllCommands, "pnpm run check") {
		t.Fatalf("lockfile commands = %#v", Derive(lockSnapshot).AllCommands)
	}

	metadataSnapshot := testSnapshot(map[string]string{
		"package.json":              `{"packageManager":"yarn@4.1.0"}`,
		"packages/app/package.json": `{"scripts":{"build":"true"}}`,
	})
	if !slices.Contains(Derive(metadataSnapshot).AllCommands, "yarn run build") {
		t.Fatalf("metadata commands = %#v", Derive(metadataSnapshot).AllCommands)
	}
}

func TestRenderWithoutEvidenceIsExplicit(t *testing.T) {
	rendered := Render(testSnapshot(nil))
	if !strings.Contains(rendered, "No verification command could be derived") ||
		!strings.Contains(rendered, "No package manifest was detected") {
		t.Fatalf("rendered = %q", rendered)
	}
}

func TestEvaluateAgentInstructionFailures(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "missing", files: nil, want: []string{"repo.agents-required"}},
		{name: "empty", files: map[string]string{"AGENTS.md": "# Agents\n"}, want: []string{"repo.agents-empty"}},
		{
			name: "command missing",
			files: map[string]string{
				"AGENTS.md":    "# Agents\n\nKeep changes small and reviewable.\n",
				"package.json": `{"scripts":{"check":"true"}}`,
			},
			want: []string{"repo.agents-commands-required"},
		},
		{
			name: "nested scope",
			files: map[string]string{
				"AGENTS.md":                    "# Agents\n\nRun `npm test` before finishing.\n",
				"packages/app/package.json":    `{"scripts":{"test":"true"}}`,
				"packages/worker/package.json": `{"scripts":{"build":"true"}}`,
			},
			want: []string{"repo.agents-scope-required"},
		},
		{
			name: "invalid and stale block",
			files: map[string]string{
				"AGENTS.md": "# AGENTS.md\n\nKeep changes small and reviewable.\n\n" + StartMarker + "\n## Repository checks\n\n```sh\nnpm test\n```\n" + EndMarker + "\n",
			},
			want: []string{"repo.agents-command-invalid", "repo.agents-stale"},
		},
		{
			name: "unterminated managed block",
			files: map[string]string{
				"AGENTS.md": "# AGENTS.md\n\nKeep changes small and reviewable.\n\n" + StartMarker + "\n",
			},
			want: []string{"repo.agents-stale"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issues := Evaluate(testSnapshot(test.files))
			got := make([]string, 0, len(issues))
			for _, issue := range issues {
				got = append(got, issue.RuleID)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("issues = %#v, want %#v", issues, test.want)
			}
		})
	}
}

func TestEvaluateTreatsSupportedCommandAsUseful(t *testing.T) {
	issues := Evaluate(testSnapshot(map[string]string{
		"AGENTS.md": "```sh\ngo test ./...\n```\n",
		"go.mod":    "module example.com/demo\n",
	}))
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestEvaluateAcceptsGeneratedAndNestedInstructions(t *testing.T) {
	snapshot := testSnapshot(map[string]string{
		"packages/app/package.json": `{"scripts":{"check":"true"}}`,
	})
	generated := Render(snapshot)
	snapshot.Files["AGENTS.md"] = repo.File{Path: "AGENTS.md", Content: generated}
	if issues := Evaluate(snapshot); len(issues) != 0 {
		t.Fatalf("generated issues = %#v", issues)
	}

	manual := testSnapshot(map[string]string{
		"AGENTS.md":                 "# Agents\n\nFollow the repository checks before finishing.\n",
		"packages/app/AGENTS.md":    "# App agents\n\nRun `npm run check` before finishing.\n",
		"packages/app/package.json": `{"scripts":{"check":"true"}}`,
	})
	if issues := Evaluate(manual); len(issues) != 1 || issues[0].RuleID != "repo.agents-commands-required" {
		t.Fatalf("manual issues = %#v", issues)
	}
}

func testSnapshot(files map[string]string) repo.Snapshot {
	repoFiles := map[string]repo.File{}
	for filePath, content := range files {
		repoFiles[filePath] = repo.File{Path: filePath, Content: content}
	}
	return repo.NewSnapshot("/repo", repoFiles)
}
