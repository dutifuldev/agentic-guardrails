import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";

import { ruleIDs } from "../rules/definitions.js";
import { expandedPackageScriptSegments } from "../rules/package-scripts.js";
import { RulePolicy } from "../rules/policy.js";
import type { Finding } from "../rules/types.js";

export type CommandResult = {
  readonly code: number;
  readonly stdout: string;
  readonly stderr: string;
  readonly infrastructureError?: boolean;
};

export type Runner = {
  readonly run: (cwd: string, command: string, args: readonly string[]) => Promise<CommandResult>;
};

export const execRunner: Runner = {
  run: (cwd, command, args) => runCommand(cwd, command, args)
};

export async function executeTypeScriptChecks(
  root: string,
  runner: Runner,
  workspaceRoot = root,
  policy: RulePolicy = new RulePolicy(new Map())
): Promise<readonly Finding[]> {
  const packageJson = packageJSON(root);
  if (packageJson === undefined) {
    return [];
  }
  const packageManager = detectPackageManager(root, workspaceRoot);
  const scripts = packageJson.scripts;
  const allChecks = scriptChecks();
  const checks = selectedChecks(allChecks, policy);
  const findings: Finding[] = [];
  for (const group of scriptGroups(checks, scripts, packageManager, allChecks, policy)) {
    const result = await runner.run(root, packageManager.command, group.args);
    if (result.infrastructureError === true) {
      throw new Error(
        `failed to run ${packageManager.command} ${group.args.join(" ")}: ${firstOutput(result)}`
      );
    }
    if (result.code === 0) {
      continue;
    }
    findings.push(groupFinding(group.checks, result));
  }
  return findings;
}

function scriptChecks(): readonly ScriptCheck[] {
  return [
    scriptCheck("format", ruleIDs.tsFormatRequired, "formatter check failed", prettierCheck),
    scriptCheck("lint", ruleIDs.tsLintRequired, "ESLint failed", eslintCheck),
    scriptCheck("typecheck", ruleIDs.tsTypecheckRequired, "typecheck failed", typecheckCheck),
    scriptCheck("test", ruleIDs.tsTestRequired, "tests failed", testCheck),
    scriptCheck("coverage", ruleIDs.tsCoverageRequired, "coverage gate failed", coverageCheck),
    optionalScriptCheck(
      "complexity",
      ruleIDs.tsComplexityRequired,
      "complexity check failed",
      complexityCheck
    ),
    scriptCheck("dry", ruleIDs.tsDryRequired, "DRY check failed", dryCheck),
    // The static rule only credits executing mutation runs, so execute
    // mode must run the real script: a forced dry run cannot fail on a
    // surviving mutant.
    optionalScriptCheck(
      "mutate",
      ruleIDs.tsMutationRequired,
      "mutation gate failed",
      executingMutationCheck
    )
  ];
}

function selectedChecks(
  checks: readonly ScriptCheck[],
  policy: RulePolicy
): readonly ScriptCheck[] {
  return checks.filter((check) => policy.active(check.ruleID));
}

type ScriptGroup = {
  readonly args: readonly string[];
  readonly checks: readonly ScriptCheck[];
};

function scriptGroups(
  checks: readonly ScriptCheck[],
  scripts: Readonly<Record<string, string>>,
  packageManager: PackageManager,
  allChecks: readonly ScriptCheck[],
  policy: RulePolicy
): readonly ScriptGroup[] {
  const groups = new Map<string, { args: readonly string[]; checks: ScriptCheck[] }>();
  for (const check of checks) {
    const script = scriptNameForCheck(scripts, check, allChecks, policy);
    if (script === undefined) {
      continue;
    }
    const args = packageManager.args(script, check.extraArgs);
    const cacheKey = [packageManager.command, ...args].join("\0");
    const existing = groups.get(cacheKey);
    if (existing === undefined) {
      groups.set(cacheKey, { args, checks: [check] });
      continue;
    }
    existing.checks.push(check);
  }
  return [...groups.values()];
}

