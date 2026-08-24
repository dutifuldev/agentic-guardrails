import path from "node:path";
import YAML from "yaml";

import type { Config } from "../config/config.js";
import { minimumCoverageThreshold } from "../config/config.js";
import { commandFiles, filesNamed, filesWithSuffix, hasFile, type Snapshot } from "../repo/repo.js";
import { dependencyBoundaryFindings } from "./dependency-boundaries.js";
import { defaultDefinitions, ruleIDs } from "./definitions.js";
import { complexityLimit, enforcedRuleValues, stripJavaScriptComments } from "./eslint-values.js";
import {
  hasOxlintComplexityRule,
  hasOxlintRule,
  hasTypeAwareOxlintRule
} from "./oxlint-evidence.js";
import { hasTypeScriptMutationCommand } from "./mutation-evidence.js";
import { expandedPackageScriptSegments, packageScripts } from "./package-scripts.js";
import { RulePolicy } from "./policy.js";
import { scopeSnapshot } from "./project-scope.js";
import { scopeFindings } from "./scope.js";
import { ignoredProjectDataPath, typeScriptSourcePath } from "./source-paths.js";
import { suppressionFindings } from "./suppressions.js";
import { compilerConfig, extendedConfigPaths } from "./typescript-config.js";
import type { Definition, Finding, Metadata, Report } from "./types.js";

export type Rule = {
  readonly metadata: () => Metadata;
  readonly check: (snapshot: Snapshot, cfg: Config) => readonly Finding[];
};

export function defaultRules(): readonly Rule[] {
  return defaultDefinitions.map(ruleFromDefinition);
}

export function explain(ruleID: string): string | undefined {
  const definition = defaultDefinitions.find((item) => item.id === ruleID);
  if (definition === undefined) {
    return undefined;
  }
  return [
    definition.id,
    "",
    definition.title,
    "",
    definition.description,
    "",
    `Default severity: ${definition.severity}`,
    `Path: ${definition.path}`
  ].join("\n");
}

export function runRules(
  snapshot: Snapshot,
  cfg: Config,
  policy: RulePolicy = new RulePolicy(cfg.rules)
): Report {
  const findings = defaultRules()
    .filter((rule) => policy.active(rule.metadata().id))
    .flatMap((rule) => rule.check(snapshot, cfg));
  const admitted = [...policy.admitAll(findings)];
  return {
    ok: admitted.length === 0,
    findings: admitted.sort((left, right) => {
      const byRule = left.rule_id.localeCompare(right.rule_id);
      return byRule === 0 ? left.path.localeCompare(right.path) : byRule;
    })
  };
}

function ruleFromDefinition(definition: Definition): Rule {
  return {
    metadata: () => ({
      id: definition.id,
      severity: definition.severity,
      description: definition.description
    }),
    check: (snapshot, cfg) => checkDefinition(definition, snapshot, cfg)
  };
}

function checkDefinition(
  definition: Definition,
  snapshot: Snapshot,
  cfg: Config
): readonly Finding[] {
  switch (definition.id) {
    case ruleIDs.readmeRequired:
      return hasRootFileNamed(snapshot, "README.md") ? [] : [finding(definition)];
    case ruleIDs.agentsRequired:
      return hasRootFileNamed(snapshot, "AGENTS.md") ? [] : [finding(definition)];
    case ruleIDs.ciRequired:
      return hasWorkflowFile(snapshot) ? [] : [finding(definition)];
    case ruleIDs.slophammerCiRequired:
      return slophammerCiFindings(definition, snapshot);
    default:
      return checkTypeScriptDefinition(definition, snapshot, cfg);
  }
}

function hasWorkflowFile(snapshot: Snapshot): boolean {
  return (
    filesWithSuffix(snapshot, ".yml").some(isWorkflow) ||
    filesWithSuffix(snapshot, ".yaml").some(isWorkflow)
  );
}

