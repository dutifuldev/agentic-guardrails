import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { RulePolicy } from "../src/rules/policy.js";
import {
  executeTypeScriptChecks,
  type CommandResult,
  type Runner
} from "../src/toolchecks/toolchecks.js";

describe("executeTypeScriptChecks filtering", () => {
  it("runs tsgo typecheck scripts", async () => {
    const root = await packageFixture({ typecheck: "tsgo --noEmit" });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(root, runner);

    expect(findings).toEqual([]);
    expect(calls).toEqual(["npm run typecheck"]);
  });

  it("runs only selected execute checks", async () => {
    const root = await packageFixture(requiredScripts());
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(
      root,
      runner,
      root,
      only("ts.typecheck-required")
    );

    expect(findings).toEqual([]);
    expect(calls).toEqual(["npm run typecheck"]);
  });

  it("does not run any disabled execute check", async () => {
    const scripts = {
      ...requiredScripts(),
      complexity: "oxlint --type-aware --deny-warnings .",
      mutate: "stryker run"
    };
    const root = await packageFixture(scripts);
    const calls: string[] = [];
    const runner = recordingRunner(calls);
    const ruleIDs = [
      "ts.format-required",
      "ts.lint-required",
      "ts.typecheck-required",
      "ts.test-required",
      "ts.coverage-required",
      "ts.complexity-required",
      "ts.dry-required",
      "ts.mutation-required"
    ];
    const policy = new RulePolicy(
      new Map(ruleIDs.map((ruleID) => [ruleID, { disabled: true, reason: "checked elsewhere" }])),
      []
    );

    const findings = await executeTypeScriptChecks(root, runner, root, policy);

    expect(findings).toEqual([]);
    expect(calls).toEqual([]);
  });

  it("does not run aggregate scripts when execute checks are filtered", async () => {
    const root = await packageFixture({ check: "eslint . && tsc --noEmit" });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(root, runner, root, only("ts.lint-required"));

    expect(findings).toEqual([]);
    expect(calls).toEqual([]);
  });

  it("runs single-purpose fallback scripts when execute checks are filtered", async () => {
    const root = await packageFixture({ check: "eslint ." });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(root, runner, root, only("ts.lint-required"));

    expect(findings).toEqual([]);
    expect(calls).toEqual(["npm run check"]);
  });

  it("runs selected checks when a single command satisfies related rules", async () => {
    const root = await packageFixture({ lint: "eslint --max-warnings 0 ." });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(root, runner, root, only("ts.lint-required"));

    expect(findings).toEqual([]);
    expect(calls).toEqual(["npm run lint"]);
  });

  it("runs linter-backed complexity checks when execute checks are filtered", async () => {
    const root = await packageFixture({ lint: "oxlint --type-aware --deny-warnings ." });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(
      root,
      runner,
      root,
      only("ts.complexity-required")
    );

    expect(findings).toEqual([]);
    expect(calls).toEqual(["npm run lint"]);
  });

  it("does not run mutating Biome format scripts", async () => {
    const root = await packageFixture({ format: "biome check --write ." });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(root, runner);

    expect(findings).toEqual([]);
    expect(calls).toEqual([]);
  });

  it("runs versioned Slophammer DRY scripts", async () => {
    const root = await packageFixture({ dry: "pnpm dlx slophammer-ts@latest dry ." });
    const calls: string[] = [];
    const runner = recordingRunner(calls);

    const findings = await executeTypeScriptChecks(root, runner);

    expect(findings).toEqual([]);
    expect(calls).toEqual(["npm run dry"]);
  });
});

function only(...ruleIDs: readonly string[]): RulePolicy {
  return new RulePolicy(new Map(), ruleIDs);
}

function recordingRunner(calls: string[]): Runner {
  return {
    run: (_cwd, command, args) => {
      calls.push([command, ...args].join(" "));
      return Promise.resolve(ok());
    }
  };
}

async function packageFixture(scripts: Readonly<Record<string, string>>): Promise<string> {
  const root = await mkdtemp(path.join(tmpdir(), "slophammer-toolchecks-filter-"));
  await mkdir(root, { recursive: true });
  await writeFile(path.join(root, "package.json"), JSON.stringify({ scripts }));
  return root;
}

function requiredScripts(): Readonly<Record<string, string>> {
  return {
    format: "prettier --check .",
    lint: "eslint .",
    typecheck: "tsc --noEmit",
    test: "vitest run",
    coverage: "vitest run --coverage",
    dry: "slophammer-ts dry ."
  };
}

function ok(): CommandResult {
  return { code: 0, stdout: "", stderr: "" };
}
