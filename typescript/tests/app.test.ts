import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { check, explain } from "../src/app/app.js";
import type { CommandResult, Runner } from "../src/toolchecks/toolchecks.js";
import { expectedReport, fixturePath, normalizeReport, parseReport } from "./helpers.js";

describe("check", () => {
  for (const name of ["clean", "missing-readme", "missing-agents", "missing-ci"]) {
    it(`matches shared fixture ${name}`, async () => {
      const result = await check({ root: fixturePath(name), format: "json", execute: false });

      expect(result.code).toBe(name === "clean" ? 0 : 1);
      expect(normalizeReport(parseReport(result.stdout))).toEqual(
        normalizeReport(await expectedReport(name))
      );
    });
  }

  for (const name of [
    "typescript-clean",
    "typescript-missing-strict",
    "typescript-missing-any-rule",
    "typescript-missing-dry"
  ]) {
    it(`matches TypeScript fixture ${name}`, async () => {
      const result = await check({ root: fixturePath(name), format: "json", execute: false });

      expect(normalizeReport(parseReport(result.stdout))).toEqual(
        normalizeReport(await expectedReport(name))
      );
    });
  }

  it("executes checks for nested TypeScript packages", async () => {
    const root = await nestedTypeScriptRepo();
    const calls: string[] = [];
    const runner: Runner = {
      run: (cwd, _command, args) => {
        calls.push(`${path.relative(root, cwd)}:${args.join(" ")}`);
        return Promise.resolve(args.includes("lint") ? failed("lint failed") : ok());
      }
    };

    const result = await check({ root, format: "json", execute: true }, runner);
    const report = parseReport(result.stdout);

    expect(calls).toContain("pkg:run lint");
    expect(report.findings).toEqual([
      {
        rule_id: "ts.lint-required",
        severity: "warn",
        path: "pkg/package.json",
        message: "ESLint failed: lint failed"
      }
    ]);
  });

  it("returns an infrastructure error when execute mode cannot run tools", async () => {
    const root = await nestedTypeScriptRepo();
    const runner: Runner = {
      run: () =>
        Promise.resolve({
          code: 2,
          stdout: "",
          stderr: "spawn npm ENOENT",
          infrastructureError: true
        })
    };

    const result = await check({ root, format: "json", execute: true }, runner);

    expect(result.code).toBe(2);
    expect(result.stderr).toContain("failed to run npm run format");
  });

  it("does not execute checks for non-TypeScript packages", async () => {
    const root = await javaScriptPackageRepo();
    const calls: string[] = [];
    const runner: Runner = {
      run: (_cwd, command, args) => {
        calls.push([command, ...args].join(" "));
        return Promise.resolve(ok());
      }
    };

    const result = await check({ root, format: "json", execute: true }, runner);

    expect(result.code).toBe(0);
    expect(parseReport(result.stdout).findings).toEqual([]);
    expect(calls).toEqual([]);
  });

  it("does not add missing-script duplicates in execute mode", async () => {
    const root = await tscScriptOnlyRepo();
    const result = await check(
      { root, format: "json", execute: true },
      { run: () => Promise.resolve(ok()) }
    );
    const findings = parseReport(result.stdout).findings;

    expect(findings.some((finding) => finding.message.startsWith("missing npm script"))).toBe(
      false
    );
    expect(findings.filter((finding) => finding.rule_id === "ts.format-required")).toHaveLength(1);
  });

  it("does not execute root package checks from nested TypeScript evidence", async () => {
    const root = await nestedTypeScriptRepoWithJavaScriptRoot();
    const calls: string[] = [];
    const runner: Runner = {
      run: (cwd, command, args) => {
        calls.push(`${path.relative(root, cwd)}:${command} ${args.join(" ")}`);
        return Promise.resolve(ok());
      }
    };

    const result = await check({ root, format: "json", execute: true }, runner);

    expect(result.code).toBe(0);
    expect(parseReport(result.stdout).findings).toEqual([]);
    expect(calls.every((call) => call.startsWith("pkg:"))).toBe(true);
  });
});

describe("explain", () => {
  it("explains known rules", () => {
    const result = explain("repo.readme-required");

    expect(result.code).toBe(0);
    expect(result.stdout).toContain("repo.readme-required");
  });
});

async function nestedTypeScriptRepo(): Promise<string> {
  const root = await mkdtemp(path.join(tmpdir(), "slophammer-nested-ts-"));
  await mkdir(path.join(root, ".github", "workflows"), { recursive: true });
  await mkdir(path.join(root, "pkg", ".github", "workflows"), { recursive: true });
  await mkdir(path.join(root, "pkg", "src"), { recursive: true });
  await writeFile(path.join(root, "README.md"), "# Repo\n");
  await writeFile(
    path.join(root, "AGENTS.md"),
    "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
  );
  await writeFile(path.join(root, ".github", "workflows", "ci.yml"), rootCheckerWorkflow());
  await writeFile(
    path.join(root, "pkg", ".github", "workflows", "ci.yml"),
    nestedPackageWorkflow()
  );
  await writeFile(path.join(root, "slophammer.yml"), nestedConfig());
  await writeFile(
    path.join(root, "pkg", "package.json"),
    JSON.stringify({ scripts: packageScripts() })
  );
  await writeFile(path.join(root, "pkg", "tsconfig.json"), strictTSConfig());
  await writeFile(
    path.join(root, "pkg", "stryker.conf.json"),
    '{"thresholds":{"high":70,"low":50,"break":50}}'
  );
  await writeFile(path.join(root, "pkg", "eslint.config.mjs"), eslintConfig());
  await writeFile(path.join(root, "pkg", "vitest.config.ts"), coverageConfig());
  return root;
}

