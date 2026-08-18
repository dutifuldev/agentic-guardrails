import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";

describe("TypeScript project scopes", () => {
  it("uses the nearest package root for nested TypeScript sources without local tool dependencies", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        { path: "packages/app/package.json", content: JSON.stringify({ name: "app" }) },
        { path: "packages/app/lib/index.ts", content: "export const value = 1;\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings).toContainEqual(
      expect.objectContaining({
        rule_id: "ts.strict-required",
        path: "packages/app/tsconfig.json"
      })
    );
    expect(report.findings).toContainEqual(
      expect.objectContaining({
        rule_id: "ts.typecheck-required",
        path: "packages/app/.github/workflows"
      })
    );
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
    { path: ".github/workflows/ci.yml", content: "name: ci\n" },
    {
      path: "tsconfig.json",
      content:
        '{"compilerOptions":{"strict":true,"noImplicitAny":true,"noImplicitOverride":true,"noUncheckedIndexedAccess":true,"exactOptionalPropertyTypes":true,"noFallthroughCasesInSwitch":true,"noPropertyAccessFromIndexSignature":true,"useUnknownInCatchVariables":true,"noEmitOnError":true}}'
    }
  ];
}

function packageWithScripts(): {
  readonly path: string;
  readonly content: string;
} {
  return {
    path: "package.json",
    content: JSON.stringify({
      scripts: {
        format: "prettier --check .",
        lint: "eslint .",
        typecheck: "tsc --noEmit",
        test: "vitest run",
        coverage:
          "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
        dry: "slophammer typescript dry .",
        mutate: "stryker run"
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
