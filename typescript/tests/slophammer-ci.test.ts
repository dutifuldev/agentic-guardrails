import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot, type RepoFile, type Snapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";

const ruleID = "repo.slophammer-ci-required";

describe("repo.slophammer-ci-required", () => {
  it("is silent without a slophammer config", () => {
    const report = run(snapshot(workflow("npm test")));

    expect(report.findings).toEqual([]);
  });

  it("fires when config exists but CI never runs a checker", () => {
    const report = run(snapshot(workflow("npm test"), config()));

    expect(report.findings).toEqual([
      expect.objectContaining({ rule_id: ruleID, path: ".github/workflows" })
    ]);
  });

  it("accepts the published action", () => {
    const content = [
      "name: CI",
      "on: [push]",
      "jobs:",
      "  gate:",
      "    steps:",
      "      - uses: osolmaz/slophammer@v1",
      ""
    ].join("\n");
    const report = run(snapshot({ path: ".github/workflows/ci.yml", content }, config()));

    expect(report.findings).toEqual([]);
  });

  it("accepts checker binaries followed by a check command", () => {
    for (const binary of ["slophammer-go", "slophammer-ts", "slophammer-rs", "slophammer-py"]) {
      const report = run(snapshot(workflow(`${binary} check .`), config()));

      expect(report.findings).toEqual([]);
    }
  });

  it("requires the check invocation near the binary", () => {
    const distant = `slophammer-ts dry . && echo ${"x".repeat(200)} && make check`;
    const report = run(snapshot(workflow(distant), config()));

    expect(report.findings).toHaveLength(1);
  });

  it("credits checker invocations reached through package scripts", () => {
    const packageFile = {
      path: "package.json",
      content: JSON.stringify({ scripts: { gate: "slophammer-ts check ." } })
    };
    const report = run(snapshot(workflow("npm run gate"), packageFile, config("slophammer.yaml")));

    expect(report.findings).toEqual([]);
  });
});

function run(target: Snapshot): ReturnType<typeof runRules> {
  return runRules(target, emptyConfig(), { onlyRuleIDs: [ruleID] });
}

function snapshot(...files: readonly RepoFile[]): Snapshot {
  return newSnapshot("/repo", files);
}

function workflow(command: string): RepoFile {
  return {
    path: ".github/workflows/ci.yml",
    content: [
      "name: CI",
      "on: [push]",
      "jobs:",
      "  gate:",
      "    steps:",
      `      - run: ${command}`,
      ""
    ].join("\n")
  };
}

function config(name = "slophammer.yml"): RepoFile {
  return { path: name, content: "rules: {}\n" };
}
