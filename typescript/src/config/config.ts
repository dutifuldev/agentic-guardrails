import YAML from "yaml";

import type { Snapshot } from "../repo/repo.js";
import type { Severity } from "../rules/types.js";

export const minimumCoverageThreshold = 85;
export const maximumComplexity = 8;

export type RuleConfig = {
  readonly severity?: Severity | undefined;
  readonly disabled?: boolean | undefined;
  readonly reason?: string | undefined;
  readonly threshold?: number | undefined;
  readonly max?: number | undefined;
};

export type DependencyBoundary = {
  readonly from: string;
  readonly allow: readonly string[];
};

export type DryCopiedConfig = {
  readonly enabled: boolean;
  readonly enabledSet: boolean;
  readonly minTokens: number;
};

export type DryConfig = {
  readonly maxFindings: number;
  readonly maxFindingsSet: boolean;
  readonly paths: readonly string[];
  readonly exclude: readonly string[];
  readonly copiedBlocks: DryCopiedConfig;
};

export type CoverageConfig = {
  readonly threshold: number;
  readonly paths: readonly string[];
  readonly exclude: readonly string[];
};

export type TypeScriptConfig = {
  readonly coverage: CoverageConfig;
  readonly complexityMax: number;
  readonly dry: DryConfig;
  readonly mutationTargets: readonly string[];
  readonly dependencyBoundaries: readonly DependencyBoundary[];
};

export type Config = {
  readonly rules: ReadonlyMap<string, RuleConfig>;
  readonly typescript: TypeScriptConfig;
};

export function emptyConfig(): Config {
  return {
    rules: new Map(),
    typescript: {
      coverage: { threshold: 0, paths: [], exclude: [] },
      complexityMax: 0,
      dry: {
        maxFindings: 0,
        maxFindingsSet: false,
        paths: [],
        exclude: [],
        copiedBlocks: { enabled: false, enabledSet: false, minTokens: 0 }
      },
      mutationTargets: [],
      dependencyBoundaries: []
    }
  };
}

export function loadConfig(snapshot: Snapshot): Config {
  const file = snapshot.files.get("slophammer.yml") ?? snapshot.files.get("slophammer.yaml");
  if (file === undefined) {
    return emptyConfig();
  }
  const parsed: unknown = YAML.parse(file.content);
  const cfg = parseConfig(asConfigRoot(parsed, file.path));
  validateConfig(cfg, file.path);
  return cfg;
}

function parseConfig(root: Readonly<Record<string, unknown>>): Config {
  assertKnownKeys(root, "root", ["rules", "go", "typescript", "rust", "python"]);
  validateIgnoredGoConfig(root["go"]);
  validateIgnoredRustConfig(root["rust"]);
  validateIgnoredPythonConfig(root["python"]);
  return {
    rules: parseRules(asSection(root["rules"], "rules")),
    typescript: parseTypeScriptConfig(asSection(root["typescript"], "typescript"))
  };
}

function parseRules(root: Readonly<Record<string, unknown>>): ReadonlyMap<string, RuleConfig> {
  const rules = new Map<string, RuleConfig>();
  for (const [ruleID, value] of Object.entries(root)) {
    const raw = asSection(value, `rules.${ruleID}`);
    assertKnownKeys(raw, `rules.${ruleID}`, ["severity", "disabled", "reason", "threshold", "max"]);
    const severity = parseSeverity(ruleID, raw["severity"]);
    rules.set(ruleID, {
      severity,
      disabled: asBoolean(raw["disabled"], `rules.${ruleID}.disabled`),
      reason: asString(raw["reason"]),
      threshold: optionalNumber(raw["threshold"], `rules.${ruleID}.threshold`),
      max: optionalNumber(raw["max"], `rules.${ruleID}.max`)
    });
  }
  return rules;
}