function groupFinding(group: readonly ScriptCheck[], result: CommandResult): Finding {
  if (group.length === 1) {
    const check = firstCheck(group);
    return toolFinding(check.ruleID, `${check.message}: ${firstOutput(result)}`);
  }
  const check = representativeAggregateCheck(group, result);
  return toolFinding(check.ruleID, `aggregate TypeScript check failed: ${firstOutput(result)}`);
}

function representativeAggregateCheck(
  checks: readonly ScriptCheck[],
  result: CommandResult
): ScriptCheck {
  const output = normalizeCommandContent(firstOutput(result));
  return checks.find((check) => outputMentionsCheck(output, check)) ?? firstCheck(checks);
}

function outputMentionsCheck(output: string, check: ScriptCheck): boolean {
  return (
    output.includes(check.script) ||
    output.includes(check.message.toLowerCase()) ||
    check.matches(output)
  );
}

function firstCheck(checks: readonly ScriptCheck[]): ScriptCheck {
  const check = checks[0];
  if (check === undefined) {
    throw new Error("script group must contain at least one check");
  }
  return check;
}

type PackageJSON = {
  readonly scripts: Readonly<Record<string, string>>;
};

type ScriptCheck = {
  readonly script: string;
  readonly ruleID: string;
  readonly message: string;
  readonly optional: boolean;
  readonly matches: (content: string) => boolean;
  readonly extraArgs: readonly string[];
};

type PackageManager = {
  readonly command: string;
  readonly args: (script: string, extraArgs?: readonly string[]) => readonly string[];
};

function scriptCheck(
  script: string,
  ruleID: string,
  message: string,
  matches: (content: string) => boolean
): ScriptCheck {
  return { script, ruleID, message, optional: false, matches, extraArgs: [] };
}

function optionalScriptCheck(
  script: string,
  ruleID: string,
  message: string,
  matches: (content: string) => boolean,
  extraArgs: readonly string[] = []
): ScriptCheck {
  return { script, ruleID, message, optional: true, matches, extraArgs };
}

function scriptNameForCheck(
  scripts: Readonly<Record<string, string>>,
  check: ScriptCheck,
  allChecks: readonly ScriptCheck[],
  policy: RulePolicy
): string | undefined {
  const scriptMap = packageScriptMap(scripts);
  const canonical = scripts[check.script];
  if (canonical !== undefined && scriptMatches(canonical, check, scriptMap, allChecks, policy)) {
    return check.script;
  }
  const match = Object.entries(scripts).find(([, content]) =>
    scriptMatches(content, check, scriptMap, allChecks, policy)
  );
  return match?.[0];
}

function scriptMatches(
  content: string,
  check: ScriptCheck,
  scripts: ReadonlyMap<string, string>,
  allChecks: readonly ScriptCheck[],
  policy: RulePolicy
): boolean {
  const segments = expandedScriptSegments(content, scripts);
  return (
    safeScriptForCheck(segments, check) &&
    filteredScriptForCheck(segments, allChecks, policy) &&
    segments.some((segment) => check.matches(segment))
  );
}

function expandedScriptSegments(
  content: string,
  scripts: ReadonlyMap<string, string>
): readonly string[] {
  return commandSegments(content).flatMap((segment) =>
    expandedPackageScriptSegments(segment, scripts).map(normalizeCommandContent)
  );
}

function safeScriptForCheck(segments: readonly string[], check: ScriptCheck): boolean {
  const containsMutation = segments.some(mutationCheck);
  if (check.ruleID !== ruleIDs.tsMutationRequired) {
    return !containsMutation;
  }
  return containsMutation && segments.every(mutationCheck);
}

