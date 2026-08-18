import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";
import { bindingScriptWorkflow } from "./helpers.js";

describe("TypeScript formatter evidence", () => {
  it("rejects mutating Biome format commands", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ format: "biome format --write ." }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.format-required");
  });

  it("accepts non-mutating Biome check commands", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ format: "biome check ." }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.format-required");
  });
});

function baseTypeScriptFiles(): readonly { readonly path: string; readonly content: string }[] {
  return [
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "README.md", content: "# Repo\n" },
    {
      path: "AGENTS.md",
      content: "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
    },
    { path: ".github/workflows/ci.yml", content: bindingScriptWorkflow() },
    {
      path: "tsconfig.json",
      content: '{"compilerOptions":{"strict":true}}\n'
    }
  ];
}

function packageWithScripts(overrides: Readonly<Partial<Record<string, string>>> = {}): {
  readonly path: string;
  readonly content: string;
} {
  return {
    path: "package.json",
    content: JSON.stringify({
      scripts: {
        typecheck: "tsc --noEmit",
        lint: "eslint .",
        test: "vitest run",
        coverage:
          "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
        dry: "slophammer-ts dry .",
        mutate: "stryker run",
        ...overrides
      }
    })
  };
}

function enabledESLintConfig(): { readonly path: string; readonly content: string } {
  return {
    path: "eslint.config.mjs",
    content:
      'export default [{rules:{"@typescript-eslint/no-explicit-any":"error","@typescript-eslint/no-unsafe-assignment":"error","@typescript-eslint/no-unsafe-call":"error","@typescript-eslint/no-unsafe-member-access":"error","@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}}];'
  };
}
