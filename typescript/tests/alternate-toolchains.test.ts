import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";

describe("TypeScript alternate toolchains", () => {
  it("accepts tsgo, Oxlint, Oxfmt, Node test, c8, and matrix Slophammer commands", () => {
    const report = runRules(newSnapshot("/repo", alternateToolchainFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "src", allow: [] }]
      }
    });

    expect(report.findings).toEqual([]);
  });
});

function alternateToolchainFiles(): readonly { readonly path: string; readonly content: string }[] {
  return [
    ...baseTypeScriptFiles(),
    {
      path: "package.json",
      content: JSON.stringify({
        scripts: {
          format: "oxfmt --check",
          lint: "oxlint --type-aware --deny-warnings src",
          typecheck: "tsgo --noEmit",
          test: "node --test dist-test/test/*.test.js",
          coverage:
            "c8 --all --check-coverage --lines 85 --branches 85 --functions 85 --statements 85 node --test dist-test/test/*.test.js",
          mutate: "stryker run"
        },
        devDependencies: {
          "@typescript/native-preview": "^7.0.0",
          oxlint: "^1.0.0",
          oxfmt: "^1.0.0"
        }
      })
    },
    {
      path: ".oxlintrc.jsonc",
      content: [
        "{",
        '  "rules": {',
        '    "typescript/no-explicit-any": "error",',
        "  },",
        '  "overrides": [',
        "    {",
        '      "files": ["src/**/*.ts"],',
        '      "rules": {',
        '        "eslint/complexity": ["error", { "max": 8 }],',
        '        "typescript/no-unsafe-assignment": "error",',
        '        "typescript/no-unsafe-call": "error",',
        '        "typescript/no-unsafe-member-access": "error",',
        '        "typescript/no-unsafe-return": "error",',
        "      },",
        "    },",
        "  ],",
        "}"
      ].join("\n")
    },
    {
      path: ".github/workflows/ci.yml",
      content: matrixWorkflow()
    }
  ];
}

function baseTypeScriptFiles(): readonly { readonly path: string; readonly content: string }[] {
  return [
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "README.md", content: "# Repo\n" },
    {
      path: "AGENTS.md",
      content: "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
    },
    { path: "src/index.ts", content: "export const value = 1;\n" },
    {
      path: "tsconfig.json",
      content: JSON.stringify({ compilerOptions: { strict: true } })
    }
  ];
}

function matrixWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  scripts:",
    "    steps:",
    "      - run: npm run format",
    "      - run: npm run lint",
    "      - run: npm run typecheck",
    "      - run: npm test",
    "      - run: npm run coverage",
    "      - run: npm run mutate",
    "  checks:",
    "    strategy:",
    "      matrix:",
    "        include:",
    "          - command: |",
    "              pnpm dlx slophammer-ts@latest dry .",
    "    steps:",
    "      - run: ${{ matrix.command }}",
    ""
  ].join("\n");
}