// Config without enforcement is decoration: when slophammer.yml is present,
// binding CI evidence must invoke a Slophammer checker.
function slophammerCiFindings(definition: Definition, snapshot: Snapshot): readonly Finding[] {
  const hasConfig = hasFile(snapshot, "slophammer.yml") || hasFile(snapshot, "slophammer.yaml");
  if (!hasConfig || slophammerInvocation(commandText(snapshot))) {
    return [];
  }
  return [finding(definition)];
}

function commandText(snapshot: Snapshot): string {
  return commandFiles(snapshot)
    .map((file) => file.content)
    .join("\n");
}

function slophammerInvocation(evidence: string): boolean {
  if (evidence.includes("uses: osolmaz/slophammer@")) {
    return true;
  }
  return ["slophammer-go", "slophammer-ts", "slophammer-rs", "slophammer-py"].some((binary) =>
    invocationWithCheck(evidence, binary)
  );
}

const checkInvocationWindow = 160;

function invocationWithCheck(evidence: string, binary: string): boolean {
  for (
    let index = evidence.indexOf(binary);
    index >= 0;
    index = evidence.indexOf(binary, index + 1)
  ) {
    if (evidence.slice(index, index + checkInvocationWindow).includes(" check")) {
      return true;
    }
  }
  return false;
}

function checkTypeScriptDefinition(
  definition: Definition,
  snapshot: Snapshot,
  cfg: Config
): readonly Finding[] {
  const check = typeScriptChecks[definition.id];
  if (check === undefined) {
    return [];
  }
  if (fullSnapshotRuleIDs.has(definition.id)) {
    return isTypeScriptProject(snapshot) ? check(definition, snapshot, cfg) : [];
  }
  return typeScriptProjectScopes(snapshot).flatMap((scope) =>
    prefixScopeFindings(check(definition, scope.snapshot, cfg), scope.root)
  );
}

// These rules evaluate once against the full repository snapshot instead of
// per project scope: their paths and config are repository-relative.
const fullSnapshotRuleIDs: ReadonlySet<string> = new Set([
  ruleIDs.tsDependencyBoundariesRequired,
  ruleIDs.tsScopeIncomplete,
  ruleIDs.tsSuppressionsJustified
]);

type TypeScriptCheck = (
  definition: Definition,
  snapshot: Snapshot,
  cfg: Config
) => readonly Finding[];

const typeScriptChecks: Readonly<Record<string, TypeScriptCheck>> = {
  [ruleIDs.tsPackageRequired]: requiredNamedFileCheck("package.json"),
  [ruleIDs.tsStrictRequired]: predicateCheck(hasStrictTypeScriptConfig),
  [ruleIDs.tsNoExplicitAny]: predicateCheck((snapshot) => hasTypeScriptNoExplicitAnyRule(snapshot)),
  [ruleIDs.tsNoUnsafeTypes]: predicateCheck(hasUnsafeTypeRules),
  [ruleIDs.tsComplexityRequired]: (definition, snapshot, cfg) =>
    hasComplexityRule(snapshot, cfg) ? [] : [finding(definition)],
  [ruleIDs.tsTypecheckRequired]: (definition, snapshot) =>
    hasTypeScriptTypecheckCommand(snapshot) ? [] : [finding(definition)],
  [ruleIDs.tsLintRequired]: predicateCheck(hasTypeScriptLintCommand),
  [ruleIDs.tsFormatRequired]: predicateCheck(hasTypeScriptFormatCommand),
  [ruleIDs.tsTestRequired]: predicateCheck(hasTypeScriptTestCommand),
  [ruleIDs.tsCoverageRequired]: (definition, snapshot, cfg) =>
    hasCoverageGate(snapshot, cfg) ? [] : [finding(definition)],
  [ruleIDs.tsDryRequired]: predicateCheck(hasTypeScriptDryCommand),
  [ruleIDs.tsMutationRequired]: predicateCheck(hasTypeScriptMutationCommand),
  [ruleIDs.tsDependencyBoundariesRequired]: dependencyBoundaryFindings,
  [ruleIDs.tsScopeIncomplete]: scopeFindings,
  [ruleIDs.tsSuppressionsJustified]: (definition, snapshot) =>
    suppressionFindings(definition, snapshot)
};

