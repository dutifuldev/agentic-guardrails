#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const repoFixtures = [
  "clean",
  "missing-agents",
  "missing-ci",
  "missing-readme",
  "unenforced-config",
];
const goFixtures = [
  ...repoFixtures,
  "go-clean",
  "go-bad-dependency",
  "go-bare-suppression",
  "go-carved-scope",
  "go-missing-complexity",
  "go-missing-coverage",
  "go-missing-crap",
  "go-missing-dry",
  "go-missing-lint",
  "go-missing-module",
  "go-missing-mutation",
  "go-missing-tests",
  "go-missing-vet",
  "go-neutralized-ci",
  "go-scan-mutation",
  "go-unreachable-script",
];
const typeScriptFixtures = [
  ...repoFixtures,
  "typescript-clean",
  "typescript-bad-dependency",
  "typescript-bare-suppression",
  "typescript-carved-scope",
  "typescript-duplicate-blocks",
  "typescript-missing-any-rule",
  "typescript-missing-complexity",
  "typescript-missing-coverage",
  "typescript-missing-dry",
  "typescript-missing-format",
  "typescript-missing-lint",
  "typescript-missing-mutation",
  "typescript-missing-package",
  "typescript-missing-strict",
  "typescript-missing-tests",
  "typescript-missing-typecheck",
  "typescript-missing-unsafe-types",
  "typescript-neutralized-ci",
  "typescript-dryrun-mutation",
  "typescript-unreachable-script",
  "adoption-before",
  "adoption-after",
];
const rustFixtures = [
  ...repoFixtures,
  "rust-clean",
  "rust-bad-dependency",
  "rust-bare-suppression",
  "rust-carved-scope",
  "rust-missing-audit",
  "rust-missing-ci",
  "rust-missing-clippy",
  "rust-missing-coverage",
  "rust-missing-dry",
  "rust-missing-fmt",
  "rust-missing-msrv",
  "rust-missing-mutation",
  "rust-missing-tests",
  "rust-unsafe",
  "rust-neutralized-ci",
  "rust-list-mutation",
  "rust-unreachable-script",
];
const pythonFixtures = [
  ...repoFixtures,
  "python-clean",
  "python-bad-dependency",
  "python-bare-suppression",
  "python-carved-scope",
  "python-demoted-rule",
  "python-missing-audit",
  "python-missing-complexity",
  "python-missing-coverage",
  "python-missing-dry",
  "python-missing-format",
  "python-missing-lint",
  "python-missing-mutation",
  "python-missing-project",
  "python-missing-tests",
  "python-missing-typecheck",
  "python-neutralized-ci",
  "python-relative-imports",
  "python-results-mutation",
  "python-soft-warnings",
  "python-unreachable-script",
];
const rustErrorFixtures = ["rust-invalid-config", "rust-unknown-config"];
const agentFixtures = [
  "agents-bun-test",
  "agents-case-duplicate",
  "agents-command-only",
  "agents-crlf-generated",
  "agents-empty",
  "agents-commands-missing",
  "agents-gitignored-package",
  "agents-python-default",
  "agents-reserved-marker",
  "agents-scope-missing",
  "agents-stale",
  "agents-unsafe-path",
  "agents-workspace-lock",
  "agents-workspace-metadata",
];
const agentInitFixtures = ["agents-init", "agents-init-shell-path"];
const baselineFixtures = [
  { fixture: "adoption-baseline", code: 0 },
  { fixture: "adoption-baseline-regression", code: 1 },
  { fixture: "adoption-baseline-stale", code: 2 },
];

run("npm", ["run", "build"], path.join(root, "typescript"), [0]);

