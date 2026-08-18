import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";
import { bindingScriptWorkflow } from "./helpers.js";

describe("TypeScript ESLint parsing", () => {
  it("accepts legacy JavaScript ESLint configs", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: ".eslintrc.cjs",
          content:
            'module.exports = {rules:{"@typescript-eslint/no-explicit-any":"error","@typescript-eslint/no-unsafe-assignment":"error","@typescript-eslint/no-unsafe-call":"error","@typescript-eslint/no-unsafe-member-access":"error","@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}};'
        }
      ]),
      emptyConfig()
    );

    const ruleIDs = report.findings.map((finding) => finding.rule_id);
    expect(ruleIDs).not.toContain("ts.no-explicit-any");
    expect(ruleIDs).not.toContain("ts.no-unsafe-types");
    expect(ruleIDs).not.toContain("ts.complexity-required");
  });

  it("accepts package.json ESLint configs", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageJSONWith({
          scripts: packageScripts(),
          eslintConfig: {
            rules: {
              "@typescript-eslint/no-explicit-any": "error",
              "@typescript-eslint/no-unsafe-assignment": "error",
              "@typescript-eslint/no-unsafe-call": "error",
              "@typescript-eslint/no-unsafe-member-access": "error",
              "@typescript-eslint/no-unsafe-return": "error",
              complexity: ["error", 8]
            }
          }
        })
      ]),
      emptyConfig()
    );

    const ruleIDs = report.findings.map((finding) => finding.rule_id);
    expect(ruleIDs).not.toContain("ts.no-explicit-any");
    expect(ruleIDs).not.toContain("ts.no-unsafe-types");
    expect(ruleIDs).not.toContain("ts.complexity-required");
  });

  it("does not accept disabled ESLint rules", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"off","@typescript-eslint/no-unsafe-assignment":0,"@typescript-eslint/no-unsafe-call":"off","@typescript-eslint/no-unsafe-member-access":"off","@typescript-eslint/no-unsafe-return":"off",complexity:["error",8]}}];'
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types"])
    );
  });

  it("does not accept rules disabled by later ESLint overrides", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content: [
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"error",',
            '"@typescript-eslint/no-unsafe-assignment":"error",',
            '"@typescript-eslint/no-unsafe-call":"error",',
            '"@typescript-eslint/no-unsafe-member-access":"error",',
            '"@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}},',
            '{rules:{"@typescript-eslint/no-explicit-any":"off",',
            '"@typescript-eslint/no-unsafe-assignment":"off",',
            '"@typescript-eslint/no-unsafe-call":"off",',
            '"@typescript-eslint/no-unsafe-member-access":"off",',
            '"@typescript-eslint/no-unsafe-return":"off",complexity:"off"}}];'
          ].join("")
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });

  it("does not accept ESLint settings as rules", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{settings:{"@typescript-eslint/no-explicit-any":"error","@typescript-eslint/no-unsafe-assignment":"error","@typescript-eslint/no-unsafe-call":"error","@typescript-eslint/no-unsafe-member-access":"error","@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}}];'
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });
});

describe("TypeScript ESLint warning policy", () => {
  it("does not accept warn-level ESLint rules unless warnings fail", () => {
    const weak = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"warn","@typescript-eslint/no-unsafe-assignment":1,"@typescript-eslint/no-unsafe-call":"warn","@typescript-eslint/no-unsafe-member-access":"warn","@typescript-eslint/no-unsafe-return":"warn",complexity:["warn",8]}}];'
        }
      ]),
      emptyConfig()
    );
    const strict = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "eslint . --max-warnings=0" }),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"warn","@typescript-eslint/no-unsafe-assignment":1,"@typescript-eslint/no-unsafe-call":"warn","@typescript-eslint/no-unsafe-member-access":"warn","@typescript-eslint/no-unsafe-return":"warn",complexity:["warn",8]}}];'
        }
      ]),
      emptyConfig()
    );

    expect(weak.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
    const strictRuleIDs = strict.findings.map((finding) => finding.rule_id);
    expect(strictRuleIDs).not.toContain("ts.no-explicit-any");
    expect(strictRuleIDs).not.toContain("ts.no-unsafe-types");
    expect(strictRuleIDs).not.toContain("ts.complexity-required");
  });

  it("accepts warn-level ESLint rules when max warnings is forwarded through npm", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "eslint ." }),
        {
          path: ".github/workflows/ci.yml",
          content:
            "name: ci\non: [push]\njobs:\n  lint:\n    steps:\n      - run: npm run lint -- --max-warnings=0\n"
        },
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"warn","@typescript-eslint/no-unsafe-assignment":1,"@typescript-eslint/no-unsafe-call":"warn","@typescript-eslint/no-unsafe-member-access":"warn","@typescript-eslint/no-unsafe-return":"warn",complexity:["warn",8]}}];'
        }
      ]),
      emptyConfig()
    );

    const ruleIDs = report.findings.map((finding) => finding.rule_id);
    expect(ruleIDs).not.toContain("ts.no-explicit-any");
    expect(ruleIDs).not.toContain("ts.no-unsafe-types");
    expect(ruleIDs).not.toContain("ts.complexity-required");
  });
});