function requiredNamedFileCheck(fileName: string): TypeScriptCheck {
  return (definition, snapshot) =>
    productionFilesNamed(snapshot, fileName).length > 0 ? [] : [finding(definition)];
}

function predicateCheck(predicate: (snapshot: Snapshot) => boolean): TypeScriptCheck {
  return (definition, snapshot) => (predicate(snapshot) ? [] : [finding(definition)]);
}

function finding(definition: Definition, pathOverride?: string, messageOverride?: string): Finding {
  return {
    rule_id: definition.id,
    severity: definition.severity,
    path: pathOverride ?? definition.path,
    message: messageOverride ?? definition.message
  };
}

function isTypeScriptProject(snapshot: Snapshot): boolean {
  return (
    productionFilesNamed(snapshot, "tsconfig.json").length > 0 ||
    [...snapshot.files.keys()].some((filePath) => typeScriptSourcePath(filePath)) ||
    productionFilesNamed(snapshot, "package.json").some(packageHasTypeScriptSignal)
  );
}

type ProjectScope = {
  readonly root: string;
  readonly snapshot: Snapshot;
};

function typeScriptProjectScopes(snapshot: Snapshot): readonly ProjectScope[] {
  const sortedRoots = sortedTypeScriptProjectRoots(snapshot);
  const boundaryRoots = [...new Set([...sortedRoots, ...packageProjectRoots(snapshot)])].sort(
    (left, right) => left.localeCompare(right)
  );
  return sortedRoots
    .map((root) => ({ root, snapshot: scopeSnapshot(snapshot, root, boundaryRoots) }))
    .filter((scope) => isTypeScriptProject(scope.snapshot));
}

function sortedTypeScriptProjectRoots(snapshot: Snapshot): readonly string[] {
  const roots = new Set<string>();
  for (const file of snapshot.files.values()) {
    addTypeScriptProjectRoot(snapshot, roots, file);
  }
  return [...roots].sort((left, right) => left.localeCompare(right));
}

function addTypeScriptProjectRoot(
  snapshot: Snapshot,
  roots: Set<string>,
  file: { readonly path: string; readonly content: string }
): void {
  if (ignoredProjectDataPath(file.path)) {
    return;
  }
  const baseName = path.posix.basename(file.path).toLowerCase();
  if (baseName === "package.json" && packageHasTypeScriptSignal(file)) {
    roots.add(parentDirectory(file.path));
    return;
  }
  if (baseName === "tsconfig.json") {
    addTypeScriptConfigRoot(snapshot, roots, file.path);
    return;
  }
  if (typeScriptSourcePath(file.path)) {
    roots.add(typeScriptSourceRoot(snapshot, file.path));
  }
}

function addTypeScriptConfigRoot(snapshot: Snapshot, roots: Set<string>, filePath: string): void {
  const root = parentDirectory(filePath);
  if (
    root === "." ||
    hasPackageFile(snapshot, root) ||
    nearestPackageRoot(snapshot, root) === undefined
  ) {
    roots.add(root);
  }
}

function packageProjectRoots(snapshot: Snapshot): readonly string[] {
  const roots = new Set<string>();
  for (const file of snapshot.files.values()) {
    if (
      !ignoredProjectDataPath(file.path) &&
      path.posix.basename(file.path).toLowerCase() === "package.json"
    ) {
      roots.add(parentDirectory(file.path));
    }
  }
  return [...roots].sort((left, right) => left.localeCompare(right));
}

function typeScriptSourceRoot(snapshot: Snapshot, filePath: string): string {
  const packageRoot = nearestPackageRoot(snapshot, filePath);
  if (packageRoot !== undefined) {
    return packageRoot;
  }
  const markerRoot = nearestTypeScriptMarkerRoot(snapshot, filePath);
  if (markerRoot !== undefined) {
    return markerRoot;
  }
  const sourceRoot = sourceDirectoryRoot(filePath);
  if (sourceRoot !== undefined) {
    return sourceRoot;
  }
  return parentDirectory(filePath);
}

