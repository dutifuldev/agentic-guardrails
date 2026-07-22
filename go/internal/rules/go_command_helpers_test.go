package rules

import (
	"testing"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

func TestContainsCommandWordRespectsWordBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		evidence string
		word     string
		want     bool
	}{
		{name: "exact word", evidence: "run make test", word: "make", want: true},
		{name: "letter prefix", evidence: "run remake test", word: "make", want: false},
		{name: "underscore suffix", evidence: "make_target", word: "make", want: false},
		{name: "dash suffix", evidence: "make-target", word: "make", want: false},
		{name: "digit suffix", evidence: "make4 things", word: "make", want: false},
		{name: "later occurrence matches", evidence: "remake then make it", word: "make", want: true},
		{name: "punctuation boundary", evidence: "(make)", word: "make", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsCommandWord(tt.evidence, tt.word); got != tt.want {
				t.Fatalf("containsCommandWord(%q, %q) = %v, want %v", tt.evidence, tt.word, got, tt.want)
			}
		})
	}
}

func TestLineHasGoCommandSignal(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "go test", line: "go test ./...", want: true},
		{name: "go global flag", line: "go -C tools vet ./...", want: true},
		{name: "env prefix", line: "GOFLAGS=-mod=readonly go build ./...", want: true},
		{name: "unsupported command", line: "go version", want: false},
		{name: "after shell separator", line: "cd go && go test ./...", want: true},
		{name: "not command token", line: "echo go test ./...", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineHasGoCommandSignal(commandTokens(tt.line)); got != tt.want {
				t.Fatalf("lineHasGoCommandSignal(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestPriorCDWorkingDirectory(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   string
		wantOK bool
	}{
		{name: "none", line: "go run ./cmd/slophammer go crap", wantOK: false},
		{name: "simple", line: "cd go && go run ./cmd/slophammer go crap ..", want: "go", wantOK: true},
		{name: "last wins", line: "cd services && cd api && go test ./...", want: "api", wantOK: true},
		{name: "flag ignored", line: "cd - && go test ./...", wantOK: false},
		{name: "separator ignored", line: "cd && go test ./...", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := priorCDWorkingDirectory(commandTokens(tt.line))
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("priorCDWorkingDirectory(%q) = %q, %v; want %q, %v", tt.line, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFirstSlophammerGoPathArgument(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   string
		wantOK bool
	}{
		{name: "default", line: "--scan", wantOK: false},
		{name: "path", line: ".. --scan", want: "..", wantOK: true},
		{name: "flag value", line: "--target internal/rules/rules.go ..", want: "..", wantOK: true},
		{name: "equals flag", line: "--target=internal/rules/rules.go ..", want: "..", wantOK: true},
		{name: "separator stops", line: "&& ..", wantOK: false},
		{name: "blank skipped", line: "   ..", want: "..", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := firstSlophammerGoPathArgument(commandTokens(tt.line))
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("firstSlophammerGoPathArgument(%q) = %q, %v; want %q, %v", tt.line, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestLineHasSlophammerGoCommandAcceptsTransitionForms(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "public direct binary", line: "slophammer-go dry .", want: true},
		{name: "public binary legacy namespace", line: "slophammer-go go dry .", want: true},
		{name: "legacy binary direct command", line: "slophammer dry .", want: true},
		{name: "legacy binary namespace", line: "slophammer go dry .", want: true},
		{name: "source public direct command", line: "go run ./cmd/slophammer-go dry .", want: true},
		{name: "released public direct command", line: "go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.1 dry .", want: true},
		{name: "released public direct command with variable version", line: "go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@${SLOPHAMMER_GO_VERSION} dry .", want: true},
		{name: "source public legacy namespace", line: "go run ./cmd/slophammer-go go dry .", want: true},
		{name: "source legacy direct command", line: "go run ./cmd/slophammer dry .", want: true},
		{name: "source legacy namespace", line: "go run ./cmd/slophammer go dry .", want: true},
		{name: "wrong subcommand", line: "slophammer check .", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineHasSlophammerGoCommand(commandTokens(tt.line), "dry", "")
			if got != tt.want {
				t.Fatalf("lineHasSlophammerGoCommand(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestFileHasConfigBackedSlophammerGoCommand(t *testing.T) {
	tests := []struct {
		name       string
		file       repo.File
		subcommand string
		want       bool
	}{
		{
			name:       "workflow working directory root path",
			subcommand: "crap",
			file: repo.File{
				Path: ".github/workflows/ci.yml",
				Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - name: crap
        working-directory: go
        run: go run ./cmd/slophammer go crap ..
`,
			},
			want: true,
		},
		{
			name:       "workflow wrong config root path",
			subcommand: "crap",
			file: repo.File{
				Path: ".github/workflows/ci.yml",
				Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - name: crap
        working-directory: go
        run: go run ./cmd/slophammer go crap ../tmp
`,
			},
			want: false,
		},
		{
			name:       "script default root",
			subcommand: "mutate",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "go run ./cmd/slophammer go mutate\n",
			},
			want: true,
		},
		{
			name:       "script public binary legacy namespace",
			subcommand: "mutate",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer-go go mutate\n",
			},
			want: true,
		},
		{
			name:       "script legacy binary direct command",
			subcommand: "crap",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer crap --max-score 8\n",
			},
			want: true,
		},
		{
			name:       "script wrong subcommand",
			subcommand: "crap",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "go run ./cmd/slophammer go mutate\n",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileHasConfigBackedSlophammerGoCommand(tt.file, tt.subcommand)
			if got != tt.want {
				t.Fatalf("fileHasConfigBackedSlophammerGoCommand = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileHasConfigBackedSlophammerGoCheckExecuteCommand(t *testing.T) {
	tests := []struct {
		name string
		file repo.File
		want bool
	}{
		{
			name: "workflow working directory root path",
			file: repo.File{
				Path: ".github/workflows/ci.yml",
				Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - name: slophammer
        working-directory: go
        run: go run github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.5 check .. --execute
`,
			},
			want: true,
		},
		{
			name: "workflow wrong config root path",
			file: repo.File{
				Path: ".github/workflows/ci.yml",
				Content: `name: CI
on: [push]
jobs:
  test:
    steps:
      - name: slophammer
        working-directory: go
        run: slophammer-go check ../tmp --execute
`,
			},
			want: false,
		},
		{
			name: "script default root",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer-go check . --execute\n",
			},
			want: true,
		},
		{
			name: "script execute before path",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer-go check --execute .\n",
			},
			want: true,
		},
		{
			name: "script coverage profile before path",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer-go check --execute --coverage-profile coverage.out .\n",
			},
			want: true,
		},
		{
			name: "script format flag",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer-go check --format json . --execute\n",
			},
			want: true,
		},
		{
			name: "script missing execute",
			file: repo.File{
				Path:    "scripts/check.sh",
				Content: "slophammer-go check .\n",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileHasConfigBackedSlophammerGoCheckExecuteCommand(tt.file)
			if got != tt.want {
				t.Fatalf("fileHasConfigBackedSlophammerGoCheckExecuteCommand = %v, want %v", got, tt.want)
			}
		})
	}
}
