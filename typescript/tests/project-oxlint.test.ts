import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";

describe("TypeScript Oxlint project evidence", () => {
  it("preserves root Oxlint config for nested TypeScript packages", () => {
    const report = runRules(newSnapshot("/repo", nestedPackageWithRootOxlintConfig()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "pkg/src", allow: [] }]
      }
    });

    expect(report.findings).toEqual([]);
  });

  it("prefers package-local Oxlint config over root evidence", () => {
    const report = runRules(newSnapshot("/repo", nestedPackageWithLocalOxlintConfig()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "pkg/src", allow: [] }]
      }
    });

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.no-explicit-any");
  });
});

function nestedPackageWithRootOxlintConfig(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    {
      path: "AGENTS.md",
      content: "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
    },
    { path: ".github/workflows/ci.yml", content: nestedPackageOxlintWorkflow() },
    { path: ".oxlintrc.json", content: oxlintConfig() },
    {
      path: "pkg/package.json",
      content: JSON.stringify({ devDependencies: { typescript: "^5.0.0", oxlint: "^1.0.0" } })
    },
    { path: "pkg/src/index.ts", content: "export const value: number = 1;\n" },
    { path: "pkg/tsconfig.json", content: strictTSConfig() },
    { path: "pkg/stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "pkg/vitest.config.ts", content: coverageConfig() }
  ];
}

function nestedPackageWithLocalOxlintConfig(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    ...nestedPackageWithRootOxlintConfig(),
    {
      path: "pkg/.oxlintrc.json",
      content: JSON.stringify({
        rules: {
          "typescript/no-explicit-any": "off",
          "typescript/no-unsafe-assignment": "error",
          "typescript/no-unsafe-call": "error",
          "typescript/no-unsafe-member-access": "error",
          "typescript/no-unsafe-return": "error",
          "eslint/complexity": ["error", { max: 8 }]
        }
      })
    }
  ];
}

function nestedPackageOxlintWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  ts:",
    "    runs-on: ubuntu-latest",
    "    defaults:",
    "      run:",
    "        working-directory: pkg",
    "    steps:",
    "      - run: prettier --check .",
    "      - run: oxlint --type-aware --deny-warnings .",
    "      - run: tsc --noEmit",
    "      - run: vitest run",
    "      - run: vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "      - run: slophammer typescript dry .",
    "      - run: stryker run"
  ].join("\n");
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

function oxlintConfig(): string {
  return JSON.stringify({
    rules: {
      "typescript/no-explicit-any": "error",
      "typescript/no-unsafe-assignment": "error",
      "typescript/no-unsafe-call": "error",
      "typescript/no-unsafe-member-access": "error",
      "typescript/no-unsafe-return": "error",
      "eslint/complexity": ["error", { max: 8 }]
    }
  });
}

function coverageConfig(): string {
  return "export default {test:{coverage:{thresholds:{lines:85,functions:85,branches:85,statements:85}}}};\n";
}