function parseTypeScriptConfig(root: Readonly<Record<string, unknown>>): TypeScriptConfig {
  assertKnownKeys(root, "typescript", [
    "coverage",
    "complexity",
    "dry",
    "mutation",
    "dependency_boundaries"
  ]);
  const coverage = asSection(root["coverage"], "typescript.coverage");
  assertKnownKeys(coverage, "typescript.coverage", ["threshold", "paths", "exclude"]);
  const complexity = asSection(root["complexity"], "typescript.complexity");
  assertKnownKeys(complexity, "typescript.complexity", ["max"]);
  const mutation = asSection(root["mutation"], "typescript.mutation");
  assertKnownKeys(mutation, "typescript.mutation", ["targets"]);
  const dry = asSection(root["dry"], "typescript.dry");
  assertKnownKeys(dry, "typescript.dry", ["max_findings", "paths", "exclude", "copied_blocks"]);
  const copiedBlocks = asSection(dry["copied_blocks"], "typescript.dry.copied_blocks");
  assertKnownKeys(copiedBlocks, "typescript.dry.copied_blocks", ["enabled", "min_tokens"]);
  return {
    coverage: {
      threshold: optionalNumber(coverage["threshold"], "typescript.coverage.threshold"),
      paths: optionalStringArray(coverage["paths"], "typescript.coverage.paths"),
      exclude: excludePatterns(coverage["exclude"], "typescript.coverage")
    },
    complexityMax: optionalNumber(complexity["max"], "typescript.complexity.max"),
    dry: {
      maxFindings: optionalNumber(dry["max_findings"], "typescript.dry.max_findings"),
      maxFindingsSet: dry["max_findings"] !== undefined,
      paths: optionalStringArray(dry["paths"], "typescript.dry.paths"),
      exclude: excludePatterns(dry["exclude"], "typescript.dry"),
      copiedBlocks: {
        enabled: optionalBoolean(copiedBlocks["enabled"], "typescript.dry.copied_blocks.enabled"),
        enabledSet: copiedBlocks["enabled"] !== undefined,
        minTokens: optionalNumber(
          copiedBlocks["min_tokens"],
          "typescript.dry.copied_blocks.min_tokens"
        )
      }
    },
    mutationTargets: optionalStringArray(mutation["targets"], "typescript.mutation.targets"),
    dependencyBoundaries: asDependencyBoundaries(root["dependency_boundaries"])
  };
}

// An exclude entry is a plain pattern when it matches the conventional
// non-production list, and a pattern with a reason when it carves out
// production files.
function excludePatterns(value: unknown, section: string): readonly string[] {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error(`${section}.exclude must be a list`);
  }
  return value.map((item, index) => excludeEntryPattern(item, section, index));
}

function excludeEntryPattern(item: unknown, section: string, index: number): string {
  if (typeof item === "string") {
    if (!conventionalExcludePattern(item)) {
      throw new Error(`${section}.exclude requires a reason for production paths`);
    }
    return item;
  }
  const field = `${section}.exclude[${String(index)}]`;
  const entry = asBoundaryRecord(item, field);
  assertKnownKeys(entry, field, ["pattern", "reason"]);
  const pattern = asString(entry["pattern"]);
  if (pattern === undefined) {
    throw new Error(`${field}.pattern must be a string`);
  }
  if ((asString(entry["reason"]) ?? "").trim() === "") {
    throw new Error(`${section}.exclude reasons must not be empty`);
  }
  return pattern;
}

// The conventional non-production list from specs/CONFIG.md: patterns that
// scope may exclude without a reason.
const conventionalExcludeMarkers = [
  "_test.",
  ".test.",
  ".spec.",
  "tests/",
  "fixtures/",
  "templates/",
  "testdata/",
  "dist/",
  "build/",
  "coverage/",
  "target/",
  "node_modules/",
  "vendor/",
  "generated",
  "scripts/"
] as const;

export function conventionalExcludePattern(pattern: string): boolean {
  return conventionalExcludeMarkers.some((marker) => pattern.includes(marker));
}

function validateConfig(cfg: Config, filePath: string): void {
  validateRules(cfg, filePath);
  validateTypeScriptConfig(cfg.typescript, filePath);
}

