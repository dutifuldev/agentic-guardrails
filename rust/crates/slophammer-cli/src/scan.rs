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
    scan_repo_with_ignores(root, false)
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
