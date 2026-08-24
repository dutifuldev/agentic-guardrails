import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { commandFiles, newSnapshot } from "../src/repo/repo.js";
import { RulePolicy } from "../src/rules/policy.js";
import { repoEvidenceFiles } from "../src/rules/project-evidence.js";
import { runRules } from "../src/rules/rules.js";

describe("synthetic repo evidence", () => {
  it("credits synthetic __repo_ files without a referencing workflow", () => {
    const snapshot = newSnapshot("/repo", [
      {
        path: "scripts/__repo_workflow_github_workflows_ci_yml.sh",
        content: "npm run lint\n"
      },
      { path: "scripts/__repo_package_scripts.sh", content: "lint: eslint .\n" },
      { path: "scripts/unreferenced.sh", content: "npm run mutate\n" }
    ]);

    const credited = commandFiles(snapshot).map((file) => file.path);

    expect(credited).toContain("scripts/__repo_workflow_github_workflows_ci_yml.sh");
    expect(credited).toContain("scripts/__repo_package_scripts.sh");
    expect(credited).not.toContain("scripts/unreferenced.sh");
  });

  it("credits real scripts referenced from synthetic workflow evidence", () => {
    const snapshot = newSnapshot("/repo", [
      {
        path: "scripts/__repo_workflow_github_workflows_ci_yml.sh",
        content: "bash scripts/gate.sh\n"
      },
      { path: "scripts/gate.sh", content: "eslint .\n" }
    ]);

    expect(commandFiles(snapshot).map((file) => file.path)).toContain("scripts/gate.sh");
  });

  it("drops steps neutralized by expression-literal continue-on-error", () => {
    const snapshot = newSnapshot("/repo", [
      {
        path: ".github/workflows/ci.yml",
        content: [
          "name: CI",
          "on: [push]",
          "jobs:",
          "  check:",
          "    steps:",
          "      - run: npm run lint",
          "        continue-on-error: ${{ true }}",
          "      - run: npm run typecheck",
          "        if: ${{ false }}",
          "      - run: npm test",
          ""
        ].join("\n")
      }
    ]);

    const evidence = commandFiles(snapshot)
      .map((file) => file.content)
      .join("\n");

    expect(evidence).not.toContain("npm run lint");
    expect(evidence).not.toContain("npm run typecheck");
    expect(evidence).toContain("npm test");
  });

  it("keeps nested package scopes supplied by scoped root workflows", () => {
    const config = emptyConfig();
    const report = runRules(
      newSnapshot("/repo", nestedPackageRepo("on: [push]")),
      config,
      new RulePolicy(config.rules, ["ts.lint-required"])
    );

    expect(report.findings).toEqual([]);
  });

  it("drops tag-only push workflows from binding evidence", () => {
    const snapshot = newSnapshot("/repo", [
      {
        path: ".github/workflows/release.yml",
        content: [
          "name: Release",
          "on:",
          "  push:",
          '    tags: ["v*"]',
          "jobs:",
          "  release:",
          "    steps:",
          "      - run: npm test",
          ""
        ].join("\n")
      }
    ]);

    const evidence = commandFiles(snapshot)
      .map((file) => file.content)
      .join("\n");

    expect(evidence).not.toContain("npm test");
  });

  it("drops scoped evidence from non-binding root workflows", () => {
    const config = emptyConfig();
    const report = runRules(
      newSnapshot("/repo", nestedPackageRepo("on: workflow_dispatch")),
      config,
      new RulePolicy(config.rules, ["ts.lint-required"])
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.lint-required");
  });

  it("synthesizes no root-scope evidence when no nested roots exist", () => {
    const snapshot = newSnapshot("/repo", [
      { path: ".github/workflows/ci.yml", content: rootWorkflow("on: [push]") },
      { path: "package.json", content: JSON.stringify({ scripts: { lint: "eslint ." } }) }
    ]);

    expect(repoEvidenceFiles(snapshot, ".", ["."])).toEqual([]);
  });
});

function nestedPackageRepo(
  trigger: string
): readonly { readonly path: string; readonly content: string }[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: "# Agents\n" },
    { path: ".github/workflows/ci.yml", content: rootWorkflow(trigger) },
    {
      path: "pkg/package.json",
      content: JSON.stringify({ devDependencies: { typescript: "^5.0.0" } })
    },
    { path: "pkg/src/index.ts", content: "export const value: number = 1;\n" },
    { path: "pkg/tsconfig.json", content: JSON.stringify({ compilerOptions: { strict: true } }) }
  ];
}

function rootWorkflow(trigger: string): string {
  return [
    "name: CI",
    trigger,
    "jobs:",
    "  check:",
    "    steps:",
    "      - run: cd pkg && eslint .",
    ""
  ].join("\n");
}