function nearestPackageRoot(snapshot: Snapshot, filePath: string): string | undefined {
  for (let current = parentDirectory(filePath); ; current = parentDirectory(current)) {
    if (hasPackageFile(snapshot, current)) {
      return current;
    }
    if (current === ".") {
      return undefined;
    }
  }
}

function sourceDirectoryRoot(filePath: string): string | undefined {
  const parts = filePath.split("/");
  const index = parts.lastIndexOf("src");
  if (index < 0) {
    return undefined;
  }
  return index === 0 ? "." : parts.slice(0, index).join("/");
}

function nearestTypeScriptMarkerRoot(snapshot: Snapshot, filePath: string): string | undefined {
  for (let current = parentDirectory(filePath); ; current = parentDirectory(current)) {
    if (hasScopedFile(snapshot, current, "tsconfig.json") || hasPackageFile(snapshot, current)) {
      return current;
    }
    if (current === ".") {
      return undefined;
    }
  }
}

function hasScopedFile(snapshot: Snapshot, root: string, name: string): boolean {
  return snapshot.files.has(scopedPath(root, name));
}

function hasPackageFile(snapshot: Snapshot, root: string): boolean {
  return snapshot.files.has(scopedPath(root, "package.json"));
}

function prefixScopeFindings(findings: readonly Finding[], root: string): readonly Finding[] {
  if (root === ".") {
    return findings;
  }
  return findings.map((item) => ({ ...item, path: scopedPath(root, item.path) }));
}

function scopedPath(root: string, filePath: string): string {
  return root === "." ? filePath : `${root}/${filePath}`;
}

function parentDirectory(filePath: string): string {
  const directory = path.posix.dirname(filePath);
  return directory === "" ? "." : directory;
}

function packageHasTypeScriptSignal(file: { readonly content: string }): boolean {
  try {
    const root = asRecord(JSON.parse(file.content) as unknown);
    return (
      packageScriptsHaveTypeScriptSignal(root) || packageDependenciesHaveTypeScriptSignal(root)
    );
  } catch {
    return false;
  }
}

function packageScriptsHaveTypeScriptSignal(root: Readonly<Record<string, unknown>>): boolean {
  const scripts = asRecord(root["scripts"]);
  const content = normalizeCommandContent(
    Object.entries(scripts)
      .filter((entry): entry is [string, string] => typeof entry[1] === "string")
      .map(([name, value]) => `${name}: ${value}`)
      .join("\n")
  );
  return content.includes("typecheck") || /\b(?:tsc|tsgo)\b/u.test(content);
}

function packageDependenciesHaveTypeScriptSignal(root: Readonly<Record<string, unknown>>): boolean {
  const dependencyNames = [
    ...Object.keys(asRecord(root["dependencies"])),
    ...Object.keys(asRecord(root["devDependencies"])),
    ...Object.keys(asRecord(root["peerDependencies"])),
    ...Object.keys(asRecord(root["optionalDependencies"]))
  ];
  return dependencyNames.some((name) => typeScriptDependencyName(name));
}

function typeScriptDependencyName(name: string): boolean {
  return (
    name === "typescript" ||
    name === "@typescript/native-preview" ||
    name === "ts-node" ||
    name === "tsx" ||
    name === "ts-jest" ||
    name === "typescript-eslint" ||
    name.startsWith("@typescript-eslint/")
  );
}

function isWorkflow(file: { readonly path: string }): boolean {
  return file.path.split("/").length === 3 && file.path.startsWith(".github/workflows/");
}

function hasRootFileNamed(snapshot: Snapshot, name: string): boolean {
  return [...snapshot.files.keys()].some(
    (filePath) => !filePath.includes("/") && filePath.toLowerCase() === name.toLowerCase()
  );
}