function validateRules(cfg: Config, filePath: string): void {
  for (const [ruleID, rule] of cfg.rules.entries()) {
    if (rule.disabled === true && (rule.reason ?? "").trim() === "") {
      throw new Error(`${filePath}: rules.${ruleID}.reason is required when disabled is true`);
    }
  }
}

function validateTypeScriptConfig(cfg: TypeScriptConfig, filePath: string): void {
  validateCoverageThreshold(cfg, filePath);
  validateComplexityMax(cfg, filePath);
  validateDryConfig(cfg, filePath);
  validateBoundaries(cfg, filePath);
}

function validateCoverageThreshold(cfg: TypeScriptConfig, filePath: string): void {
  if (cfg.coverage.threshold > 0 && cfg.coverage.threshold < minimumCoverageThreshold) {
    throw new Error(`${filePath}: typescript.coverage.threshold must be at least 85`);
  }
}

function validateComplexityMax(cfg: TypeScriptConfig, filePath: string): void {
  if (cfg.complexityMax > 0 && cfg.complexityMax > maximumComplexity) {
    throw new Error(`${filePath}: typescript.complexity.max must be at most 8`);
  }
}

function validateDryConfig(cfg: TypeScriptConfig, filePath: string): void {
  if (cfg.dry.maxFindingsSet) {
    validateNonNegativeInteger(cfg.dry.maxFindings, `${filePath}: typescript.dry.max_findings`);
  }
  validateNonNegativeInteger(
    cfg.dry.copiedBlocks.minTokens,
    `${filePath}: typescript.dry.copied_blocks.min_tokens`
  );
}

function validateBoundaries(cfg: TypeScriptConfig, filePath: string): void {
  for (const [index, boundary] of cfg.dependencyBoundaries.entries()) {
    if (boundary.from.trim() === "") {
      throw new Error(
        `${filePath}: typescript.dependency_boundaries[${String(index)}].from is required`
      );
    }
  }
}

function asConfigRoot(value: unknown, filePath: string): Readonly<Record<string, unknown>> {
  if (value === undefined || value === null) {
    return {};
  }
  if (typeof value === "object" && !Array.isArray(value)) {
    return value as Readonly<Record<string, unknown>>;
  }
  throw new Error(`${filePath}: root must be an object`);
}