async function javaScriptPackageRepo(): Promise<string> {
  const root = await mkdtemp(path.join(tmpdir(), "slophammer-js-"));
  await mkdir(path.join(root, ".github", "workflows"), { recursive: true });
  await mkdir(path.join(root, "src"), { recursive: true });
  await mkdir(path.join(root, "tests"), { recursive: true });
  await writeFile(path.join(root, "README.md"), "# Repo\n");
  await writeFile(
    path.join(root, "AGENTS.md"),
    "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
  );
  await writeFile(path.join(root, ".github", "workflows", "ci.yml"), "name: CI\n");
  await writeFile(path.join(root, "src", "index.js"), "export const value = 1;\n");
  await writeFile(path.join(root, "src", "index.d.ts"), "export declare const value: number;\n");
  await writeFile(path.join(root, "tests", "index.test.ts"), "export const testOnly = true;\n");
  await writeFile(path.join(root, "vitest.config.ts"), "export default {};\n");
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify({ scripts: { test: "node --test" } })
  );
  return root;
}

async function tscScriptOnlyRepo(): Promise<string> {
  const root = await mkdtemp(path.join(tmpdir(), "slophammer-tsc-script-"));
  await mkdir(path.join(root, ".github", "workflows"), { recursive: true });
  await writeFile(path.join(root, "README.md"), "# Repo\n");
  await writeFile(
    path.join(root, "AGENTS.md"),
    "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
  );
  await writeFile(path.join(root, ".github", "workflows", "ci.yml"), "name: CI\n");
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify({ scripts: { build: "tsc --noEmit" } })
  );
  return root;
}

async function nestedTypeScriptRepoWithJavaScriptRoot(): Promise<string> {
  const root = await nestedTypeScriptRepo();
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify({ scripts: { test: "node --test" } })
  );
  return root;
}

// rootCheckerWorkflow keeps repo.slophammer-ci-required satisfied for temp
// repos that carry a slophammer.yml.
function rootCheckerWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  slophammer:",
    "    steps:",
    "      - run: npx slophammer-ts check .",
    ""
  ].join("\n");
}

function nestedPackageWorkflow(): string {
  return [
    "name: package ci",
    "on: [push]",
    "jobs:",
    "  check:",
    "    steps:",
    "      - run: npm run format",
    "      - run: npm run lint",
    "      - run: npm run typecheck",
    "      - run: npm test",
    "      - run: npm run coverage",
    "      - run: npm run dry",
    "      - run: npm run mutate",
    ""
  ].join("\n");
}

function nestedConfig(): string {
  return [
    "rules:",
    "  ts.lint-required:",
    "    severity: warn",
    "typescript:",
    "  coverage:",
    "    threshold: 85",
    "  complexity:",
    "    max: 8",
    "  dry:",
    "    max_findings: 0",
    "    copied_blocks:",
    "      enabled: true",
    "      min_tokens: 100",
    "  mutation:",
    "    targets:",
    "      - pkg/src/rules.ts",
    "  dependency_boundaries:",
    "    - from: pkg/src/rules",
    "      allow:",
    "        - pkg/src/repo",
    ""
  ].join("\n");
}

function packageScripts(): Readonly<Record<string, string>> {
  return {
    format: "prettier --check .",
    lint: "eslint .",
    typecheck: "tsc --noEmit",
    test: "vitest run",
    coverage: "vitest run --coverage",
    dry: "slophammer typescript dry .",
    mutate: "stryker run"
  };
}

function strictTSConfig(): string {
  return JSON.stringify({
    compilerOptions: {
      strict: true,
      noImplicitAny: true,
      noImplicitOverride: true,
      noUncheckedIndexedAccess: true,
      exactOptionalPropertyTypes: true,
      noFallthroughCasesInSwitch: true,
      noPropertyAccessFromIndexSignature: true,
      useUnknownInCatchVariables: true,
      noEmitOnError: true
    }
  });
}

function eslintConfig(): string {
  return [
    'export default [{rules:{"@typescript-eslint/no-explicit-any":"error",',
    '"@typescript-eslint/no-unsafe-assignment":"error",',
    '"@typescript-eslint/no-unsafe-call":"error",',
    '"@typescript-eslint/no-unsafe-member-access":"error",',
    '"@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}}];'
  ].join("");
}

function coverageConfig(): string {
  return "export default {test:{coverage:{thresholds:{lines:85,functions:85,branches:85,statements:85}}}};\n";
}

function ok(): CommandResult {
  return { code: 0, stdout: "", stderr: "" };
}

function failed(message: string): CommandResult {
  return { code: 1, stdout: "", stderr: message };
}