for (const fixture of goFixtures) {
  assertFixture({
    implementation: "go",
    fixture,
    command: "go",
    args: [
      "run",
      "./cmd/slophammer-go",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: path.join(root, "go"),
  });
}

for (const fixture of typeScriptFixtures) {
  assertFixture({
    implementation: "typescript",
    fixture,
    command: "node",
    args: [
      "dist/src/cli/main.js",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: path.join(root, "typescript"),
  });
}

for (const fixture of pythonFixtures) {
  assertFixture({
    implementation: "python",
    fixture,
    command: "uv",
    args: [
      "run",
      "--frozen",
      "--directory",
      "python",
      "slophammer-py",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: root,
  });
}

for (const fixture of rustFixtures) {
  assertFixture({
    implementation: "rust",
    fixture,
    command: "cargo",
    args: [
      "run",
      "-q",
      "-p",
      "slophammer-rs",
      "--",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: path.join(root, "rust"),
  });
}

for (const fixture of rustErrorFixtures) {
  run(
    "cargo",
    [
      "run",
      "-q",
      "-p",
      "slophammer-rs",
      "--",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    path.join(root, "rust"),
    [2],
  );
}

for (const fixture of agentFixtures) {
  assertFixture({
    implementation: "go agents",
    fixture,
    command: "go",
    args: [
      "run",
      "./cmd/slophammer-go",
      "agents",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: path.join(root, "go"),
  });
  assertFixture({
    implementation: "typescript agents",
    fixture,
    command: "node",
    args: [
      "dist/src/cli/main.js",
      "agents",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: path.join(root, "typescript"),
  });
  assertFixture({
    implementation: "rust agents",
    fixture,
    command: "cargo",
    args: [
      "run",
      "-q",
      "-p",
      "slophammer-rs",
      "--",
      "agents",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: path.join(root, "rust"),
  });
  assertFixture({
    implementation: "python agents",
    fixture,
    command: "uv",
    args: [
      "run",
      "--frozen",
      "--directory",
      "python",
      "slophammer-py",
      "agents",
      "check",
      fixturePath(fixture),
      "--format",
      "json",
    ],
    cwd: root,
  });
}

for (const fixture of agentInitFixtures) {
  const expectedAgents = readFileSync(
    path.join(root, "fixtures", "expected", `${fixture}.md`),
    "utf8",
  );
  const initFixture = fixturePath(fixture);
  assertOutput(
    `go agents init ${fixture}`,
    run(
      "go",
      ["run", "./cmd/slophammer-go", "agents", "init", initFixture, "--dry-run"],
      path.join(root, "go"),
      [0],
    ).stdout,
    expectedAgents,
  );
  assertOutput(
    `typescript agents init ${fixture}`,
    run(
      "node",
      ["dist/src/cli/main.js", "agents", "init", initFixture, "--dry-run"],
      path.join(root, "typescript"),
      [0],
    ).stdout,
    expectedAgents,
  );
  assertOutput(
    `rust agents init ${fixture}`,
    run(
      "cargo",
      ["run", "-q", "-p", "slophammer-rs", "--", "agents", "init", initFixture, "--dry-run"],
      path.join(root, "rust"),
      [0],
    ).stdout,
    expectedAgents,
  );
  assertOutput(
    `python agents init ${fixture}`,
    run(
      "uv",
      ["run", "--frozen", "--directory", "python", "slophammer-py", "agents", "init", initFixture, "--dry-run"],
      root,
      [0],
    ).stdout,
    expectedAgents,
  );
}

// go run reports every child failure as exit 1, so baseline exit codes need
// a real binary.
const goBinary = path.join(os.tmpdir(), "slophammer-go-conformance");
run("go", ["build", "-o", goBinary, "./cmd/slophammer-go"], path.join(root, "go"), [0]);
for (const { fixture, code } of baselineFixtures) {
  run(goBinary, ["check", fixturePath(fixture), "--baseline"], path.join(root, "go"), [code]);
  run(
    "node",
    ["dist/src/cli/main.js", "check", fixturePath(fixture), "--baseline"],
    path.join(root, "typescript"),
    [code],
  );
  run(
    "cargo",
    [
      "run",
      "-q",
      "-p",
      "slophammer-rs",
      "--",
      "check",
      fixturePath(fixture),
      "--baseline",
    ],
    path.join(root, "rust"),
    [code],
  );
  run(
    "uv",
    [
      "run",
      "--frozen",
      "--directory",
      "python",
      "slophammer-py",
      "check",
      fixturePath(fixture),
      "--baseline",
    ],
    root,
    [code],
  );
}

console.log(
  `Conformance passed: ${String(goFixtures.length)} Go fixtures, ${String(typeScriptFixtures.length)} TypeScript fixtures, ${String(pythonFixtures.length)} Python fixtures, ${String(rustFixtures.length)} Rust fixtures, ${String(agentFixtures.length)} AGENTS.md fixtures, ${String(agentInitFixtures.length)} AGENTS.md init fixtures, ${String(rustErrorFixtures.length)} Rust error fixtures, ${String(baselineFixtures.length)} baseline cases`,
);

function assertFixture({ implementation, fixture, command, args, cwd }) {
  const expected = expectedReport(fixture);
  const expectedCode = expected.ok ? 0 : 1;
  const result = run(command, args, cwd, [expectedCode]);
  const actual = normalizeReport(JSON.parse(result.stdout));
  const normalizedExpected = normalizeReport(expected);
  if (JSON.stringify(actual) !== JSON.stringify(normalizedExpected)) {
    throw new Error(
      `${implementation} fixture ${fixture} report mismatch\nexpected:\n${JSON.stringify(normalizedExpected, null, 2)}\nactual:\n${JSON.stringify(actual, null, 2)}`,
    );
  }
}

function assertOutput(label, actual, expected) {
  if (actual !== expected) {
    throw new Error(`${label} output mismatch\nexpected:\n${expected}\nactual:\n${actual}`);
  }
}

function expectedReport(fixture) {
  return JSON.parse(
    readFileSync(
      path.join(root, "fixtures", "expected", `${fixture}.json`),
      "utf8",
    ),
  );
}

function fixturePath(fixture) {
  return path.join(root, "fixtures", "repos", fixture);
}

function normalizeReport(report) {
  return {
    ok: Boolean(report.ok),
    findings: [...report.findings].sort((left, right) => {
      const leftKey = `${left.rule_id}\0${left.path}\0${left.message}`;
      const rightKey = `${right.rule_id}\0${right.path}\0${right.message}`;
      return leftKey.localeCompare(rightKey);
    }),
  };
}

function run(command, args, cwd, acceptedCodes) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  if (!acceptedCodes.includes(result.status ?? -1)) {
    throw new Error(
      `${command} ${args.join(" ")} failed with ${String(result.status)} in ${cwd}\n${result.stdout}${result.stderr}`,
    );
  }
  return { stdout: result.stdout, stderr: result.stderr };
}