describe("TypeScript ESLint rule names", () => {
  it("does not accept unscoped names for TypeScript ESLint safety rules", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"no-explicit-any":"error","no-unsafe-assignment":"error","no-unsafe-call":"error","no-unsafe-member-access":"error","no-unsafe-return":"error",complexity:["error",8]}}];'
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types"])
    );
  });
});

describe("TypeScript ESLint comment and YAML parsing", () => {
  it("does not accept commented ESLint rules", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content: [
            "export default [{rules:{",
            '// "@typescript-eslint/no-explicit-any": "error",',
            '// "@typescript-eslint/no-unsafe-assignment": "error",',
            '// "@typescript-eslint/no-unsafe-call": "error",',
            '// "@typescript-eslint/no-unsafe-member-access": "error",',
            '// "@typescript-eslint/no-unsafe-return": "error",',
            '// complexity: ["error", 8]',
            "}}];"
          ].join("\n")
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });

  it("does not accept commented YAML ESLint rules", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: ".eslintrc.yml",
          content: [
            "rules:",
            '  # "@typescript-eslint/no-explicit-any": "error"',
            '  # "@typescript-eslint/no-unsafe-assignment": "error"',
            '  # "@typescript-eslint/no-unsafe-call": "error"',
            '  # "@typescript-eslint/no-unsafe-member-access": "error"',
            '  # "@typescript-eslint/no-unsafe-return": "error"',
            '  # "complexity": ["error", 8]'
          ].join("\n")
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });

  it("accepts unquoted YAML ESLint severities", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: ".eslintrc.yml",
          content: [
            "rules:",
            "  @typescript-eslint/no-explicit-any: error",
            "  @typescript-eslint/no-unsafe-assignment: error",
            "  @typescript-eslint/no-unsafe-call: error",
            "  @typescript-eslint/no-unsafe-member-access: error",
            "  @typescript-eslint/no-unsafe-return: error",
            "  complexity: [error, 8]"
          ].join("\n")
        }
      ]),
      emptyConfig()
    );

    const ruleIDs = report.findings.map((finding) => finding.rule_id);
    expect(ruleIDs).not.toContain("ts.no-explicit-any");
    expect(ruleIDs).not.toContain("ts.no-unsafe-types");
    expect(ruleIDs).not.toContain("ts.complexity-required");
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
      content:
        '{"compilerOptions":{"strict":true,"noImplicitAny":true,"noImplicitOverride":true,"noUncheckedIndexedAccess":true,"exactOptionalPropertyTypes":true,"noFallthroughCasesInSwitch":true,"noPropertyAccessFromIndexSignature":true,"useUnknownInCatchVariables":true,"noEmitOnError":true}}'
    }
  ];
}

function packageWithScripts(overrides: Readonly<Record<string, string>> = {}): {
  readonly path: string;
  readonly content: string;
} {
  return packageJSONWith({
    scripts: { ...packageScripts(), ...overrides }
  });
}

function packageJSONWith(value: Readonly<Record<string, unknown>>): {
  readonly path: string;
  readonly content: string;
} {
  return {
    path: "package.json",
    content: JSON.stringify(value)
  };
}

function packageScripts(): Readonly<Record<string, string>> {
  return {
    format: "prettier --check .",
    lint: "eslint .",
    typecheck: "tsc --noEmit",
    test: "vitest run",
    coverage:
      "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    dry: "slophammer typescript dry .",
    mutate: "stryker run"
  };
}