function filteredScriptForCheck(
  segments: readonly string[],
  allChecks: readonly ScriptCheck[],
  policy: RulePolicy
): boolean {
  return segments.every((segment) => {
    const segmentChecks = allChecks.filter((check) => check.matches(segment));
    return segmentChecks.length === 0 || segmentChecks.some((check) => policy.active(check.ruleID));
  });
}

function commandSegments(content: string): readonly string[] {
  return normalizeCommandContent(content)
    .split(/\n|&&|;/u)
    .map((segment) => segment.trim())
    .filter((segment) => segment !== "");
}

function packageScriptMap(scripts: Readonly<Record<string, string>>): ReadonlyMap<string, string> {
  return new Map(Object.entries(scripts).map(([name, content]) => [name.toLowerCase(), content]));
}

function prettierCheck(content: string): boolean {
  return (
    (/\bprettier\b/u.test(content) && /(?:--check\b|\s-c\b)/u.test(content)) ||
    (/\boxfmt\b/u.test(content) && /--check\b/u.test(content)) ||
    (/\bdprint\b/u.test(content) && /\bcheck\b/u.test(content)) ||
    biomeFormatCheck(content)
  );
}

function biomeFormatCheck(content: string): boolean {
  return /\bbiome\s+(?:check|format)\b/u.test(content) && !mutatingFormatFlag(content);
}

function mutatingFormatFlag(content: string): boolean {
  return /(?:^|\s)--(?:write|fix|unsafe)(?:\s|$)/u.test(content);
}

function eslintCheck(content: string): boolean {
  return /\b(?:eslint|oxlint)\b/u.test(content) || biomeLintCheck(content);
}

function biomeLintCheck(content: string): boolean {
  return /\bbiome\s+(?:check|lint)\b/u.test(content) && !mutatingFormatFlag(content);
}

function typecheckCheck(content: string): boolean {
  return /\b(?:tsc|tsgo)\b(?=[^;&|]*--noemit(?:\s|$))/u.test(content);
}

function testCheck(content: string): boolean {
  if (placeholderTestCommand(content) || /--coverage(?:[=\s.]|$)/u.test(content)) {
    return false;
  }
  return [
    /\bvitest\b/u,
    /\bjest\b/u,
    /\bmocha\b/u,
    /\bava\b/u,
    /\buvu\b/u,
    /\btap\b/u,
    /\bnode\s+--test\b/u,
    /\btsx\s+--test\b/u,
    /\bplaywright\s+test\b/u
  ].some((pattern) => pattern.test(content));
}

function coverageCheck(content: string): boolean {
  return (
    (/\b(?:vitest|jest)\b/u.test(content) &&
      /(?:^|\s)--coverage(?:=true)?(?:\s|$)/u.test(content)) ||
    /\b(?:nyc|c8)\b/u.test(content)
  );
}

function complexityCheck(content: string): boolean {
  return (
    /\bcomplexity\b/u.test(content) ||
    /\boxlint\b/u.test(content) ||
    /\beslint\b(?=[^;&|]*--max-warnings(?:=|\s+)0\b)/u.test(content) ||
    biomeLintCheck(content)
  );
}

function dryCheck(content: string): boolean {
  return (
    /\bslophammer-ts(?:@\S+)?\s+dry\b/u.test(content) ||
    /\bdist\/src\/cli\/main\.js\s+(?:typescript\s+)?dry\b/u.test(content) ||
    /\bslophammer(?:@\S+)?\s+typescript\s+dry\b/u.test(content)
  );
}

function mutationCheck(content: string): boolean {
  return /\bstryker\b/u.test(content);
}

// Execute mode must select the same evidence the static rule accepts: an
// executing `stryker run`, never a dry-run-only or non-run invocation.
function executingMutationCheck(content: string): boolean {
  return /\bstryker run\b/u.test(content) && !/--dry-?run-?only\b/u.test(content);
}

function placeholderTestCommand(content: string): boolean {
  return content.includes("no test specified") || content.includes("missing script: test");
}