function asSection(value: unknown, field: string): Readonly<Record<string, unknown>> {
  if (value === undefined || value === null) {
    return {};
  }
  if (typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${field} must be a mapping`);
  }
  return value as Readonly<Record<string, unknown>>;
}

function assertKnownKeys(
  root: Readonly<Record<string, unknown>>,
  field: string,
  allowed: readonly string[]
): void {
  const allowedSet = new Set(allowed);
  for (const key of Object.keys(root)) {
    if (!allowedSet.has(key)) {
      throw new Error(`${field}.${key} is not supported`);
    }
  }
}

function validateIgnoredGoConfig(value: unknown): void {
  const root = asSection(value, "go");
  assertKnownKeys(root, "go", [
    "coverage",
    "targets",
    "exclude",
    "dry",
    "crap",
    "mutation",
    "dependency_boundaries"
  ]);
  assertKnownKeys(asSection(root["coverage"], "go.coverage"), "go.coverage", [
    "threshold",
    "profile"
  ]);
  assertKnownKeys(asSection(root["crap"], "go.crap"), "go.crap", ["max_score"]);
  const mutation = asSection(root["mutation"], "go.mutation");
  assertKnownKeys(mutation, "go.mutation", ["targets", "exclude"]);
  const dry = asSection(root["dry"], "go.dry");
  assertKnownKeys(dry, "go.dry", [
    "max_findings",
    "paths",
    "exclude",
    "structural",
    "copied_blocks"
  ]);
  validateIgnoredExcludeEntries(root["exclude"], "go");
  validateIgnoredExcludeEntries(mutation["exclude"], "go.mutation");
  validateIgnoredExcludeEntries(dry["exclude"], "go.dry");
  assertKnownKeys(asSection(dry["structural"], "go.dry.structural"), "go.dry.structural", [
    "enabled",
    "threshold",
    "min_lines",
    "min_nodes"
  ]);
  assertKnownKeys(asSection(dry["copied_blocks"], "go.dry.copied_blocks"), "go.dry.copied_blocks", [
    "enabled",
    "min_tokens"
  ]);
  validateIgnoredDependencyBoundaryKeys(root["dependency_boundaries"], "go.dependency_boundaries");
}

function validateIgnoredRustConfig(value: unknown): void {
  const root = asSection(value, "rust");
  assertKnownKeys(root, "rust", [
    "coverage",
    "complexity",
    "targets",
    "exclude",
    "dry",
    "unsafe",
    "mutation",
    "dependency_boundaries"
  ]);
  const coverage = asSection(root["coverage"], "rust.coverage");
  assertKnownKeys(coverage, "rust.coverage", ["threshold", "paths", "exclude"]);
  assertKnownKeys(asSection(root["complexity"], "rust.complexity"), "rust.complexity", [
    "cognitive_max"
  ]);
  const dry = asSection(root["dry"], "rust.dry");
  assertKnownKeys(dry, "rust.dry", ["max_findings", "paths", "exclude", "copied_blocks"]);
  assertKnownKeys(
    asSection(dry["copied_blocks"], "rust.dry.copied_blocks"),
    "rust.dry.copied_blocks",
    ["enabled", "min_tokens"]
  );
  const unsafePolicy = asSection(root["unsafe"], "rust.unsafe");
  assertKnownKeys(unsafePolicy, "rust.unsafe", ["policy", "allow"]);
  validateIgnoredUnsafeAllowKeys(unsafePolicy["allow"], "rust.unsafe.allow");
  const mutation = asSection(root["mutation"], "rust.mutation");
  assertKnownKeys(mutation, "rust.mutation", ["targets", "exclude"]);
  validateIgnoredExcludeEntries(root["exclude"], "rust");
  validateIgnoredExcludeEntries(coverage["exclude"], "rust.coverage");
  validateIgnoredExcludeEntries(dry["exclude"], "rust.dry");
  validateIgnoredExcludeEntries(mutation["exclude"], "rust.mutation");
  validateIgnoredDependencyBoundaryKeys(
    root["dependency_boundaries"],
    "rust.dependency_boundaries"
  );
}

function validateIgnoredPythonConfig(value: unknown): void {
  const root = asSection(value, "python");
  assertKnownKeys(root, "python", [
    "coverage",
    "complexity",
    "dry",
    "mutation",
    "dependency_boundaries",
    "typecheck"
  ]);
  const coverage = asSection(root["coverage"], "python.coverage");
  assertKnownKeys(coverage, "python.coverage", ["threshold", "paths", "exclude"]);
  assertKnownKeys(asSection(root["complexity"], "python.complexity"), "python.complexity", ["max"]);
  const dry = asSection(root["dry"], "python.dry");
  assertKnownKeys(dry, "python.dry", ["max_findings", "paths", "exclude", "copied_blocks"]);
  assertKnownKeys(
    asSection(dry["copied_blocks"], "python.dry.copied_blocks"),
    "python.dry.copied_blocks",
    ["enabled", "min_tokens"]
  );
  const mutation = asSection(root["mutation"], "python.mutation");
  assertKnownKeys(mutation, "python.mutation", ["targets", "exclude"]);
  validateIgnoredExcludeEntries(coverage["exclude"], "python.coverage");
  validateIgnoredExcludeEntries(dry["exclude"], "python.dry");
  validateIgnoredExcludeEntries(mutation["exclude"], "python.mutation");
  validateIgnoredDependencyBoundaryKeys(
    root["dependency_boundaries"],
    "python.dependency_boundaries"
  );
  validateIgnoredTypecheckDemotions(root["typecheck"]);
}

// Reasoned ty rule demotions: each entry names the demoted rule and why the
// demotion is justified. Enforced by the Python checker; shape-checked here.
function validateIgnoredTypecheckDemotions(value: unknown): void {
  const typecheck = asSection(value, "python.typecheck");
  assertKnownKeys(typecheck, "python.typecheck", ["demotions"]);
  const demotions = typecheck["demotions"];
  if (demotions === undefined) {
    return;
  }
  if (!Array.isArray(demotions)) {
    throw new Error("python.typecheck.demotions must be a list");
  }
  for (const [index, item] of demotions.entries()) {
    const field = `python.typecheck.demotions[${String(index)}]`;
    assertKnownKeys(asBoundaryRecord(item, field), field, ["rule", "reason"]);
  }
}

// Exclude entries in cross-language sections are not enforced here, but
// they must keep the shared shape: a plain pattern string or a strict
// pattern-plus-reason object.
function validateIgnoredExcludeEntries(value: unknown, section: string): void {
  if (value === undefined) {
    return;
  }
  if (!Array.isArray(value)) {
    throw new Error(`${section}.exclude must be a list`);
  }
  for (const [index, item] of value.entries()) {
    if (typeof item === "string") {
      continue;
    }
    const field = `${section}.exclude[${String(index)}]`;
    assertKnownKeys(asBoundaryRecord(item, field), field, ["pattern", "reason"]);
  }
}

function validateIgnoredUnsafeAllowKeys(value: unknown, field: string): void {
  if (value === undefined) {
    return;
  }
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be an object array`);
  }
  for (const [index, item] of value.entries()) {
    assertKnownKeys(
      asBoundaryRecord(item, `${field}[${String(index)}]`),
      `${field}[${String(index)}]`,
      ["path", "reason"]
    );
  }
}

