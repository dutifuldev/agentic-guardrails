use crate::core::{RuleDefinition, Severity};

pub mod rule_ids {
    pub const README_REQUIRED: &str = "repo.readme-required";
    pub const AGENTS_REQUIRED: &str = "repo.agents-required";
    pub const AGENTS_EMPTY: &str = "repo.agents-empty";
    pub const AGENTS_COMMANDS_REQUIRED: &str = "repo.agents-commands-required";
    pub const AGENTS_COMMAND_INVALID: &str = "repo.agents-command-invalid";
    pub const AGENTS_SCOPE_REQUIRED: &str = "repo.agents-scope-required";
    pub const AGENTS_STALE: &str = "repo.agents-stale";
    pub const CI_REQUIRED: &str = "repo.ci-required";
    pub const SLOPHAMMER_CI_REQUIRED: &str = "repo.slophammer-ci-required";
    pub const RUST_MANIFEST_REQUIRED: &str = "rust.manifest-required";
    pub const RUST_MSRV_REQUIRED: &str = "rust.msrv-required";
    pub const RUST_CHECK_REQUIRED: &str = "rust.check-required";
    pub const RUST_FMT_REQUIRED: &str = "rust.fmt-required";
    pub const RUST_CLIPPY_REQUIRED: &str = "rust.clippy-required";
    pub const RUST_TEST_REQUIRED: &str = "rust.test-required";
    pub const RUST_COVERAGE_REQUIRED: &str = "rust.coverage-required";
    pub const RUST_COMPLEXITY_REQUIRED: &str = "rust.complexity-required";
    pub const RUST_DRY_REQUIRED: &str = "rust.dry-required";
    pub const RUST_MUTATION_REQUIRED: &str = "rust.mutation-required";
    pub const RUST_UNSAFE_POLICY_REQUIRED: &str = "rust.unsafe-policy-required";
    pub const RUST_DEPENDENCY_AUDIT_REQUIRED: &str = "rust.dependency-audit-required";
    pub const RUST_DEPENDENCY_BOUNDARIES_REQUIRED: &str = "rust.dependency-boundaries-required";
    pub const RUST_SCOPE_INCOMPLETE: &str = "rust.scope-incomplete";
    pub const RUST_SUPPRESSIONS_JUSTIFIED: &str = "rust.suppressions-justified";
}

