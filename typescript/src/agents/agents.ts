import type { RepoFile, Snapshot } from "../repo/repo.js";

export type AgentIssue = {
  readonly rule_id: string;
  readonly severity: "error";
  readonly path: string;
  readonly message: string;
};

export const startMarker = "<!-- slophammer:agents:start -->";
export const endMarker = "<!-- slophammer:agents:end -->";
export const agentRuleIDs = [
  "repo.agents-required",
  "repo.agents-empty",
  "repo.agents-commands-required",
  "repo.agents-command-invalid",
  "repo.agents-scope-required",
  "repo.agents-stale"
] as const;

const manifests = new Set(["Cargo.toml", "go.mod", "package.json", "pyproject.toml"]);

type PackageArea = {
  readonly path: string;
  readonly manifests: readonly string[];
  readonly commands: readonly string[];
};

export type AgentEvidence = {
  readonly globalCommands: readonly string[];
  readonly packages: readonly PackageArea[];
  readonly allCommands: readonly string[];
  readonly renderedCommands: readonly string[];
};

export function deriveEvidence(snapshot: Snapshot): AgentEvidence {
  const globalCommands = rootRunnerCommands(snapshot);
  const byPath = new Map<string, { manifests: string[]; commands: string[] }>();
  for (const file of snapshot.files.values()) {
    const name = baseName(file.path);
    if (
      !manifests.has(name) ||
      ignoredManifestPath(file.path) ||
      unsafePackagePath(file.path) ||
      reservedMarkerPackagePath(file.path)
    ) {
      continue;
    }
    const packagePath = directory(file.path);
    const area = byPath.get(packagePath) ?? { manifests: [], commands: [] };
    area.manifests.push(name);
    area.commands.push(...manifestCommands(snapshot, file, packagePath));
    byPath.set(packagePath, area);
  }
  const packages = [...byPath.entries()]
    .sort(([left], [right]) => compareUTF8(left, right))
    .map(([packagePath, values]) => ({
      path: packagePath,
      manifests: unique(values.manifests).sort(compareUTF8),
      commands: unique(values.commands)
    }));
  const packageCommands = packages.flatMap((packageArea) => packageArea.commands);
  const allCommands = unique([
    ...globalCommands,
    ...packageCommands,
    ...packageCommands.map(commandVariant)
  ]);
  const renderedCommands =
    globalCommands.length > 0
      ? globalCommands
      : unique(packages.flatMap((packageArea) => packageArea.commands));
  return { globalCommands, packages, allCommands, renderedCommands };
}

export function hasUnsafePackagePath(snapshot: Snapshot): boolean {
  return hasPackagePath(snapshot, unsafePackagePath);
}

export function hasReservedMarkerPackagePath(snapshot: Snapshot): boolean {
  return hasPackagePath(snapshot, reservedMarkerPackagePath);
}

function hasPackagePath(snapshot: Snapshot, matches: (filePath: string) => boolean): boolean {
  return [...snapshot.files.values()].some(
    (file) =>
      manifests.has(baseName(file.path)) && !ignoredManifestPath(file.path) && matches(file.path)
  );
}

function unsafePackagePath(filePath: string): boolean {
  return /[\r\n]/u.test(directory(filePath));
}

function reservedMarkerPackagePath(filePath: string): boolean {
  const packagePath = directory(filePath);
  return packagePath.includes(startMarker) || packagePath.includes(endMarker);
}

function ignoredManifestPath(filePath: string): boolean {
  const ignored = new Set([
    "fixtures",
    "templates",
    "testdata",
    "tests",
    "test",
    "scripts",
    "vendor",
    "dist",
    "build",
    "target",
    "node_modules",
    ".venv",
    "coverage"
  ]);
  return directory(filePath)
    .split("/")
    .some((segment) => ignored.has(segment) || (segment.startsWith(".") && segment !== "."));
}

function rootRunnerCommands(snapshot: Snapshot): readonly string[] {
  const runners: readonly [string, string, RegExp][] = [
    ["Makefile", "make check", /^check\s*:/mu],
    ["makefile", "make check", /^check\s*:/mu],
    ["Taskfile.yml", "task check", /^\s{0,2}check\s*:/mu],
    ["Taskfile.yaml", "task check", /^\s{0,2}check\s*:/mu],
    ["justfile", "just check", /^check\s*:/mu]
  ];
  return unique(
    runners
      .filter(([name, , pattern]) => pattern.test(snapshot.files.get(name)?.content ?? ""))
      .map(([, command]) => command)
  );
}

