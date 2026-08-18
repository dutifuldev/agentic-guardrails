import {
  agentsCheck,
  agentsInit,
  boundaries,
  check,
  explain,
  exitError,
  exitOK,
  ruleCatalog,
  typescriptDry,
  type AgentsInitOptions,
  type CheckOptions
} from "../app/app.js";
import type { DryOptions } from "../dry/types.js";

type Result = {
  readonly code: number;
  readonly stdout: string;
  readonly stderr: string;
};

export async function run(args: readonly string[]): Promise<Result> {
  try {
    return await dispatch(args);
  } catch (error) {
    return { code: exitError, stdout: "", stderr: `${errorMessage(error)}\n` };
  }
}

async function dispatch(args: readonly string[]): Promise<Result> {
  const command = args[0];
  if (command === undefined || helpFlag(command)) {
    return { code: exitOK, stdout: usage(), stderr: "" };
  }
  return await dispatchCommand(command, args.slice(1));
}

async function dispatchCommand(command: string, args: readonly string[]): Promise<Result> {
  switch (command) {
    case "check":
      return await check(parseCheckArgs(args));
    case "agents":
      return await runAgents(args);
    case "boundaries":
      return await boundaries(parseBoundaryArgs(args));
    case "explain":
      return runExplain(args);
    case "rules":
      return runRules(args);
    case "dry":
      return await typescriptDry(parseDryArgs(args));
    case "typescript":
      return await runTypeScript(args);
    default:
      return { code: exitError, stdout: "", stderr: `unknown command: ${command}\n${usage()}` };
  }
}

async function runAgents(args: readonly string[]): Promise<Result> {
  const subcommand = args[0];
  if (subcommand === "check") {
    return await agentsCheck(parseAgentsCheckArgs(args.slice(1)));
  }
  if (subcommand === "init") {
    return await agentsInit(parseAgentsInitArgs(args.slice(1)));
  }
  return {
    code: exitError,
    stdout: "",
    stderr: `unknown agents command: ${subcommand ?? ""}\n${agentsUsage()}`
  };
}

function parseAgentsInitArgs(args: readonly string[]): AgentsInitOptions {
  let root = ".";
  let rootSet = false;
  let dryRun = false;
  let force = false;
  for (const argument of args) {
    if (argument === "--dry-run") {
      dryRun = true;
    } else if (argument === "--force") {
      force = true;
    } else if (argument.startsWith("-")) {
      throw new Error(`unknown agents init option: ${argument}`);
    } else {
      if (rootSet) {
        throw new Error("agents init accepts exactly one path");
      }
      root = argument;
      rootSet = true;
    }
  }
  return { root, dryRun, force };
}

function parseAgentsCheckArgs(args: readonly string[]): Pick<CheckOptions, "root" | "format"> {
  let root = ".";
  let rootSet = false;
  let format: CheckOptions["format"] = "text";
  for (let index = 0; index < args.length; index++) {
    const argument = args[index] ?? "";
    if (argument === "--format") {
      format = parseFormat(nextValue(args, index, "--format"));
      index++;
    } else if (argument.startsWith("-")) {
      throw new Error(`unknown agents check option: ${argument}`);
    } else if (rootSet) {
      throw new Error("agents check accepts exactly one path");
    } else {
      root = argument;
      rootSet = true;
    }
  }
  return { root, format };
}

function runRules(args: readonly string[]): Result {
  return ruleCatalog(parseRulesArgs(args));
}

function helpFlag(command: string): boolean {
  return command === "-h" || command === "--help" || command === "help";
}

function runExplain(args: readonly string[]): Result {
  const ruleID = args[0];
  if (ruleID === undefined || args.length !== 1) {
    return { code: exitError, stdout: "", stderr: "usage: slophammer-ts explain <rule-id>\n" };
  }
  return explain(ruleID);
}

function runTypeScript(args: readonly string[]): Promise<Result> | Result {
  const subcommand = args[0];
  if (subcommand === "dry") {
    return typescriptDry(parseDryArgs(args.slice(1)));
  }
  return {
    code: exitError,
    stdout: "",
    stderr: `unknown typescript command: ${subcommand ?? ""}\n${typescriptUsage()}`
  };
}