pub fn default_definitions() -> Vec<RuleDefinition> {
    vec![
        RuleDefinition {
            id: rule_ids::README_REQUIRED,
            title: "README required",
            category: "repo",
            severity: Severity::Error,
            path: "README.md",
            message: "README.md is required",
            description: "The target repo should have a README.md.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::AGENTS_REQUIRED,
            title: "Agent instructions required",
            category: "repo",
            severity: Severity::Error,
            path: "AGENTS.md",
            message: "AGENTS.md is required",
            description: "The target repo should have an AGENTS.md.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::AGENTS_EMPTY,
            title: "Useful agent instructions required",
            category: "repo",
            severity: Severity::Error,
            path: "AGENTS.md",
            message: "AGENTS.md must contain useful repository instructions",
            description: "The root AGENTS.md must contain useful text or name a verification command supported by repository evidence.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::AGENTS_COMMANDS_REQUIRED,
            title: "Agent verification commands required",
            category: "repo",
            severity: Severity::Error,
            path: "AGENTS.md",
            message: "AGENTS.md must name a verification command supported by the repository",
            description: "When verification commands can be derived from repository evidence, the governing AGENTS.md must name at least one of them.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::AGENTS_COMMAND_INVALID,
            title: "Generated agent commands must be valid",
            category: "repo",
            severity: Severity::Error,
            path: "AGENTS.md",
            message: "AGENTS.md contains generated commands without repository evidence",
            description: "Commands in a Slophammer-managed AGENTS.md evidence block must still be supported by current repository evidence.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::AGENTS_SCOPE_REQUIRED,
            title: "Package agent instructions required",
            category: "repo",
            severity: Severity::Error,
            path: "AGENTS.md",
            message: "Package instructions must name a verification command for this package",
            description: "Each package boundary must be governed by an AGENTS.md that names a root umbrella command or a command derived for that package.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::AGENTS_STALE,
            title: "Generated agent evidence must be current",
            category: "repo",
            severity: Severity::Error,
            path: "AGENTS.md",
            message: "The Slophammer AGENTS.md evidence block is stale",
            description: "A Slophammer-managed AGENTS.md evidence block must match the package and command facts derived from the current repository.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::CI_REQUIRED,
            title: "CI workflow required",
            category: "repo",
            severity: Severity::Error,
            path: ".github/workflows",
            message: ".github/workflows must contain at least one .yml or .yaml workflow",
            description: "The target repo should have a CI workflow under .github/workflows.",
            tool: None,
            status: "implemented",
        },
        RuleDefinition {
            id: rule_ids::SLOPHAMMER_CI_REQUIRED,
            title: "Slophammer enforcement required",
            category: "repo",
            severity: Severity::Error,
            path: ".github/workflows",
            message: "CI must run a Slophammer checker when slophammer.yml is present",
            description: "A repository that carries slophammer.yml must execute a Slophammer checker from binding CI evidence; config without enforcement is decoration.",
            tool: None,
            status: "implemented",
        },
        rust_definition(
            rule_ids::RUST_MANIFEST_REQUIRED,
            "Rust manifest required",
            "Cargo.toml",
            "Rust projects must include Cargo.toml",
            "Rust projects should include a Cargo manifest.",
            Some("cargo"),
        ),
        rust_definition(
            rule_ids::RUST_MSRV_REQUIRED,
            "Rust MSRV required",
            "Cargo.toml",
            "Rust projects must declare a minimum supported Rust version",
            "Rust projects should declare a minimum supported Rust version.",
            Some("cargo"),
        ),
        rust_definition(
            rule_ids::RUST_CHECK_REQUIRED,
            "Rust cargo check required",
            ".github/workflows",
            "Rust projects must declare cargo check in CI or scripts",
            "Rust projects should declare cargo check in an inspectable workflow or script.",
            Some("cargo check"),
        ),
        rust_definition(
            rule_ids::RUST_FMT_REQUIRED,
            "Rust formatter required",
            ".github/workflows",
            "Rust projects must declare cargo fmt --check in CI or scripts",
            "Rust projects should declare cargo fmt --check in CI or scripts.",
            Some("cargo fmt"),
        ),
        rust_definition(
            rule_ids::RUST_CLIPPY_REQUIRED,
            "Rust Clippy required",
            ".github/workflows",
            "Rust projects must declare cargo clippy in CI or scripts",
            "Rust projects should declare cargo clippy with warnings denied.",
            Some("cargo clippy"),
        ),
        rust_definition(
            rule_ids::RUST_TEST_REQUIRED,
            "Rust tests required",
            ".github/workflows",
            "Rust projects must declare cargo test in CI or scripts",
            "Rust projects should declare cargo test in CI or scripts.",
            Some("cargo test"),
        ),
        rust_definition(
            rule_ids::RUST_COVERAGE_REQUIRED,
            "Rust coverage gate required",
            ".github/workflows",
            "Rust projects must declare a coverage gate",
            "Rust projects should declare an enforceable coverage gate.",
            Some("cargo llvm-cov"),
        ),
        rust_definition(
            rule_ids::RUST_COMPLEXITY_REQUIRED,
            "Rust complexity required",
            ".github/workflows",
            "Rust projects must enforce complexity limits",
            "Rust projects should enforce complexity limits through Clippy or Slophammer policy.",
            Some("clippy"),
        ),
        rust_definition(
            rule_ids::RUST_DRY_REQUIRED,
            "Rust DRY check required",
            ".github/workflows",
            "Rust projects must declare a DRY check",
            "Rust projects should declare Slophammer's native Rust DRY check.",
            Some("slophammer-rs dry"),
        ),
        rust_definition(
            rule_ids::RUST_MUTATION_REQUIRED,
            "Rust mutation check required",
            ".github/workflows",
            "Rust projects must declare mutation testing",
            "Rust projects should declare mutation testing, normally through cargo-mutants.",
            Some("cargo mutants"),
        ),
        rust_definition(
            rule_ids::RUST_UNSAFE_POLICY_REQUIRED,
            "Rust unsafe policy required",
            "slophammer.yml",
            "Rust projects must declare and respect an unsafe-code policy",
            "Rust projects should declare and respect an unsafe-code policy in slophammer.yml.",
            None,
        ),
        rust_definition(
            rule_ids::RUST_DEPENDENCY_AUDIT_REQUIRED,
            "Rust dependency audit required",
            ".github/workflows",
            "Rust projects must declare dependency audit checks",
            "Rust projects should declare cargo audit or cargo deny.",
            Some("cargo audit/cargo deny"),
        ),
        rust_definition(
            rule_ids::RUST_DEPENDENCY_BOUNDARIES_REQUIRED,
            "Rust dependency boundaries required",
            "slophammer.yml",
            "Rust projects must respect configured dependency boundaries",
            "Rust projects should declare dependency boundaries in slophammer.yml and keep local dependencies inside them.",
            None,
        ),
        rust_definition(
            rule_ids::RUST_SCOPE_INCOMPLETE,
            "Rust scope completeness",
            "slophammer.yml",
            "Configured Rust scope must cover all production files or exclude them with reasons",
            "Every production Rust file must be in configured scope or covered by a conventional or reasoned exclude, so findings cannot be hidden by narrowing scope.",
            None,
        ),
        rust_definition(
            rule_ids::RUST_SUPPRESSIONS_JUSTIFIED,
            "Rust suppressions justified",
            ".",
            "allow attributes in production Rust code must carry a reason",
            "An #[allow(...)] attribute in production scope must carry an adjacent // reason comment or use #[expect(..., reason = \"...\")]; bare suppressions accumulate silently.",
            None,
        ),
    ]
}

