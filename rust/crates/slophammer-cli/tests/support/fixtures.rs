use std::fs;
use std::path::Path;
use tempfile::{Builder, TempDir};

pub fn fixture(name: &str) -> TempDir {
    let root = temp_root(name);
    write_rust_fixture(root.path(), name);
    root
}

pub fn fixture_path(root: &TempDir) -> String {
    root.path().to_string_lossy().into_owned()
}

pub fn temp_root(name: &str) -> TempDir {
    Builder::new()
        .prefix(&format!("slophammer-rs-{name}-"))
        .tempdir()
        .expect("create temp root")
}

pub fn write_file(root: &Path, path: &str, content: &str) {
    let path = root.join(path);
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).expect("create parent");
    }
    fs::write(path, content).expect("write file");
}

fn write_rust_fixture(root: &Path, name: &str) {
    match name {
        "rust-clean" => write_clean_fixture(
            root,
            CLEAN_WORKFLOW,
            "pub fn message() -> &'static str {\n    \"ok\"\n}\n",
        ),
        "rust-bad-dependency" => write_bad_dependency_fixture(root),
        "rust-invalid-config" => write_invalid_config_fixture(root),
        "rust-unknown-config" => write_unknown_config_fixture(root),
        "rust-unsafe" => write_unsafe_fixture(root),
        _ => write_clean_fixture(
            root,
            "jobs:\n  rust:\n    steps:\n      - run: cargo check --workspace\n",
            "pub fn message() -> &'static str {\n    \"ok\"\n}\n",
        ),
    }
}

fn write_clean_fixture(root: &Path, workflow: &str, source: &str) {
    write_file(root, "README.md", "# Rust Fixture\n");
    write_file(
        root,
        "AGENTS.md",
        "# Agents\n\nRun `cargo test` before finishing.\n",
    );
    write_file(
        root,
        "Cargo.toml",
        "[package]\nname = \"rust-fixture\"\nversion = \"0.1.0\"\nedition = \"2024\"\nrust-version = \"1.86\"\n",
    );
    write_file(root, "src/lib.rs", source);
    write_file(root, "clippy.toml", "cognitive-complexity-threshold = 8\n");
    write_file(root, ".github/workflows/ci.yml", workflow);
    write_file(root, "slophammer.yml", CLEAN_CONFIG);
}

fn write_bad_dependency_fixture(root: &Path) {
    write_clean_fixture(
        root,
        CLEAN_WORKFLOW,
        "pub fn message() -> &'static str {\n    local_dep::message()\n}\n",
    );
    write_file(
        root,
        "Cargo.toml",
        "[package]\nname = \"rust-fixture\"\nversion = \"0.1.0\"\nedition = \"2024\"\nrust-version = \"1.86\"\n\n[dependencies]\nlocal-dep = { path = \"local-dep\" }\n",
    );
    write_file(
        root,
        "local-dep/Cargo.toml",
        "[package]\nname = \"local-dep\"\nversion = \"0.1.0\"\nedition = \"2024\"\nrust-version = \"1.86\"\n",
    );
    write_file(
        root,
        "local-dep/src/lib.rs",
        "pub fn message() -> &'static str {\n    \"dep\"\n}\n",
    );
}

fn write_unsafe_fixture(root: &Path) {
    write_clean_fixture(
        root,
        CLEAN_WORKFLOW,
        "pub fn pointer_is_null() -> bool {\n    let value = unsafe { core::ptr::null::<u8>().as_ref() };\n    value.is_none()\n}\n",
    );
}

fn write_invalid_config_fixture(root: &Path) {
    write_clean_fixture(
        root,
        CLEAN_WORKFLOW,
        "pub fn message() -> &'static str {\n    \"ok\"\n}\n",
    );
    write_file(
        root,
        "slophammer.yml",
        "rust:\n  coverage:\n    threshold: 40\n",
    );
}

fn write_unknown_config_fixture(root: &Path) {
    write_clean_fixture(
        root,
        CLEAN_WORKFLOW,
        "pub fn message() -> &'static str {\n    \"ok\"\n}\n",
    );
    write_file(root, "slophammer.yml", "rust:\n  not_a_real_key: true\n");
}

const CLEAN_WORKFLOW: &str = r#"name: CI
on: [push]
jobs:
  rust:
    runs-on: ubuntu-latest
    steps:
      - run: cargo check --workspace
      - run: cargo fmt --check
      - run: cargo clippy --workspace --all-targets -- -D warnings
      - run: cargo test --workspace --all-targets
      - run: cargo llvm-cov --workspace --fail-under-lines 85
      - run: cargo audit
      - run: slophammer-rs dry .
      - run: slophammer-rs check .
      - run: cargo mutants --workspace
"#;

const CLEAN_CONFIG: &str = r#"rust:
  coverage:
    threshold: 85
  complexity:
    cognitive_max: 8
  targets:
    - src
  dry:
    max_findings: 0
    paths:
      - src
    copied_blocks:
      enabled: true
      min_tokens: 100
  unsafe:
    policy: forbid
  mutation:
    targets:
      - src
  dependency_boundaries:
    - from: .
      allow: []
"#;