function manifestCommands(
  snapshot: Snapshot,
  file: RepoFile,
  packagePath: string
): readonly string[] {
  const name = baseName(file.path);
  if (name === "go.mod") {
    return [scopedCommand(packagePath, "go test ./...")];
  }
  if (name === "Cargo.toml") {
    const command = /^\s*\[workspace\]\s*$/mu.test(file.content)
      ? "cargo test --workspace"
      : "cargo test";
    return [scopedCommand(packagePath, command)];
  }
  if (name === "package.json") {
    return packageJSONCommands(snapshot, file, packagePath);
  }
  if (name === "pyproject.toml") {
    let command = "python -m compileall .";
    if (file.content.toLowerCase().includes("pytest")) {
      command = adjacentFile(snapshot, packagePath, "uv.lock")
        ? "uv run pytest"
        : "python -m pytest";
    }
    return [scopedCommand(packagePath, command)];
  }
  return [];
}

function packageJSONCommands(
  snapshot: Snapshot,
  file: RepoFile,
  packagePath: string
): readonly string[] {
  const scripts = packageScripts(file.content);
  const script = ["check", "test", "build"].find((name) => typeof scripts[name] === "string");
  if (script === undefined) {
    return [];
  }
  const manager = packageManager(snapshot, packagePath);
  const command =
    script === "test" && ["npm", "yarn"].includes(manager)
      ? `${manager} test`
      : `${manager} run ${script}`;
  return [scopedCommand(packagePath, command)];
}

function packageScripts(content: string): Readonly<Record<string, unknown>> {
  const parsed = parseJSONRecord(content);
  if (parsed === undefined || !recordValue(parsed["scripts"])) {
    return {};
  }
  return parsed["scripts"];
}

function parseJSONRecord(content: string): Readonly<Record<string, unknown>> | undefined {
  try {
    const parsed: unknown = JSON.parse(content);
    return recordValue(parsed) ? parsed : undefined;
  } catch {
    return undefined;
  }
}

function recordValue(value: unknown): value is Readonly<Record<string, unknown>> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function packageManager(snapshot: Snapshot, packagePath: string): string {
  for (const directoryPath of ancestorPaths(packagePath)) {
    const declared = declaredPackageManager(snapshot, directoryPath);
    if (declared !== undefined) {
      return declared;
    }
    if (adjacentFile(snapshot, directoryPath, "pnpm-lock.yaml")) {
      return "pnpm";
    }
    if (adjacentFile(snapshot, directoryPath, "yarn.lock")) {
      return "yarn";
    }
    if (
      adjacentFile(snapshot, directoryPath, "bun.lock") ||
      adjacentFile(snapshot, directoryPath, "bun.lockb")
    ) {
      return "bun";
    }
  }
  return "npm";
}

function declaredPackageManager(
  snapshot: Snapshot,
  packagePath: string
): "npm" | "pnpm" | "yarn" | "bun" | undefined {
  const filePath = packagePath === "." ? "package.json" : `${packagePath}/package.json`;
  const file = snapshot.files.get(filePath);
  if (file === undefined) {
    return undefined;
  }
  return supportedPackageManager(parseJSONRecord(file.content)?.["packageManager"]);
}

function supportedPackageManager(value: unknown): "npm" | "pnpm" | "yarn" | "bun" | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  switch (value.split("@", 1)[0] ?? "") {
    case "npm":
      return "npm";
    case "pnpm":
      return "pnpm";
    case "yarn":
      return "yarn";
    case "bun":
      return "bun";
    default:
      return undefined;
  }
}

function ancestorPaths(packagePath: string): readonly string[] {
  const paths: string[] = [];
  for (let current = packagePath; ; current = directory(current)) {
    paths.push(current);
    if (current === ".") {
      return paths;
    }
  }
}

function adjacentFile(snapshot: Snapshot, packagePath: string, name: string): boolean {
  return snapshot.files.has(packagePath === "." ? name : `${packagePath}/${name}`);
}