pub fn definition(rule_id: &str) -> &'static RuleDefinition {
    match rule_id {
        rule_ids::README_REQUIRED => &README_REQUIRED,
        rule_ids::AGENTS_REQUIRED => &AGENTS_REQUIRED,
        rule_ids::CI_REQUIRED => &CI_REQUIRED,
        rule_ids::RUST_MANIFEST_REQUIRED => &RUST_MANIFEST_REQUIRED,
        rule_ids::RUST_MSRV_REQUIRED => &RUST_MSRV_REQUIRED,
        rule_ids::RUST_CHECK_REQUIRED => &RUST_CHECK_REQUIRED,
        rule_ids::RUST_FMT_REQUIRED => &RUST_FMT_REQUIRED,
        rule_ids::RUST_CLIPPY_REQUIRED => &RUST_CLIPPY_REQUIRED,
        rule_ids::RUST_TEST_REQUIRED => &RUST_TEST_REQUIRED,
        rule_ids::RUST_COVERAGE_REQUIRED => &RUST_COVERAGE_REQUIRED,
        rule_ids::RUST_COMPLEXITY_REQUIRED => &RUST_COMPLEXITY_REQUIRED,
        rule_ids::RUST_DRY_REQUIRED => &RUST_DRY_REQUIRED,
        rule_ids::RUST_MUTATION_REQUIRED => &RUST_MUTATION_REQUIRED,
        rule_ids::RUST_UNSAFE_POLICY_REQUIRED => &RUST_UNSAFE_POLICY_REQUIRED,
        rule_ids::RUST_DEPENDENCY_AUDIT_REQUIRED => &RUST_DEPENDENCY_AUDIT_REQUIRED,
        rule_ids::RUST_DEPENDENCY_BOUNDARIES_REQUIRED => &RUST_DEPENDENCY_BOUNDARIES_REQUIRED,
        rule_ids::SLOPHAMMER_CI_REQUIRED => &SLOPHAMMER_CI_REQUIRED,
        rule_ids::RUST_SCOPE_INCOMPLETE => &RUST_SCOPE_INCOMPLETE,
        rule_ids::RUST_SUPPRESSIONS_JUSTIFIED => &RUST_SUPPRESSIONS_JUSTIFIED,
        _ => &README_REQUIRED,
    }
}

