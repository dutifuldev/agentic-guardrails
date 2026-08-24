import { mkdir, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { check } from "../src/app/app.js";
import type { Runner } from "../src/toolchecks/toolchecks.js";
import { parseReport } from "./helpers.js";

describe("check execute filters", () => {
  it("does not execute TypeScript tools when --only selects a repo rule", async () => {
    const root = await typeScriptRepoWithoutReadme();
    const calls: string[] = [];
    const runner: Runner = {
      run: (_cwd, command, args) => {
        calls.push([command, ...args].join(" "));
        return Promise.resolve({ code: 2, stdout: "", stderr: "should not run" });
      }
    };

    const result = await check(
      {
        root,
        format: "json",
        execute: true,
        onlyRuleIDs: ["repo.readme-required"]
      },
      runner
    );

    expect(result.code).toBe(1);
    expect(parseReport(result.stdout).findings).toEqual([
      expect.objectContaining({ rule_id: "repo.readme-required" })
    ]);
    expect(calls).toEqual([]);
  });

  it("does not launch a disabled selected tool check", async () => {
    const root = await tsgoOnlyTypeScriptRepo();
    await writeFile(
      path.join(root, "slophammer.yml"),
      [
        "rules:",
        "  ts.typecheck-required:",
        "    disabled: true",
        "    reason: checked by another required job",
        ""
      ].join("\n")
    );
    const calls: string[] = [];
    const runner: Runner = {
      run: (_cwd, command, args) => {
        calls.push([command, ...args].join(" "));
        return Promise.resolve({ code: 2, stdout: "", stderr: "should not run" });
      }
    };

    const result = await check(
      {
        root,
        format: "json",
        execute: true,
        onlyRuleIDs: ["ts.typecheck-required"]
      },
      runner
    );

    expect(result.code).toBe(0);
    expect(parseReport(result.stdout).findings).toEqual([]);
    expect(calls).toEqual([]);
  });

  it("does not launch disabled mutation work", async () => {
    const root = await mutationTypeScriptRepo();
    const calls: string[] = [];
    const runner: Runner = {
      run: (_cwd, command, args) => {
        calls.push([command, ...args].join(" "));
        return Promise.resolve({ code: 2, stdout: "", stderr: "should not run" });
      }
    };

    const result = await check(
      {
        root,
        format: "json",
        execute: true,
        onlyRuleIDs: ["ts.mutation-required"]
      },
      runner
    );

    expect(result.code).toBe(0);
    expect(parseReport(result.stdout).findings).toEqual([]);
    expect(calls).toEqual([]);
  });

  it("executes tsgo-only TypeScript packages", async () => {
    const root = await tsgoOnlyTypeScriptRepo();
    const calls: string[] = [];
    const runner: Runner = {
      run: (_cwd, command, args) => {
        calls.push([command, ...args].join(" "));
        return Promise.resolve({ code: 1, stdout: "typecheck failed", stderr: "" });
      }
    };

    const result = await check(
      {
        root,
        format: "json",
        execute: true,
        onlyRuleIDs: ["ts.typecheck-required"]
      },
      runner
    );

    expect(result.code).toBe(1);
    expect(parseReport(result.stdout).findings).toEqual([
      expect.objectContaining({ rule_id: "ts.typecheck-required" })
    ]);
    expect(calls).toEqual(["npm run typecheck"]);
  });
});

async function typeScriptRepoWithoutReadme(): Promise<string> {
  const root = await mkdtemp(path.join(tmpdir(), "slophammer-only-execute-"));
  await mkdir(path.join(root, ".github", "workflows"), { recursive: true });
  await mkdir(path.join(root, "src"), { recursive: true });
  await writeFile(path.join(root, "AGENTS.md"), "# Agents\n");
  await writeFile(path.join(root, ".github", "workflows", "ci.yml"), "name: CI\n");
  await writeFile(path.join(root, "src", "index.ts"), "export const value = 1;\n");
  await writeFile(path.join(root, "tsconfig.json"), '{"compilerOptions":{"strict":true}}\n');
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify({
      scripts: {
        typecheck: "tsgo --noEmit"
      },
      devDependencies: {
        "@typescript/native-preview": "^7.0.0"
      }
    })
  );
  return root;
}

async function mutationTypeScriptRepo(): Promise<string> {
  const root = await tsgoOnlyTypeScriptRepo();
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify({
      scripts: { mutate: "stryker run" },
      devDependencies: { typescript: "^5.0.0" }
    })
  );
  await writeFile(
    path.join(root, ".github", "workflows", "ci.yml"),
    "name: CI\non: [push]\njobs:\n  check:\n    steps:\n      - run: npm run mutate\n"
  );
  await writeFile(
    path.join(root, "slophammer.yml"),
    [
      "rules:",
      "  ts.mutation-required:",
      "    disabled: true",
      "    reason: checked by another required job",
      "typescript:",
      "  mutation:",
      "    targets:",
      "      - src",
      ""
    ].join("\n")
  );
  return root;
}

async function tsgoOnlyTypeScriptRepo(): Promise<string> {
  const root = await mkdtemp(path.join(tmpdir(), "slophammer-tsgo-execute-"));
  await mkdir(path.join(root, ".github", "workflows"), { recursive: true });
  await writeFile(path.join(root, "README.md"), "# Repo\n");
  await writeFile(path.join(root, "AGENTS.md"), "# Agents\n");
  await writeFile(
    path.join(root, ".github", "workflows", "ci.yml"),
    "name: CI\non: [push]\njobs:\n  check:\n    steps:\n      - run: npm run typecheck\n"
  );
  await writeFile(
    path.join(root, "package.json"),
    JSON.stringify({
      scripts: {
        typecheck: "tsgo --noEmit"
      },
      devDependencies: {
        "@typescript/native-preview": "^7.0.0"
      }
    })
  );
  return root;
}