function parseCheckArgs(args: readonly string[]): CheckOptions {
  let options: CheckOptions = {
    root: "",
    format: "text",
    execute: false,
    onlyRuleIDs: [],
    baseline: "off"
  };
  for (let index = 0; index < args.length; index++) {
    const parsed = parseCheckArg(options, args, index);
    options = parsed.options;
    index += parsed.advance;
  }
  if (options.root === "") {
    throw new Error(
      "usage: slophammer-ts check <path> [--format text|json|sarif] [--execute] [--only rule-id] [--baseline | --baseline-write]"
    );
  }
  return options;
}

function parseBoundaryArgs(args: readonly string[]): Omit<CheckOptions, "onlyRuleIDs"> {
  const parsed = parseCheckArgs(args);
  if (parsed.baseline !== "off") {
    throw new Error("usage: slophammer-ts boundaries <path> [--format text|json|sarif]");
  }
  return { root: parsed.root, format: parsed.format, execute: parsed.execute };
}

function parseRulesArgs(args: readonly string[]): { readonly format: "text" | "json" } {
  let format: "text" | "json" = "text";
  for (let index = 0; index < args.length; index++) {
    const arg = args[index];
    if (arg === "--format") {
      format = parseRulesFormat(nextValue(args, index, "--format"));
      index++;
    } else if (arg === "--json") {
      format = "json";
    } else {
      throw new Error("usage: slophammer-ts rules [--format text|json]");
    }
  }
  return { format };
}

function parseCheckArg(
  options: CheckOptions,
  args: readonly string[],
  index: number
): { readonly options: CheckOptions; readonly advance: number } {
  const arg = args[index];
  if (arg === "--format") {
    return parseFormatOption(options, args, index);
  }
  if (arg?.startsWith("-")) {
    return parseCheckFlag(options, args, index);
  }
  if (arg !== undefined) {
    if (options.root !== "") {
      throw new Error("check accepts exactly one path");
    }
    return { options: { ...options, root: arg }, advance: 0 };
  }
  return { options, advance: 0 };
}

function parseFormatOption(
  options: CheckOptions,
  args: readonly string[],
  index: number
): { readonly options: CheckOptions; readonly advance: number } {
  return {
    options: { ...options, format: parseFormat(nextValue(args, index, "--format")) },
    advance: 1
  };
}

function parseCheckFlag(
  options: CheckOptions,
  args: readonly string[],
  index: number
): { readonly options: CheckOptions; readonly advance: number } {
  const arg = args[index];
  if (arg === "--json") {
    return { options: { ...options, format: "json" }, advance: 0 };
  }
  if (arg === "--execute") {
    return { options: { ...options, execute: true }, advance: 0 };
  }
  if (arg === "--only") {
    return {
      options: {
        ...options,
        onlyRuleIDs: [
          ...(options.onlyRuleIDs ?? []),
          ...parseOnlyRuleIDs(nextValue(args, index, "--only"))
        ]
      },
      advance: 1
    };
  }
  return { options: parseBaselineFlag(options, arg), advance: 0 };
}

const baselineFlagModes: Readonly<Record<string, "check" | "write">> = {
  "--baseline": "check",
  "--baseline-write": "write"
};

function parseBaselineFlag(options: CheckOptions, arg: string | undefined): CheckOptions {
  const mode = arg === undefined ? undefined : baselineFlagModes[arg];
  if (mode === undefined) {
    throw new Error(`unknown check option: ${arg ?? ""}`);
  }
  if (options.baseline !== "off" && options.baseline !== mode) {
    throw new Error("--baseline and --baseline-write are mutually exclusive");
  }
  return { ...options, baseline: mode };
}

function parseOnlyRuleIDs(value: string): readonly string[] {
  const ruleIDs = value
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
  if (ruleIDs.length === 0) {
    throw new Error("--only requires a rule id");
  }
  return ruleIDs;
}