fn rust_definition(
    id: &'static str,
    title: &'static str,
    path: &'static str,
    message: &'static str,
    description: &'static str,
    tool: Option<&'static str>,
) -> RuleDefinition {
    RuleDefinition {
        id,
        title,
        category: "rust",
        severity: Severity::Error,
        path,
        message,
        description,
        tool,
        status: "implemented",
    }
}

static README_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::README_REQUIRED,
    title: "README required",
    category: "repo",
    severity: Severity::Error,
    path: "README.md",
    message: "README.md is required",
    description: "The target repo should have a README.md.",
    tool: None,
    status: "implemented",
};
static AGENTS_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::AGENTS_REQUIRED,
    title: "Agent instructions required",
    category: "repo",
    severity: Severity::Error,
    path: "AGENTS.md",
    message: "AGENTS.md is required",
    description: "The target repo should have an AGENTS.md.",
    tool: None,
    status: "implemented",
};
static CI_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::CI_REQUIRED,
    title: "CI workflow required",
    category: "repo",
    severity: Severity::Error,
    path: ".github/workflows",
    message: ".github/workflows must contain at least one .yml or .yaml workflow",
    description: "The target repo should have a CI workflow under .github/workflows.",
    tool: None,
    status: "implemented",
};
static RUST_MANIFEST_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_MANIFEST_REQUIRED,
    title: "Rust manifest required",
    category: "rust",
    severity: Severity::Error,
    path: "Cargo.toml",
    message: "Rust projects must include Cargo.toml",
    description: "Rust projects should include a Cargo manifest.",
    tool: Some("cargo"),
    status: "implemented",
};
static RUST_MSRV_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_MSRV_REQUIRED,
    title: "Rust MSRV required",
    category: "rust",
    severity: Severity::Error,
    path: "Cargo.toml",
    message: "Rust projects must declare a minimum supported Rust version",
    description: "Rust projects should declare a minimum supported Rust version.",
    tool: Some("cargo"),
    status: "implemented",
};
static RUST_CHECK_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_CHECK_REQUIRED,
    title: "Rust cargo check required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare cargo check in CI or scripts",
    description: "Rust projects should declare cargo check in an inspectable workflow or script.",
    tool: Some("cargo check"),
    status: "implemented",
};
static RUST_FMT_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_FMT_REQUIRED,
    title: "Rust formatter required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare cargo fmt --check in CI or scripts",
    description: "Rust projects should declare cargo fmt --check in CI or scripts.",
    tool: Some("cargo fmt"),
    status: "implemented",
};
static RUST_CLIPPY_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_CLIPPY_REQUIRED,
    title: "Rust Clippy required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare cargo clippy in CI or scripts",
    description: "Rust projects should declare cargo clippy with warnings denied.",
    tool: Some("cargo clippy"),
    status: "implemented",
};
static RUST_TEST_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_TEST_REQUIRED,
    title: "Rust tests required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare cargo test in CI or scripts",
    description: "Rust projects should declare cargo test in CI or scripts.",
    tool: Some("cargo test"),
    status: "implemented",
};
static RUST_COVERAGE_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_COVERAGE_REQUIRED,
    title: "Rust coverage gate required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare a coverage gate",
    description: "Rust projects should declare an enforceable coverage gate.",
    tool: Some("cargo llvm-cov"),
    status: "implemented",
};
static RUST_COMPLEXITY_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_COMPLEXITY_REQUIRED,
    title: "Rust complexity required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must enforce complexity limits",
    description: "Rust projects should enforce complexity limits through Clippy or Slophammer policy.",
    tool: Some("clippy"),
    status: "implemented",
};
static RUST_DRY_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_DRY_REQUIRED,
    title: "Rust DRY check required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare a DRY check",
    description: "Rust projects should declare Slophammer's native Rust DRY check.",
    tool: Some("slophammer-rs dry"),
    status: "implemented",
};
static RUST_MUTATION_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_MUTATION_REQUIRED,
    title: "Rust mutation check required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare mutation testing",
    description: "Rust projects should declare mutation testing, normally through cargo-mutants. Only executing invocations count: list, scan, check, dry-run, and manifest-only forms cannot fail on a surviving mutant and are not evidence.",
    tool: Some("cargo mutants"),
    status: "implemented",
};
static RUST_UNSAFE_POLICY_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_UNSAFE_POLICY_REQUIRED,
    title: "Rust unsafe policy required",
    category: "rust",
    severity: Severity::Error,
    path: "slophammer.yml",
    message: "Rust projects must declare and respect an unsafe-code policy",
    description: "Rust projects should declare and respect an unsafe-code policy in slophammer.yml.",
    tool: None,
    status: "implemented",
};
static RUST_DEPENDENCY_AUDIT_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_DEPENDENCY_AUDIT_REQUIRED,
    title: "Rust dependency audit required",
    category: "rust",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "Rust projects must declare dependency audit checks",
    description: "Rust projects should declare cargo audit or cargo deny.",
    tool: Some("cargo audit/cargo deny"),
    status: "implemented",
};
static RUST_DEPENDENCY_BOUNDARIES_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_DEPENDENCY_BOUNDARIES_REQUIRED,
    title: "Rust dependency boundaries required",
    category: "rust",
    severity: Severity::Error,
    path: "slophammer.yml",
    message: "Rust projects must respect configured dependency boundaries",
    description: "Rust projects should declare dependency boundaries in slophammer.yml and keep local dependencies inside them.",
    tool: None,
    status: "implemented",
};
static SLOPHAMMER_CI_REQUIRED: RuleDefinition = RuleDefinition {
    id: rule_ids::SLOPHAMMER_CI_REQUIRED,
    title: "Slophammer enforcement required",
    category: "repo",
    severity: Severity::Error,
    path: ".github/workflows",
    message: "CI must run a Slophammer checker when slophammer.yml is present",
    description: "A repository that carries slophammer.yml must execute a Slophammer checker from binding CI evidence; config without enforcement is decoration.",
    tool: None,
    status: "implemented",
};
static RUST_SCOPE_INCOMPLETE: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_SCOPE_INCOMPLETE,
    title: "Rust scope completeness",
    category: "rust",
    severity: Severity::Error,
    path: "slophammer.yml",
    message: "Configured Rust scope must cover all production files or exclude them with reasons",
    description: "Every production Rust file must be in configured scope or covered by a conventional or reasoned exclude, so findings cannot be hidden by narrowing scope.",
    tool: None,
    status: "implemented",
};
static RUST_SUPPRESSIONS_JUSTIFIED: RuleDefinition = RuleDefinition {
    id: rule_ids::RUST_SUPPRESSIONS_JUSTIFIED,
    title: "Rust suppressions justified",
    category: "rust",
    severity: Severity::Error,
    path: ".",
    message: "allow attributes in production Rust code must carry a reason",
    description: "An #[allow(...)] attribute in production scope must carry an adjacent // reason comment or use #[expect(..., reason = \"...\")]; bare suppressions accumulate silently.",
    tool: None,
    status: "implemented",
};