function hasStrictTypeScriptConfig(snapshot: Snapshot): boolean {
  const files = productionTypeScriptConfigFiles(snapshot);
  if (files.length === 0) {
    return false;
  }
  return files.every((file) => {
    const options = effectiveCompilerOptions(snapshot, file.path, new Set());
    return options["strict"] === true;
  });
}

function productionTypeScriptConfigFiles(
  snapshot: Snapshot
): readonly { readonly path: string; readonly content: string }[] {
  return productionFilesNamed(snapshot, "tsconfig.json").filter(
    (file) => parentDirectory(file.path) === "."
  );
}

function hasTypeScriptNoExplicitAnyRule(snapshot: Snapshot): boolean {
  return (
    (hasESLintCommand(snapshot) && hasTypeScriptESLintRule(snapshot, "no-explicit-any")) ||
    (hasOxlintCommand(snapshot) && hasOxlintRule(snapshot, "typescript/no-explicit-any"))
  );
}

function hasTypeScriptESLintRule(snapshot: Snapshot, ruleName: string): boolean {
  const warningsFail = hasESLintMaxWarningsZero(snapshot);
  return eslintConfigContent(snapshot).some(
    (content) => enforcedRuleValues(content, ruleName, warningsFail, "typescript-eslint").length > 0
  );
}

function hasUnsafeTypeRules(snapshot: Snapshot): boolean {
  const warningsFail = hasESLintMaxWarningsZero(snapshot);
  const rules = [
    "no-unsafe-assignment",
    "no-unsafe-call",
    "no-unsafe-member-access",
    "no-unsafe-return"
  ];
  return (
    (hasESLintCommand(snapshot) &&
      eslintConfigContent(snapshot).some((content) =>
        rules.every(
          (rule) => enforcedRuleValues(content, rule, warningsFail, "typescript-eslint").length > 0
        )
      )) ||
    rules.every((rule) => hasTypeAwareOxlintRule(snapshot, `typescript/${rule}`))
  );
}

function hasComplexityRule(snapshot: Snapshot, cfg: Config): boolean {
  const warningsFail = hasESLintMaxWarningsZero(snapshot);
  const maximum = cfg.typescript.complexityMax > 0 ? cfg.typescript.complexityMax : 8;
  return (
    (hasESLintCommand(snapshot) &&
      eslintConfigContent(snapshot).some((content) =>
        enforcedRuleValues(content, "complexity", warningsFail, "core").some((value) =>
          complexityLimit(value, maximum)
        )
      )) ||
    (hasOxlintCommand(snapshot) && hasOxlintComplexityRule(snapshot, maximum))
  );
}

function eslintConfigContent(snapshot: Snapshot): readonly string[] {
  return [
    ...productionFilesNamed(
      snapshot,
      "eslint.config.js",
      "eslint.config.mjs",
      "eslint.config.cjs",
      ".eslintrc",
      ".eslintrc.js",
      ".eslintrc.cjs",
      ".eslintrc.json",
      ".eslintrc.yml",
      ".eslintrc.yaml"
    ).map((file) => eslintConfigRuleContent(file.path, file.content)),
    ...packageESLintConfigContent(snapshot)
  ];
}

function packageESLintConfigContent(snapshot: Snapshot): readonly string[] {
  return productionFilesNamed(snapshot, "package.json").flatMap((file) => {
    try {
      const config = asRecord(asRecord(JSON.parse(file.content) as unknown)["eslintConfig"]);
      return Object.keys(config).length === 0 ? [] : [JSON.stringify(config)];
    } catch {
      return [];
    }
  });
}

function eslintConfigRuleContent(filePath: string, content: string): string {
  return yamlLikeESLintConfigPath(filePath) ? yamlESLintRuleContent(content) : content;
}

function yamlLikeESLintConfigPath(filePath: string): boolean {
  return filePath.endsWith(".yml") || filePath.endsWith(".yaml") || filePath.endsWith(".eslintrc");
}