function validateIgnoredDependencyBoundaryKeys(value: unknown, field: string): void {
  if (value === undefined) {
    return;
  }
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be an object array`);
  }
  for (const [index, item] of value.entries()) {
    assertKnownKeys(
      asBoundaryRecord(item, `${field}[${String(index)}]`),
      `${field}[${String(index)}]`,
      ["from", "allow"]
    );
  }
}

function asBoundaryRecord(value: unknown, field: string): Readonly<Record<string, unknown>> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${field} must be an object`);
  }
  return value as Readonly<Record<string, unknown>>;
}

function asString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function optionalNumber(value: unknown, field: string): number {
  if (value === undefined) {
    return 0;
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  throw new Error(`${field} must be a number`);
}

function asBoolean(value: unknown, field: string): boolean | undefined {
  if (value === undefined || typeof value === "boolean") {
    return value;
  }
  throw new Error(`${field} must be a boolean`);
}

function optionalBoolean(value: unknown, field: string): boolean {
  if (value === undefined) {
    return false;
  }
  if (typeof value === "boolean") {
    return value;
  }
  throw new Error(`${field} must be a boolean`);
}

function validateNonNegativeInteger(value: number, field: string): void {
  if (!Number.isInteger(value) || value < 0) {
    throw new Error(`${field} must be a non-negative integer`);
  }
}

function parseSeverity(ruleID: string, value: unknown): Severity | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (value === "error" || value === "warn") {
    return value;
  }
  throw new Error(`rules.${ruleID}.severity must be error or warn`);
}

function optionalStringArray(value: unknown, field: string): readonly string[] {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error(`${field} must be a string array`);
  }
  return value.map((item, index) => {
    if (typeof item !== "string") {
      throw new Error(`${field}[${String(index)}] must be a string`);
    }
    return item;
  });
}

function asDependencyBoundaries(value: unknown): readonly DependencyBoundary[] {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error("typescript.dependency_boundaries must be an object array");
  }
  return value.map((item, index) => {
    const field = `typescript.dependency_boundaries[${String(index)}]`;
    if (typeof item !== "object" || item === null || Array.isArray(item)) {
      throw new Error(`${field} must be an object`);
    }
    const entry = item as Readonly<Record<string, unknown>>;
    assertKnownKeys(entry, field, ["from", "allow"]);
    return {
      from: asString(entry["from"]) ?? "",
      allow: optionalStringArray(entry["allow"], `${field}.allow`)
    };
  });
}