function parseDryArgs(args: readonly string[]): DryOptions {
  let options = defaultDryOptions();
  for (let index = 0; index < args.length; index++) {
    const parsed = parseDryArg(options, args, index);
    options = parsed.options;
    index += parsed.advance;
  }
  return options;
}

function parseDryArg(
  options: DryOptions,
  args: readonly string[],
  index: number
): { readonly options: DryOptions; readonly advance: number } {
  const arg = args[index];
  if (arg === "--show-report") {
    return { options: { ...options, showReport: true }, advance: 0 };
  }
  if (arg === "--max-findings") {
    return {
      options: {
        ...options,
        maxFindings: parseNonNegativeInteger(nextValue(args, index, "--max-findings")),
        maxFindingsSet: true
      },
      advance: 1
    };
  }
  if (arg === "--format") {
    return {
      options: { ...options, format: parseDryFormat(nextValue(args, index, "--format")) },
      advance: 1
    };
  }
  if (arg?.startsWith("-")) {
    throw new Error(`unknown typescript dry option: ${arg}`);
  }
  return arg === undefined
    ? { options, advance: 0 }
    : { options: { ...options, root: arg }, advance: 0 };
}

function defaultDryOptions(): DryOptions {
  return {
    root: ".",
    paths: ["."],
    exclude: [
      "test/**",
      "tests/**",
      "**/test/**",
      "**/tests/**",
      "**/*.test.cts",
      "**/*.test.js",
      "**/*.test.jsx",
      "**/*.test.mts",
      "**/*.test.ts",
      "**/*.test.tsx",
      "**/*.spec.cts",
      "**/*.spec.js",
      "**/*.spec.jsx",
      "**/*.spec.mts",
      "**/*.spec.ts",
      "**/*.spec.tsx",
      ".stryker-tmp/**",
      "fixtures/**",
      "dist/**",
      "coverage/**"
    ],
    maxFindings: 0,
    maxFindingsSet: false,
    copiedBlockEnabled: false,
    copiedBlockSet: false,
    copiedBlockTokens: 0,
    showReport: false,
    format: ""
  };
}

function parseFormat(value: string): CheckOptions["format"] {
  if (value === "text" || value === "json" || value === "sarif") {
    return value;
  }
  throw new Error(`unsupported format: ${value}`);
}

function parseDryFormat(value: string): DryOptions["format"] {
  if (value === "text" || value === "json") {
    return value;
  }
  throw new Error(`unsupported typescript dry format: ${value}`);
}

function parseRulesFormat(value: string): "text" | "json" {
  if (value === "text" || value === "json") {
    return value;
  }
  throw new Error(`unsupported rules format: ${value}`);
}

function parseNonNegativeInteger(value: string): number {
  if (!/^(?:0|[1-9]\d*)$/u.test(value)) {
    throw new Error("--max-findings must be a non-negative integer");
  }
  const parsed = Number.parseInt(value, 10);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error("--max-findings must be a non-negative integer");
  }
  return parsed;
}

function nextValue(args: readonly string[], index: number, flag: string): string {
  const value = args[index + 1];
  if (value === undefined) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function usage(): string {
  return `${[
    "usage:",
    "  slophammer-ts check <path> [--format text|json|sarif] [--execute] [--only rule-id] [--baseline | --baseline-write]",
    "  slophammer-ts agents init [path] [--dry-run] [--force]",
    "  slophammer-ts agents check [path] [--format text|json|sarif]",
    "  slophammer-ts boundaries <path> [--format text|json|sarif]",
    "  slophammer-ts explain <rule-id>",
    "  slophammer-ts rules [--format text|json]",
    "  slophammer-ts dry <path>"
  ].join("\n")}\n`;
}

function agentsUsage(): string {
  return `${[
    "usage:",
    "  slophammer-ts agents init [path] [--dry-run] [--force]",
    "  slophammer-ts agents check [path] [--format text|json|sarif]"
  ].join("\n")}\n`;
}

function typescriptUsage(): string {
  return "usage: slophammer-ts dry <path> [--max-findings n] [--show-report] [--format json|text]\n";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