function stripHashComments(content: string): string {
  return content
    .split("\n")
    .map((line) => line.split("#", 1)[0] ?? "")
    .join("\n");
}

function yamlESLintRuleContent(content: string): string {
  try {
    const root = asRecord(YAML.parse(quoteReservedYAMLRuleKeys(content)));
    const rules = asRecord(root["rules"]);
    const entries = Object.entries(rules).map(([rule, value]) => {
      return `${JSON.stringify(rule)}: ${JSON.stringify(value)}`;
    });
    return `export default { rules: { ${entries.join(", ")} } };`;
  } catch {
    return stripHashComments(content);
  }
}

function quoteReservedYAMLRuleKeys(content: string): string {
  return content.replace(
    /^(\s*)(@typescript-eslint\/[^\s:]+)(\s*:)/gmu,
    (_match: string, indent: string, rule: string, suffix: string) => {
      return `${indent}${JSON.stringify(rule)}${suffix}`;
    }
  );
}

function hasESLintMaxWarningsZero(snapshot: Snapshot): boolean {
  const scripts = packageScripts(snapshot);
  return commandFiles(snapshot).some((file) =>
    commandSegments(file.content)
      .flatMap((segment) => expandedPackageScriptSegments(segment, scripts))
      .some((segment) => /\beslint\b/u.test(segment) && /--max-warnings(?:=|\s+)0\b/u.test(segment))
  );
}

function hasCoverageGate(snapshot: Snapshot, cfg: Config): boolean {
  const threshold = configuredCoverageThreshold(cfg);
  return (
    hasCoverageCommandThreshold(snapshot, threshold) || hasRunnerCoverageConfig(snapshot, threshold)
  );
}

function configuredCoverageThreshold(cfg: Config): number {
  if (cfg.typescript.coverage.threshold > 0) {
    return cfg.typescript.coverage.threshold;
  }
  return minimumCoverageThreshold;
}

function hasTypeScriptTypecheckCommand(snapshot: Snapshot): boolean {
  return commandSegmentsWithPackageExpansion(snapshot).some((segment) =>
    /\b(?:tsc|tsgo)\b(?=[^;&|]*--noemit(?:\s|$))/u.test(segment)
  );
}

function hasTypeScriptLintCommand(snapshot: Snapshot): boolean {
  return commandSegmentsWithPackageExpansion(snapshot).some(
    (segment) =>
      /\b(?:eslint|oxlint)\b/u.test(segment) ||
      (/\bbiome\s+(?:check|lint)\b/u.test(segment) && !mutatingFormatFlag(segment))
  );
}

function hasESLintCommand(snapshot: Snapshot): boolean {
  return commandSegmentsWithPackageExpansion(snapshot).some((segment) =>
    /\beslint\b/u.test(segment)
  );
}

function hasOxlintCommand(snapshot: Snapshot): boolean {
  return commandSegmentsWithPackageExpansion(snapshot).some((segment) =>
    /\boxlint\b/u.test(segment)
  );
}

function hasTypeScriptFormatCommand(snapshot: Snapshot): boolean {
  return commandSegmentsWithPackageExpansion(snapshot).some((segment) =>
    formatterCheckCommand(segment)
  );
}

function prettierCheckCommand(segment: string): boolean {
  return /\bprettier\b/u.test(segment) && /(?:--check\b|\s-c\b)/u.test(segment);
}

function formatterCheckCommand(segment: string): boolean {
  return (
    prettierCheckCommand(segment) ||
    (/\boxfmt\b/u.test(segment) && /--check\b/u.test(segment)) ||
    (/\bdprint\b/u.test(segment) && /\bcheck\b/u.test(segment)) ||
    (/\bbiome\s+(?:check|format)\b/u.test(segment) && !mutatingFormatFlag(segment))
  );
}

function mutatingFormatFlag(segment: string): boolean {
  return /(?:^|\s)--(?:write|fix|unsafe)(?:\s|$)/u.test(segment);
}

