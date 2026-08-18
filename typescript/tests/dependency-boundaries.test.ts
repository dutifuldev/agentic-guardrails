import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";

describe("TypeScript dependency boundaries", () => {
  it("checks dynamic imports", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.ts",
          content: "export async function load() { return import('../forbidden'); }\n"
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: ["src/allowed"] }])
    );

    expect(report.findings).toContainEqual(
      expect.objectContaining({
        rule_id: "ts.dependency-boundaries-required",
        path: "src/app/index.ts"
      })
    );
  });

  it("checks CommonJS require calls", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.js",
          content: 'const forbidden = require("../forbidden");\nmodule.exports = forbidden;\n'
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: ["src/allowed"] }])
    );

    expect(report.findings).toContainEqual(
      expect.objectContaining({
        rule_id: "ts.dependency-boundaries-required",
        path: "src/app/index.js"
      })
    );
  });

  it("checks Node module source extensions", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.mjs",
          content: 'import "../forbidden.mjs";\n'
        },
        {
          path: "src/app/legacy.cjs",
          content: 'const forbidden = require("../forbidden.cjs");\nmodule.exports = forbidden;\n'
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: ["src/allowed"] }])
    );

    expect(report.findings).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          rule_id: "ts.dependency-boundaries-required",
          path: "src/app/index.mjs"
        }),
        expect.objectContaining({
          rule_id: "ts.dependency-boundaries-required",
          path: "src/app/legacy.cjs"
        })
      ])
    );
  });

  it("checks TypeScript import-equals require declarations", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.ts",
          content: 'import forbidden = require("../forbidden");\nexport const value = forbidden;\n'
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: ["src/allowed"] }])
    );

    expect(report.findings).toContainEqual(
      expect.objectContaining({
        rule_id: "ts.dependency-boundaries-required",
        path: "src/app/index.ts"
      })
    );
  });
});

describe("TypeScript dependency boundary exclusions", () => {
  it("ignores commented and string-only import text", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.ts",
          content: [
            '// import "../forbidden";',
            'const text = "import \\"../forbidden\\"";',
            "export const value = 1;"
          ].join("\n")
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: ["src/allowed"] }])
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain(
      "ts.dependency-boundaries-required"
    );
  });

  it("allows imports inside the same boundary", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.ts",
          content: 'import { helper } from "./helper";\nexport const value = helper();\n'
        },
        {
          path: "src/app/helper.ts",
          content: "export function helper() { return 1; }\n"
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: [] }])
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain(
      "ts.dependency-boundaries-required"
    );
  });

  it("allows NodeNext js specifiers for TypeScript source files", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "src/app/index.ts",
          content: 'import { logger } from "../logger.js";\nexport const value = logger;\n'
        },
        {
          path: "src/logger.ts",
          content: "export const logger = 1;\n"
        }
      ]),
      configWithBoundaries([{ from: "src/app", allow: ["src/logger.ts"] }])
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain(
      "ts.dependency-boundaries-required"
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

function packageWithScripts(): { readonly path: string; readonly content: string } {
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

function configWithBoundaries(
  dependencyBoundaries: readonly { readonly from: string; readonly allow: readonly string[] }[]
): ReturnType<typeof emptyConfig> {
  return {
    ...emptyConfig(),
    typescript: {
      ...emptyConfig().typescript,
      dependencyBoundaries
    }
  };
}