function commandVariant(command: string): string {
  return command.startsWith("cd '") && command.includes(" && ")
    ? (command.split(" && ", 2)[1] ?? command)
    : command;
}

function scopedCommand(packagePath: string, command: string): string {
  if (packagePath === ".") {
    return command;
  }
  const escaped = packagePath.replaceAll("'", "'\\''");
  return `cd '${escaped}' && ${command}`;
}

export function renderAgents(snapshot: Snapshot): string {
  const evidence = deriveEvidence(snapshot);
  return [
    "# AGENTS.md",
    "",
    "These instructions apply to this repository.",
    "",
    renderEvidenceBlock(evidence),
    "",
    "## Working rules",
    "",
    "- Keep changes small and reviewable.",
    "- Add or update tests when behavior changes.",
    "- Run the repository checks before you finish.",
    "- Do not weaken existing checks to make a change pass.",
    ""
  ].join("\n");
}

export function renderEvidenceBlock(evidence: AgentEvidence): string {
  const lines = [startMarker, "## Repository checks", ""];
  if (evidence.renderedCommands.length > 0) {
    lines.push(
      "Run these commands before you finish:",
      "",
      "```sh",
      ...evidence.renderedCommands,
      "```"
    );
  } else {
    lines.push("No verification command could be derived from repository files.");
  }
  lines.push("", "## Package areas", "");
  if (evidence.packages.length > 0) {
    for (const packageArea of evidence.packages) {
      const names = packageArea.manifests.map((name) => `\`${name}\``).join(", ");
      lines.push(`- \`${packageArea.path}\`: ${names}`);
    }
  } else {
    lines.push("- No package manifest was detected.");
  }
  lines.push(endMarker);
  return lines.join("\n");
}

export function agentsFindings(snapshot: Snapshot): readonly AgentIssue[] {
  const rootFile = rootAgentsFile(snapshot);
  if (rootFile === undefined) {
    return [agentFinding("repo.agents-required", "AGENTS.md", "AGENTS.md is required")];
  }
  const evidence = deriveEvidence(snapshot);
  const findings: AgentIssue[] = [];
  if (missingUsefulInstructions(rootFile.content, evidence)) {
    findings.push(
      agentFinding(
        "repo.agents-empty",
        "AGENTS.md",
        "AGENTS.md must contain useful repository instructions"
      )
    );
  }
  if (
    evidence.allCommands.length > 0 &&
    !containsAnyCommand(rootFile.content, evidence.allCommands)
  ) {
    findings.push(
      agentFinding(
        "repo.agents-commands-required",
        "AGENTS.md",
        "AGENTS.md must name a verification command supported by the repository"
      )
    );
  }
  const managed = managedBlock(rootFile.content);
  if (managed !== undefined) {
    const invalid = managedCommands(managed).filter(
      (command) => !evidence.allCommands.includes(command)
    );
    if (invalid.length > 0) {
      findings.push(
        agentFinding(
          "repo.agents-command-invalid",
          "AGENTS.md",
          `AGENTS.md contains generated commands without repository evidence: ${invalid.join(", ")}`
        )
      );
    }
    if (managedEvidenceStale(rootFile.content, managed, evidence)) {
      findings.push(
        agentFinding(
          "repo.agents-stale",
          "AGENTS.md",
          "The Slophammer AGENTS.md evidence block is stale"
        )
      );
    }
  }
  findings.push(...unsupportedPackageFindings(snapshot), ...scopeFindings(snapshot, evidence));
  return findings;
}

function unsupportedPackageFindings(snapshot: Snapshot): readonly AgentIssue[] {
  const findings: AgentIssue[] = [];
  if (hasUnsafePackagePath(snapshot)) {
    findings.push(
      agentFinding(
        "repo.agents-scope-required",
        "AGENTS.md",
        "Package paths containing newlines cannot be represented safely in AGENTS.md"
      )
    );
  }
  if (hasReservedMarkerPackagePath(snapshot)) {
    findings.push(
      agentFinding(
        "repo.agents-scope-required",
        "AGENTS.md",
        "Package paths containing Slophammer evidence markers cannot be represented safely in AGENTS.md"
      )
    );
  }
  return findings;
}

function missingUsefulInstructions(content: string, evidence: AgentEvidence): boolean {
  return !usefulContent(content) && !containsAnyCommand(content, evidence.allCommands);
}