function normalizeCommandContent(content: string): string {
  return content.replaceAll("\\\n", " ").replace(/\s+/gu, " ").toLowerCase();
}

function packageJSON(root: string): PackageJSON | undefined {
  const filePath = path.join(root, "package.json");
  if (!existsSync(filePath)) {
    return undefined;
  }
  return parsePackageJSON(readFileSync(filePath, "utf8"));
}

function parsePackageJSON(content: string): PackageJSON {
  const parsed: unknown = JSON.parse(content);
  const root = asRecord(parsed);
  return { scripts: stringRecord(root["scripts"]) };
}

function detectPackageManager(root: string, workspaceRoot: string): PackageManager {
  for (const dir of packageManagerSearchRoots(root, workspaceRoot)) {
    const lockfiles = ["pnpm-lock.yaml", "yarn.lock", "package-lock.json"].filter((lockfile) =>
      existsSync(path.join(dir, lockfile))
    );
    if (lockfiles.length > 1) {
      throw new Error(`multiple package lockfiles found: ${lockfiles.join(", ")}`);
    }
    const packageManager = packageManagerForLockfile(lockfiles[0]);
    if (packageManager !== undefined) {
      return packageManager;
    }
  }
  return { command: "npm", args: npmStyleRunArgs };
}

function packageManagerForLockfile(lockfile: string | undefined): PackageManager | undefined {
  if (lockfile === "pnpm-lock.yaml") {
    return { command: "pnpm", args: npmStyleRunArgs };
  }
  if (lockfile === "yarn.lock") {
    return { command: "yarn", args: yarnRunArgs };
  }
  if (lockfile === "package-lock.json") {
    return { command: "npm", args: npmStyleRunArgs };
  }
  return undefined;
}

function npmStyleRunArgs(script: string, extraArgs: readonly string[] = []): readonly string[] {
  return extraArgs.length === 0 ? ["run", script] : ["run", script, "--", ...extraArgs];
}

function yarnRunArgs(script: string, extraArgs: readonly string[] = []): readonly string[] {
  return [script, ...extraArgs];
}

function packageManagerSearchRoots(root: string, workspaceRoot: string): readonly string[] {
  const start = path.resolve(root);
  const stop = path.resolve(workspaceRoot);
  const dirs: string[] = [];
  for (let current = start; ; current = path.dirname(current)) {
    dirs.push(current);
    if (current === stop || current === path.dirname(current)) {
      return dirs;
    }
  }
}

function runCommand(cwd: string, command: string, args: readonly string[]): Promise<CommandResult> {
  return new Promise((resolve) => {
    const child = spawn(command, [...args], { cwd, shell: false });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    child.stdout.on("data", (chunk: Buffer) => stdout.push(chunk));
    child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk));
    child.on("close", (code) => {
      resolve({
        code: code ?? 2,
        stdout: Buffer.concat(stdout).toString("utf8"),
        stderr: Buffer.concat(stderr).toString("utf8")
      });
    });
    child.on("error", (error) => {
      resolve({
        code: 2,
        stdout: "",
        stderr: error instanceof Error ? error.message : String(error),
        infrastructureError: true
      });
    });
  });
}

function firstOutput(result: CommandResult): string {
  const output = `${result.stderr}\n${result.stdout}`.trim();
  return output === "" ? "command failed" : (output.split("\n")[0] ?? "command failed");
}

function toolFinding(ruleID: string, message: string): Finding {
  return { rule_id: ruleID, severity: "error", path: "package.json", message };
}

function asRecord(value: unknown): Readonly<Record<string, unknown>> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return {};
  }
  return value as Readonly<Record<string, unknown>>;
}

function stringRecord(value: unknown): Readonly<Record<string, string>> {
  const root = asRecord(value);
  const out: Record<string, string> = {};
  for (const [key, item] of Object.entries(root)) {
    if (typeof item === "string") {
      out[key] = item;
    }
  }
  return out;
}