function hasTypeScriptDryCommand(snapshot: Snapshot): boolean {
  return commandSegmentsWithPackageExpansion(snapshot).some(
    (segment) =>
      /\bslophammer-ts(?:@\S+)?\s+dry\b/u.test(segment) ||
      /\bslophammer(?:@\S+)?\s+typescript\s+dry\b/u.test(segment) ||
      /\bdist\/src\/cli\/main\.js\s+(?:typescript\s+)?dry\b/u.test(segment)
  );
}

function commandSegmentsWithPackageExpansion(snapshot: Snapshot): readonly string[] {
  const scripts = packageScripts(snapshot);
  return commandFiles(snapshot).flatMap((file) =>
    commandSegments(file.content).flatMap((segment) =>
      expandedPackageScriptSegments(segment, scripts)
    )
  );
}

function hasCoverageCommandThreshold(snapshot: Snapshot, threshold: number): boolean {
  return coverageCommandSegments(snapshot).some(
    (segment) => coverageCommandSegment(segment) && coverageCommandThresholds(segment, threshold)
  );
}

function coverageCommandSegment(segment: string): boolean {
  return (
    testRunnerCoverageCommandSegment(segment) || /\bnyc\b/u.test(segment) || /\bc8\b/u.test(segment)
  );
}

type TestRunner = "vitest" | "jest";

function hasRunnerCoverageConfig(snapshot: Snapshot, threshold: number): boolean {
  const activeRunners = activeCoverageRunners(snapshot);
  return coverageConfigEvidence(snapshot).some((evidence) => {
    if (!activeRunners.has(evidence.runner)) {
      return false;
    }
    const content = evidence.stripComments
      ? stripJavaScriptComments(evidence.content)
      : evidence.content;
    const normalized = normalizeCommandContent(content);
    return normalized.includes("threshold") && coverageMetricThresholds(normalized, threshold);
  });
}

function activeCoverageRunners(snapshot: Snapshot): ReadonlySet<TestRunner> {
  const runners = new Set<TestRunner>();
  for (const segment of coverageCommandSegments(snapshot)) {
    const runner = coverageRunner(segment);
    if (runner !== undefined && coverageEnabledFlag(segment)) {
      runners.add(runner);
    }
  }
  return runners;
}

function coverageCommandSegments(snapshot: Snapshot): readonly string[] {
  const scripts = packageScripts(snapshot);
  return commandFiles(snapshot).flatMap((file) =>
    commandSegments(file.content).flatMap((segment) =>
      expandedPackageScriptSegments(segment, scripts)
    )
  );
}

function testRunnerCoverageCommandSegment(segment: string): boolean {
  return coverageRunner(segment) !== undefined && coverageEnabledFlag(segment);
}

function coverageRunner(segment: string): TestRunner | undefined {
  if (/\bvitest\b/u.test(segment)) {
    return "vitest";
  }
  if (/\bjest\b/u.test(segment)) {
    return "jest";
  }
  return undefined;
}

function coverageEnabledFlag(segment: string): boolean {
  return /(?:^|\s)--coverage(?:=true)?(?:\s|$)/u.test(segment);
}

function commandSegments(content: string): readonly string[] {
  return content
    .replaceAll("\\\n", " ")
    .split(/\n|&&|;/u)
    .map((segment) => normalizeCommandContent(segment).trim())
    .filter((segment) => !segment.includes("||"))
    .filter((segment) => segment.length > 0);
}

type CoverageEvidence = {
  readonly runner: TestRunner;
  readonly content: string;
  readonly stripComments: boolean;
};