function managedEvidenceStale(content: string, managed: string, evidence: AgentEvidence): boolean {
  return duplicateManagedMarkers(content) || managed !== renderEvidenceBlock(evidence);
}

function duplicateManagedMarkers(content: string): boolean {
  return countOccurrences(content, startMarker) > 1 || countOccurrences(content, endMarker) > 1;
}

function countOccurrences(content: string, marker: string): number {
  return content.split(marker).length - 1;
}

export function rootAgentsFile(snapshot: Snapshot): RepoFile | undefined {
  return (
    snapshot.files.get("AGENTS.md") ??
    [...snapshot.files.values()].find(
      (file) => !file.path.includes("/") && file.path.toLowerCase() === "agents.md"
    )
  );
}

function usefulContent(content: string): boolean {
  const withoutComments = content.replace(/<!--.*?-->/gsu, "");
  const kept = withoutComments
    .split("\n")
    .filter(
      (line) => line.trim() !== "" && !line.trimStart().startsWith("#") && line.trim() !== "```"
    )
    .join(" ");
  return kept.replace(/[^A-Za-z0-9]+/gu, "").length >= 20;
}

function containsAnyCommand(content: string, commands: readonly string[]): boolean {
  const lines = content.split("\n").map((line) => line.trim());
  return commands.some((command) => content.includes(`\`${command}\``) || lines.includes(command));
}

function managedBlock(content: string): string | undefined {
  const start = content.indexOf(startMarker);
  if (start < 0) {
    return undefined;
  }
  const end = content.indexOf(endMarker, start);
  const block =
    end < 0 ? content.slice(start).trim() : content.slice(start, end + endMarker.length).trim();
  return normalizeLineEndings(block);
}

function normalizeLineEndings(content: string): string {
  return content.replaceAll("\r\n", "\n").replaceAll("\r", "\n");
}

function managedCommands(block: string): readonly string[] {
  const match = /```sh\n(.*?)\n```/su.exec(block);
  return (
    match?.[1]
      ?.split("\n")
      .map((line) => line.trim())
      .filter((line) => line !== "") ?? []
  );
}

function scopeFindings(snapshot: Snapshot, evidence: AgentEvidence): readonly AgentIssue[] {
  return evidence.packages.flatMap((packageArea) => {
    if (packageArea.path === ".") {
      return [];
    }
    const acceptable = unique([
      ...evidence.globalCommands,
      ...packageArea.commands,
      ...packageArea.commands.map(commandVariant)
    ]);
    if (acceptable.length === 0) {
      return [];
    }
    const governing = governingAgentsFile(snapshot, packageArea.path);
    if (governing !== undefined && containsAnyCommand(governing.content, acceptable)) {
      return [];
    }
    const findingPath = packageArea.path === "." ? "AGENTS.md" : `${packageArea.path}/AGENTS.md`;
    return [
      agentFinding(
        "repo.agents-scope-required",
        findingPath,
        "Package instructions must name a verification command for this package"
      )
    ];
  });
}

function governingAgentsFile(snapshot: Snapshot, packagePath: string): RepoFile | undefined {
  const parts = packagePath === "." ? [] : packagePath.split("/");
  for (let length = parts.length; length >= 0; length--) {
    const directoryPath = parts.slice(0, length).join("/");
    const wanted = directoryPath === "" ? "AGENTS.md" : `${directoryPath}/AGENTS.md`;
    const file =
      snapshot.files.get(wanted) ??
      [...snapshot.files.values()].find((item) => item.path.toLowerCase() === wanted.toLowerCase());
    if (file !== undefined) {
      return file;
    }
  }
  return undefined;
}

function agentFinding(ruleID: string, path: string, message: string): AgentIssue {
  return { rule_id: ruleID, severity: "error", path, message };
}

function baseName(filePath: string): string {
  return filePath.split("/").at(-1) ?? filePath;
}

function directory(filePath: string): string {
  const parts = filePath.split("/");
  return parts.length === 1 ? "." : parts.slice(0, -1).join("/");
}

function compareUTF8(left: string, right: string): number {
  return Buffer.compare(Buffer.from(left, "utf8"), Buffer.from(right, "utf8"));
}

function unique(values: readonly string[]): string[] {
  return [...new Set(values)];
}
