mod agents;
mod app;
mod baseline;
mod config;
mod core;
mod exec;
mod report;
mod rust_rules;
mod scan;

use crate::app::{AgentsInitOptions, AppResult, CheckOptions, DirectOptions, OutputFormat};
use clap::{Parser, Subcommand, ValueEnum};
use std::io::{self, Write};
use std::process::ExitCode;

#[derive(Parser)]
#[command(name = "slophammer-rs")]
#[command(version)]
#[command(about = "Rust implementation of the Slophammer repository quality checker.")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    Check {
        path: String,
        #[arg(long, value_enum, default_value_t = FormatArg::Text)]
        format: FormatArg,
        #[arg(long)]
        execute: bool,
        #[arg(long = "only")]
        only_rule_ids: Vec<String>,
        #[arg(long, conflicts_with = "baseline_write")]
        baseline: bool,
        #[arg(long = "baseline-write")]
        baseline_write: bool,
    },
    Agents {
        #[command(subcommand)]
        command: AgentsCommand,
    },
    Explain {
        rule_id: String,
    },
    Rules {
        #[arg(long, value_enum, default_value_t = FormatArg::Text)]
        format: FormatArg,
    },
    Dry {
        #[arg(default_value = ".")]
        path: String,
        #[arg(long, value_enum, default_value_t = FormatArg::Text)]
        format: FormatArg,
        #[arg(long = "max-findings")]
        max_findings: Option<usize>,
    },
    Boundaries {
        #[arg(default_value = ".")]
        path: String,
        #[arg(long, value_enum, default_value_t = FormatArg::Text)]
        format: FormatArg,
    },
    Unsafe {
        #[arg(default_value = ".")]
        path: String,
        #[arg(long, value_enum, default_value_t = FormatArg::Text)]
        format: FormatArg,
    },
}

#[derive(Subcommand)]
enum AgentsCommand {
    Init {
        #[arg(default_value = ".")]
        path: String,
        #[arg(long)]
        dry_run: bool,
        #[arg(long)]
        force: bool,
    },
    Check {
        #[arg(default_value = ".")]
        path: String,
        #[arg(long, value_enum, default_value_t = FormatArg::Text)]
        format: FormatArg,
    },
}

#[derive(Clone, Copy, Debug, ValueEnum)]
enum FormatArg {
    Text,
    Json,
    Sarif,
}

fn main() -> ExitCode {
    let cli = Cli::parse();
    exit(run(cli.command))
}

fn run(command: Command) -> AppResult {
    match command {
        Command::Check {
            path,
            format,
            execute,
            only_rule_ids,
            baseline,
            baseline_write,
        } => app::check(CheckOptions {
            root: path,
            format: format.into(),
            execute,
            only_rule_ids,
            baseline: baseline_mode(baseline, baseline_write),
        }),
        Command::Agents { command } => run_agents(command),
        Command::Explain { rule_id } => app::explain(&rule_id),
        Command::Rules { format } => app::rules(format.into()),
        Command::Dry {
            path,
            format,
            max_findings,
        } => app::dry(DirectOptions {
            root: path,
            format: format.into(),
            max_findings,
        }),
        Command::Boundaries { path, format } => app::boundaries(DirectOptions {
            root: path,
            format: format.into(),
            max_findings: None,
        }),
        Command::Unsafe { path, format } => app::unsafe_policy(DirectOptions {
            root: path,
            format: format.into(),
            max_findings: None,
        }),
    }
}

fn run_agents(command: AgentsCommand) -> AppResult {
    match command {
        AgentsCommand::Check { path, format } => app::agents_check(path, format.into()),
        AgentsCommand::Init {
            path,
            dry_run,
            force,
        } => app::agents_init(AgentsInitOptions {
            root: path,
            dry_run,
            force,
        }),
    }
}

fn baseline_mode(baseline: bool, baseline_write: bool) -> baseline::BaselineMode {
    if baseline_write {
        baseline::BaselineMode::Write
    } else if baseline {
        baseline::BaselineMode::Check
    } else {
        baseline::BaselineMode::Off
    }
}

fn exit(result: AppResult) -> ExitCode {
    let mut stdout = io::stdout();
    let mut stderr = io::stderr();
    let _ = stdout.write_all(result.stdout.as_bytes());
    let _ = stderr.write_all(result.stderr.as_bytes());
    ExitCode::from(u8::try_from(result.code).unwrap_or(2))
}

impl From<FormatArg> for OutputFormat {
    fn from(value: FormatArg) -> Self {
        match value {
            FormatArg::Text => Self::Text,
            FormatArg::Json => Self::Json,
            FormatArg::Sarif => Self::Sarif,
        }
    }
}
