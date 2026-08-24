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
  "reasoned-disabled-rule",
  "unenforced-config",
];
const goFixtures = [
  ...repoFixtures,
  "go-clean",
  "go-disabled-mutation",
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
  "typescript-disabled-mutation",
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
  "rust-disabled-mutation",
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
  "python-disabled-mutation",
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
const sharedErrorFixtures = ["disabled-rule-missing-reason"];
const baselineFixtures = [
  { fixture: "adoption-baseline", code: 0 },
  { fixture: "adoption-baseline-regression", code: 1 },
  { fixture: "adoption-baseline-stale", code: 2 },
  { fixture: "disabled-rule-stale-baseline", code: 2 },
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

const disabledCommands = [
  {
    implementation: "go",
    command: "go",
    prefix: ["run", "./cmd/slophammer-go"],
    cwd: path.join(root, "go"),
  },
  {
    implementation: "typescript",
    command: "node",
    prefix: ["dist/src/cli/main.js"],
    cwd: path.join(root, "typescript"),
  },
  {
    implementation: "python",
    command: "uv",
    prefix: ["run", "--frozen", "--directory", "python", "slophammer-py"],
    cwd: root,
  },
  {
    implementation: "rust",
    command: "cargo",
    prefix: ["run", "-q", "-p", "slophammer-rs", "--"],
    cwd: path.join(root, "rust"),
  },
];
const disabledMatrix = [
  { format: "text", args: [] },
  { format: "json", args: ["--format", "json"] },
  { format: "sarif", args: ["--format", "sarif"] },
  {
    format: "json",
    args: ["--only", "repo.readme-required", "--format", "json"],
  },
  { format: "json", args: ["--execute", "--format", "json"] },
];
for (const checker of disabledCommands) {
  for (const entry of disabledMatrix) {
    const result = run(
      checker.command,
      [
        ...checker.prefix,
        "check",
        fixturePath("reasoned-disabled-rule"),
        ...entry.args,
      ],
      checker.cwd,
      [0],
    );
    assertCleanDisabledOutput(
      checker.implementation,
      entry.format,
      result.stdout,
    );
  }
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

// go run reports every child failure as exit 1, so exact error and baseline
// exit codes need a real binary.
const goBinary = path.join(os.tmpdir(), "slophammer-go-conformance");
run(
  "go",
  ["build", "-o", goBinary, "./cmd/slophammer-go"],
  path.join(root, "go"),
  [0],
);
for (const fixture of sharedErrorFixtures) {
  run(
    goBinary,
    ["check", fixturePath(fixture), "--format", "json"],
    path.join(root, "go"),
    [2],
  );
  run(
    "node",
    ["dist/src/cli/main.js", "check", fixturePath(fixture), "--format", "json"],
    path.join(root, "typescript"),
    [2],
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
      "--format",
      "json",
    ],
    root,
    [2],
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
      "--format",
      "json",
    ],
    path.join(root, "rust"),
    [2],
  );
}
for (const { fixture, code } of baselineFixtures) {
  run(
    goBinary,
    ["check", fixturePath(fixture), "--baseline"],
    path.join(root, "go"),
    [code],
  );
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
  `Conformance passed: ${String(goFixtures.length)} Go fixtures, ${String(typeScriptFixtures.length)} TypeScript fixtures, ${String(pythonFixtures.length)} Python fixtures, ${String(rustFixtures.length)} Rust fixtures, ${String(rustErrorFixtures.length + sharedErrorFixtures.length)} error fixtures, ${String(baselineFixtures.length)} baseline cases`,
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

function assertCleanDisabledOutput(implementation, format, stdout) {
  if (format === "text") {
    if (stdout.includes("repo.readme-required")) {
      throw new Error(
        `${implementation} text output contains a disabled finding`,
      );
    }
    return;
  }
  const parsed = JSON.parse(stdout);
  const findings =
    format === "sarif"
      ? (parsed.runs?.[0]?.results ?? [])
      : (parsed.findings ?? []);
  if (findings.length !== 0) {
    throw new Error(
      `${implementation} ${format} output contains disabled findings`,
    );
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
