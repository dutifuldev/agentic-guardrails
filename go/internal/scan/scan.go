package scan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/osolmaz/slophammer/go/internal/repo"
)

const maxFileBytes = 1 << 20

func Repo(root string) (repo.Snapshot, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return repo.Snapshot{}, err
	}
	scanner := repoScanner{
		root:  cleanRoot,
		files: map[string]repo.File{},
	}
	if err := filepath.WalkDir(cleanRoot, scanner.visit); err != nil {
		return repo.Snapshot{}, err
	}
	return repo.NewSnapshot(cleanRoot, scanner.files), nil
}

type repoScanner struct {
	root  string
	files map[string]repo.File
}

func (s repoScanner) visit(filePath string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() {
		return s.visitDir(filePath, entry)
	}
	if !entry.Type().IsRegular() {
		return nil
	}
	relPath, err := filepath.Rel(s.root, filePath)
	if err != nil {
		return err
	}
	relPath = filepath.ToSlash(relPath)
	content, err := readSmallTextFile(filePath, entry)
	if err != nil {
		return err
	}
	s.files[relPath] = repo.File{Path: relPath, Content: content}
	return nil
}

func (s repoScanner) visitDir(filePath string, entry os.DirEntry) error {
	if shouldSkipDir(entry.Name()) && filePath != s.root {
		return filepath.SkipDir
	}
	return nil
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".venv", ".mypy_cache", ".pytest_cache", ".ruff_cache", "__pycache__":
		return true
	default:
		return false
	}
}

func readSmallTextFile(filePath string, entry os.DirEntry) (string, error) {
	info, err := entry.Info()
	if err != nil {
		return "", err
	}
	if info.Size() > maxFileBytes {
		return "", nil
	}
	// #nosec G304 -- filePath comes from filepath.WalkDir rooted at the target repository.
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(string(content), '\x00') {
		return "", nil
	}
	return string(content), nil
}
