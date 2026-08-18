use camino::Utf8PathBuf;
use ignore::WalkBuilder;
use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use thiserror::Error;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct RepoFile {
    pub path: String,
    pub content: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Snapshot {
    pub root: PathBuf,
    pub files: BTreeMap<String, RepoFile>,
}

impl Snapshot {
    pub fn file(&self, path: &str) -> Option<&RepoFile> {
        self.files.get(path)
    }

    pub fn has_case_insensitive(&self, filename: &str) -> bool {
        self.files
            .keys()
            .filter(|path| !path.contains('/'))
            .any(|path| path.eq_ignore_ascii_case(filename))
    }

    pub fn has_workflow(&self) -> bool {
        self.files.keys().any(|path| active_workflow_path(path))
    }
}

fn active_workflow_path(path: &str) -> bool {
    let Some(name) = path.strip_prefix(".github/workflows/") else {
        return false;
    };
    !name.contains('/') && (name.ends_with(".yml") || name.ends_with(".yaml"))
}

#[derive(Debug, Error)]
pub enum ScanError {
    #[error("repository root does not exist: {0}")]
    MissingRoot(String),
    #[error("walk failed: {0}")]
    Walk(#[from] ignore::Error),
    #[error("path is not valid UTF-8: {0}")]
    NonUtf8Path(String),
}

pub fn scan_repo(root: impl AsRef<Path>) -> Result<Snapshot, ScanError> {
    scan_repo_with_ignores(root, true)
}

pub fn scan_repo_unignored(root: impl AsRef<Path>) -> Result<Snapshot, ScanError> {
    let root = root.as_ref();
    if !root.exists() {
        return Err(ScanError::MissingRoot(root.display().to_string()));
    }
    let root = root.to_path_buf();
    let mut files = BTreeMap::new();
    let mut builder = WalkBuilder::new(&root);
    let filter_root = root.clone();
    builder
        .hidden(false)
        .ignore(false)
        .git_ignore(false)
        .git_global(false)
        .git_exclude(false)
        .parents(false)
        .filter_entry(move |entry| {
            entry.path() == filter_root || !ignored_agent_entry(entry.path())
        });
    for entry in builder.build() {
        let entry = entry?;
        if !entry.file_type().is_some_and(|item| item.is_file())
            || !agent_evidence_file(&root, entry.path())
        {
            continue;
        }
        let Some(path) = relative_path(&root, entry.path())? else {
            continue;
        };
        let content = bounded_content(entry.path());
        files.insert(path.clone(), RepoFile { path, content });
    }
    Ok(Snapshot { root, files })
}

const MAX_AGENT_FILE_BYTES: u64 = 1 << 20;

fn bounded_content(path: &Path) -> String {
    let readable = fs::metadata(path).is_ok_and(|metadata| metadata.len() <= MAX_AGENT_FILE_BYTES);
    if !readable {
        return String::new();
    }
    fs::read_to_string(path).unwrap_or_default()
}

fn agent_evidence_file(root: &Path, path: &Path) -> bool {
    let Some(name) = path.file_name().and_then(|name| name.to_str()) else {
        return false;
    };
    if name.eq_ignore_ascii_case("AGENTS.md") {
        return true;
    }
    if matches!(
        name,
        "Cargo.toml"
            | "go.mod"
            | "package.json"
            | "pyproject.toml"
            | "package-lock.json"
            | "pnpm-lock.yaml"
            | "yarn.lock"
            | "bun.lock"
            | "bun.lockb"
            | "uv.lock"
    ) {
        return true;
    }
    path.parent() == Some(root)
        && matches!(
            name,
            "Makefile" | "makefile" | "Taskfile.yml" | "Taskfile.yaml" | "justfile"
        )
}

fn scan_repo_with_ignores(
    root: impl AsRef<Path>,
    respect_git_ignores: bool,
) -> Result<Snapshot, ScanError> {
    let root = root.as_ref();
    if !root.exists() {
        return Err(ScanError::MissingRoot(root.display().to_string()));
    }
    let root = root.to_path_buf();
    let mut files = BTreeMap::new();
    let mut builder = WalkBuilder::new(&root);
    builder
        .hidden(false)
        .ignore(respect_git_ignores)
        .git_ignore(respect_git_ignores)
        .git_global(respect_git_ignores)
        .git_exclude(respect_git_ignores)
        .parents(respect_git_ignores)
        .filter_entry(|entry| !ignored_entry(entry.path()));
    let walker = builder.build();
    for entry in walker {
        let entry = entry?;
        let file_type = entry.file_type();
        if !file_type.is_some_and(|item| item.is_file()) {
            continue;
        }
        let Some(path) = relative_path(&root, entry.path())? else {
            continue;
        };
        if let Ok(content) = fs::read_to_string(entry.path()) {
            files.insert(path.clone(), RepoFile { path, content });
        }
    }
    Ok(Snapshot { root, files })
}

fn relative_path(root: &Path, path: &Path) -> Result<Option<String>, ScanError> {
    let Ok(relative) = path.strip_prefix(root) else {
        return Ok(None);
    };
    let utf8 = Utf8PathBuf::from_path_buf(relative.to_path_buf())
        .map_err(|path| ScanError::NonUtf8Path(path.display().to_string()))?;
    Ok(Some(utf8.as_str().replace('\\', "/")))
}

fn ignored_agent_entry(path: &Path) -> bool {
    path.file_name()
        .and_then(|name| name.to_str())
        .is_some_and(|name| {
            name.starts_with('.')
                || matches!(
                    name,
                    "node_modules"
                        | ".venv"
                        | "target"
                        | "dist"
                        | "build"
                        | "coverage"
                        | "vendor"
                        | "fixtures"
                        | "templates"
                        | "testdata"
                        | "tests"
                        | "test"
                        | "scripts"
                        | "__pycache__"
                )
        })
}

fn ignored_entry(path: &Path) -> bool {
    path.file_name()
        .and_then(|name| name.to_str())
        .is_some_and(|name| {
            matches!(
                name,
                ".git" | "node_modules" | "target" | "dist" | "coverage"
            )
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn unignored_scan_reads_only_agent_evidence() {
        let root = tempfile::tempdir().expect("create temp root");
        fs::create_dir(root.path().join(".git")).expect("create git directory");
        fs::write(root.path().join(".gitignore"), "ignored/\n").expect("write gitignore");
        fs::create_dir(root.path().join("ignored")).expect("create ignored directory");
        fs::write(
            root.path().join("ignored/package.json"),
            "{\"scripts\":{\"check\":\"true\"}}",
        )
        .expect("write package manifest");
        fs::write(root.path().join("ignored/large.log"), vec![b'x'; 2 << 20])
            .expect("write ignored log");
        fs::write(
            root.path().join("ignored/pnpm-lock.yaml"),
            vec![b'x'; 2 << 20],
        )
        .expect("write large agent evidence");

        let regular = scan_repo(root.path()).expect("scan regular repository");
        assert!(!regular.files.contains_key("ignored/package.json"));
        let agents = scan_repo_unignored(root.path()).expect("scan agent evidence");
        assert!(agents.files.contains_key("ignored/package.json"));
        assert!(!agents.files.contains_key("ignored/large.log"));
        assert_eq!(agents.files["ignored/pnpm-lock.yaml"].content, "");
    }

    #[test]
    fn unignored_scan_does_not_filter_the_repository_root() {
        let parent = tempfile::tempdir().expect("create temp parent");
        let root = parent.path().join("test");
        fs::create_dir(&root).expect("create test-named root");
        fs::write(
            root.join("AGENTS.md"),
            "# Agents\n\nUseful instructions live here.\n",
        )
        .expect("write AGENTS.md");

        let snapshot = scan_repo_unignored(&root).expect("scan test-named root");
        assert!(snapshot.files.contains_key("AGENTS.md"));
    }

    #[test]
    fn detects_workflow_paths() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                ".github/workflows/ci.yml".to_owned(),
                RepoFile {
                    path: ".github/workflows/ci.yml".to_owned(),
                    content: String::new(),
                },
            )]),
        };
        assert!(snapshot.has_workflow());
    }

    #[test]
    fn ignores_nested_archived_workflow_paths() {
        let snapshot = Snapshot {
            root: PathBuf::from("."),
            files: BTreeMap::from([(
                ".github/workflows/archive/old.yml".to_owned(),
                RepoFile {
                    path: ".github/workflows/archive/old.yml".to_owned(),
                    content: String::new(),
                },
            )]),
        };
        assert!(!snapshot.has_workflow());
    }
}