function coverageConfigEvidence(snapshot: Snapshot): readonly CoverageEvidence[] {
  return [
    ...productionFilesNamed(
      snapshot,
      "vitest.config.ts",
      "vitest.config.mts",
      "vitest.config.cts",
      "vitest.config.js",
      "vitest.config.mjs",
      "vitest.config.cjs",
      "vite.config.ts",
      "vite.config.mts",
      "vite.config.cts",
      "vite.config.js",
      "vite.config.mjs",
      "vite.config.cjs"
    ).map((file) => ({ runner: "vitest" as const, content: file.content, stripComments: true })),
    ...productionFilesNamed(
      snapshot,
      "jest.config.ts",
      "jest.config.js",
      "jest.config.mjs",
      "jest.config.cjs"
    ).map((file) => ({ runner: "jest" as const, content: file.content, stripComments: true }))
  ];
}

function coverageCommandThresholds(content: string, threshold: number): boolean {
  if (nycCoverageThresholds(content, threshold)) {
    return true;
  }
  return coverageRunner(content) === "vitest" && vitestCoverageThresholds(content, threshold);
}

function nycCoverageThresholds(content: string, threshold: number): boolean {
  if (!/\b(?:nyc|c8)\b/u.test(content) || !/--check-coverage\b/u.test(content)) {
    return false;
  }
  return ["lines", "functions", "branches", "statements"].every((metric) =>
    matchedNumbers(content, new RegExp(`--${metric}(?:=|\\s+)(\\d+(?:\\.\\d+)?)`, "gu")).some(
      (value) => value >= threshold
    )
  );
}

function vitestCoverageThresholds(content: string, threshold: number): boolean {
  return ["lines", "functions", "branches", "statements"].every((metric) =>
    matchedNumbers(
      content,
      new RegExp(`--(?:coverage\\.)?thresholds?\\.${metric}(?:=|\\s+)(\\d+(?:\\.\\d+)?)`, "gu")
    ).some((value) => value >= threshold)
  );
}

function coverageMetricThresholds(content: string, threshold: number): boolean {
  return ["lines", "functions", "branches", "statements"].every((metric) =>
    matchedNumbers(
      content,
      new RegExp(`\\b["']?${metric}["']?\\s*:\\s*(\\d+(?:\\.\\d+)?)`, "gu")
    ).some((value) => value >= threshold)
  );
}

function matchedNumbers(content: string, pattern: RegExp): readonly number[] {
  return [...content.matchAll(pattern)]
    .map((match) => Number.parseFloat(match[1] ?? ""))
    .filter((value) => Number.isFinite(value));
}

function hasTypeScriptTestCommand(snapshot: Snapshot): boolean {
  return commandFiles(snapshot).some((file) =>
    commandSegments(file.content).some(
      (segment) => !placeholderTestCommand(segment) && directTestCommand(segment)
    )
  );
}

function placeholderTestCommand(content: string): boolean {
  return content.includes("no test specified") || content.includes("missing script: test");
}

function directTestCommand(content: string): boolean {
  if (/--coverage(?:[=\s.]|$)/u.test(content)) {
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

function productionFilesNamed(
  snapshot: Snapshot,
  ...names: readonly string[]
): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return filesNamed(snapshot, ...names).filter((file) => !ignoredProjectDataPath(file.path));
}

function effectiveCompilerOptions(
  snapshot: Snapshot,
  filePath: string,
  seen: ReadonlySet<string>
): Readonly<Record<string, unknown>> {
  if (seen.has(filePath)) {
    return {};
  }
  const file = snapshot.files.get(filePath);
  if (file === undefined) {
    return {};
  }
  const config = compilerConfig(file.path, file.content);
  const inherited = extendedConfigPaths(file.path, config)
    .map((extendedPath) =>
      effectiveCompilerOptions(snapshot, extendedPath, new Set([...seen, filePath]))
    )
    .reduce<Readonly<Record<string, unknown>>>(
      (merged, options) => ({ ...merged, ...options }),
      {}
    );
  return { ...inherited, ...asRecord(config["compilerOptions"]) };
}

function normalizeCommandContent(content: string): string {
  return content.replaceAll("\\\n", " ").replace(/\s+/g, " ").toLowerCase();
}

function asRecord(value: unknown): Readonly<Record<string, unknown>> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return {};
  }
  return value as Readonly<Record<string, unknown>>;
}
