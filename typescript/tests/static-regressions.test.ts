import { describe, expect, it } from "vitest";

import type { Config } from "../src/config/config.js";
import { emptyConfig } from "../src/config/config.js";
import { newSnapshot, type Snapshot } from "../src/repo/repo.js";
import { RulePolicy } from "../src/rules/policy.js";
import { runRules } from "../src/rules/rules.js";
import { bindingScriptWorkflow } from "./helpers.js";

describe("TypeScript static rule regressions", () => {
  it("does not accept npm test wrappers without a real package test runner", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: ".github/workflows/ci.yml",
          content: "name: ci\non: [push]\njobs:\n  test:\n    steps:\n      - run: npm test\n"
        },
        packageWithScripts({ test: "echo ok" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.test-required");
  });

  it("does not apply Vitest config thresholds to nyc without check coverage", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "nyc mocha" }),
        enabledESLintConfig(),
        coverageConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("does not apply Vitest config thresholds to Jest coverage", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "jest --coverage" }),
        enabledESLintConfig(),
        coverageConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("accepts Jest coverage with Jest config thresholds", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "jest --coverage" }),
        enabledESLintConfig(),
        jestCoverageConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("accepts Vitest coverage thresholds from Vite config", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage" }),
        enabledESLintConfig(),
        { path: "vite.config.ts", content: coverageConfig().content }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("accepts coverage wrappers that forward coverage args to the test script", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ test: "vitest run", coverage: "npm run test -- --coverage" }),
        enabledESLintConfig(),
        coverageConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("does not accept Oxlint unsafe rules without a type-aware Oxlint command", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "oxlint src" }),
        enabledOxlintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.no-unsafe-types");
  });

  it("accepts Oxlint warning rules when deny-warnings runs", () => {
    const report = runOnly(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "oxlint --deny-warnings src" }),
        enabledOxlintNoExplicitAnyWarnConfig()
      ]),
      "ts.no-explicit-any"
    );

    expect(report.findings).toEqual([]);
  });

  it("does not satisfy lint-rule evidence with an unused ESLint config", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "biome check ." }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });
});

describe("TypeScript tool evidence false positives", () => {
  it("does not accept Oxlint rules disabled by later overrides", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "oxlint --type-aware --deny-warnings src" }),
        disabledOxlintOverrideConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });

  it("does not let test-only Oxlint overrides disable production rule evidence", () => {
    const report = runOnly(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "oxlint --deny-warnings src" }),
        testOverrideOxlintConfig()
      ]),
      "ts.no-explicit-any"
    );

    expect(report.findings).toEqual([]);
  });

  it("does not treat echoed workflow matrix values as executed commands", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: ".github/workflows/ci.yml",
          content: echoedMatrixWorkflow()
        },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.typecheck-required");
  });

  it("does not treat action input command keys as workflow matrix commands", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: ".github/workflows/ci.yml",
          content: actionInputCommandWorkflow()
        },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.typecheck-required");
  });

  it("does not share workflow matrix commands across jobs", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: ".github/workflows/ci.yml",
          content: multiJobMatrixWorkflow()
        },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.typecheck-required");
  });
});

describe("TypeScript mutation evidence regressions", () => {
  it("does not accept missing TypeScript mutation command placeholders", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          mutate: "slophammer typescript mutate . && echo typescript mutation"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });

  it("does not accept dry-run mutation commands", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ mutate: "stryker run --dryRunOnly" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });

  it("accepts executing stryker runs with dry-run timeout tuning", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ mutate: "stryker run --dryRunTimeoutMinutes 10" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.mutation-required");
  });

  it("does not accept fixture stryker configs as the breaking threshold", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
        {
          path: "fixtures/demo/stryker.conf.json",
          content: '{"thresholds":{"high":70,"low":50,"break":50}}'
        },
        packageWithScripts({ mutate: "stryker run" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });
});

describe("TypeScript mutation threshold regressions", () => {
  it("does not accept commented-out or malformed breaking thresholds", () => {
    const weakConfigs = [
      { path: "stryker.conf.mjs", content: "// thresholds: { break: 50 }\nexport default {};\n" },
      { path: "stryker.conf.json", content: '{"thresholds":{"break":50}' },
      { path: "stryker.conf.json", content: '{"thresholds":{"break":0}}' },
      { path: "stryker.conf.json", content: '{"dryRunOnly":true,"thresholds":{"break":50}}' },
      {
        path: "stryker.conf.mjs",
        content: "export default { dryRunOnly: true, thresholds: { break: 50 } };\n"
      }
    ];
    for (const config of weakConfigs) {
      const report = runRules(
        newSnapshot("/repo", [
          ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
          config,
          packageWithScripts({ mutate: "stryker run" }),
          enabledESLintConfig()
        ]),
        emptyConfig()
      );

      expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
    }
  });

  it("does not accept a subdirectory stryker config the run never loads", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
        {
          path: "docs/stryker.conf.json",
          content: '{"thresholds":{"high":70,"low":50,"break":50}}'
        },
        packageWithScripts({ mutate: "stryker run" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });

  it("honors an explicit stryker config with a breaking threshold", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
        {
          path: "config/stryker.conf.json",
          content: '{"thresholds":{"high":70,"low":50,"break":50}}'
        },
        packageWithScripts({ mutate: "stryker run config/stryker.conf.json" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.mutation-required");
  });

  it("does not accept an explicit stryker config without a breaking threshold", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
        { path: "config/stryker.conf.json", content: '{"thresholds":{"high":70,"low":50}}' },
        packageWithScripts({ mutate: "stryker run config/stryker.conf.json" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });

  it("accepts an uncommented breaking threshold in a js config", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
        {
          path: "stryker.config.mjs",
          content: "export default { thresholds: { high: 70, low: 50, break: 50 } };\n"
        },
        packageWithScripts({ mutate: "stryker run" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.mutation-required");
  });

  it("does not accept stryker runs without a breaking threshold", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles().filter((file) => file.path !== "stryker.conf.json"),
        packageWithScripts({ mutate: "stryker run" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });

  it("does not accept non-run stryker invocations", () => {
    for (const weak of ["stryker init", "stryker --help"]) {
      const report = runRules(
        newSnapshot("/repo", [
          ...baseTypeScriptFiles(),
          packageWithScripts({ mutate: weak }),
          enabledESLintConfig()
        ]),
        emptyConfig()
      );

      expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
    }
  });

  it("does not accept mutation commands whose failures are ignored", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ mutate: "stryker run || true" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });
});

describe("TypeScript command failure regressions", () => {
  it("does not accept lint commands whose failures are ignored", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ lint: "eslint . || true" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.lint-required");
  });

  it("does not accept DRY commands whose failures are ignored", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ dry: "slophammer typescript dry . || true" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.dry-required");
  });

  it("checks import type expressions against dependency boundaries", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        { path: "src/app/index.ts", content: 'type Secret = import("../secret/secret").Secret;\n' },
        { path: "src/secret/secret.ts", content: "export type Secret = string;\n" }
      ]),
      configWithBoundary()
    );

    expect(report.findings).toEqual([
      {
        rule_id: "ts.dependency-boundaries-required",
        severity: "error",
        path: "src/app/index.ts",
        message: "Import src/secret/secret is outside allowed dependencies for src/app"
      }
    ]);
  });
});

function runOnly(snapshot: Snapshot, ruleID: string): ReturnType<typeof runRules> {
  const config = emptyConfig();
  return runRules(snapshot, config, new RulePolicy(config.rules, [ruleID]));
}

function baseTypeScriptFiles(): readonly { readonly path: string; readonly content: string }[] {
  return [
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: "# Agents\n" },
    { path: ".github/workflows/ci.yml", content: bindingScriptWorkflow() },
    {
      path: "tsconfig.json",
      content:
        '{"compilerOptions":{"strict":true,"noImplicitAny":true,"noImplicitOverride":true,"noUncheckedIndexedAccess":true,"exactOptionalPropertyTypes":true,"noFallthroughCasesInSwitch":true,"noPropertyAccessFromIndexSignature":true,"useUnknownInCatchVariables":true,"noEmitOnError":true}}'
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
      scripts: { ...packageScripts(), ...overrides }
    })
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

function enabledESLintConfig(): { readonly path: string; readonly content: string } {
  return {
    path: "eslint.config.mjs",
    content:
      'export default [{rules:{"@typescript-eslint/no-explicit-any":"error","@typescript-eslint/no-unsafe-assignment":"error","@typescript-eslint/no-unsafe-call":"error","@typescript-eslint/no-unsafe-member-access":"error","@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}}];'
  };
}

function enabledOxlintConfig(): { readonly path: string; readonly content: string } {
  return {
    path: ".oxlintrc.json",
    content: JSON.stringify({
      rules: {
        "typescript/no-explicit-any": "error",
        "typescript/no-unsafe-assignment": "error",
        "typescript/no-unsafe-call": "error",
        "typescript/no-unsafe-member-access": "error",
        "typescript/no-unsafe-return": "error",
        "eslint/complexity": ["error", { max: 8 }]
      }
    })
  };
}

function enabledOxlintNoExplicitAnyWarnConfig(): {
  readonly path: string;
  readonly content: string;
} {
  return {
    path: ".oxlintrc.json",
    content: JSON.stringify({
      rules: {
        "typescript/no-explicit-any": "warn"
      }
    })
  };
}

function disabledOxlintOverrideConfig(): {
  readonly path: string;
  readonly content: string;
} {
  return {
    path: ".oxlintrc.json",
    content: JSON.stringify({
      rules: {
        "typescript/no-explicit-any": "error",
        "typescript/no-unsafe-assignment": "error",
        "typescript/no-unsafe-call": "error",
        "typescript/no-unsafe-member-access": "error",
        "typescript/no-unsafe-return": "error",
        "eslint/complexity": ["error", { max: 8 }]
      },
      overrides: [
        {
          files: ["src/**/*.ts"],
          rules: {
            "typescript/no-explicit-any": "off",
            "typescript/no-unsafe-assignment": "off",
            "typescript/no-unsafe-call": "off",
            "typescript/no-unsafe-member-access": "off",
            "typescript/no-unsafe-return": "off",
            "eslint/complexity": "off"
          }
        }
      ]
    })
  };
}

function testOverrideOxlintConfig(): {
  readonly path: string;
  readonly content: string;
} {
  return {
    path: ".oxlintrc.json",
    content: JSON.stringify({
      rules: {
        "typescript/no-explicit-any": "error"
      },
      overrides: [
        {
          files: ["**/*.test.ts"],
          rules: {
            "typescript/no-explicit-any": "off"
          }
        }
      ]
    })
  };
}

function echoedMatrixWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  checks:",
    "    strategy:",
    "      matrix:",
    "        include:",
    "          - command: tsc --noEmit",
    "    steps:",
    "      - run: echo ${{ matrix.command }}"
  ].join("\n");
}

function actionInputCommandWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  checks:",
    "    strategy:",
    "      matrix:",
    "        command:",
    "          - echo ok",
    "    steps:",
    "      - uses: example/action@v1",
    "        with:",
    "          command: tsc --noEmit",
    "      - run: ${{ matrix.command }}"
  ].join("\n");
}

function multiJobMatrixWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  typecheck-template:",
    "    strategy:",
    "      matrix:",
    "        command:",
    "          - tsc --noEmit",
    "    steps:",
    "      - run: echo config only",
    "  checks:",
    "    strategy:",
    "      matrix:",
    "        command:",
    "          - echo ok",
    "    steps:",
    "      - run: ${{ matrix.command }}"
  ].join("\n");
}

function coverageConfig(): { readonly path: string; readonly content: string } {
  return {
    path: "vitest.config.ts",
    content:
      "export default {test:{coverage:{thresholds:{lines:85,functions:85,branches:85,statements:85}}}};\n"
  };
}

function jestCoverageConfig(): { readonly path: string; readonly content: string } {
  return {
    path: "jest.config.js",
    content:
      "export default {coverageThreshold:{global:{lines:85,functions:85,branches:85,statements:85}}};\n"
  };
}

function configWithBoundary(): Config {
  const cfg = emptyConfig();
  return {
    ...cfg,
    typescript: {
      ...cfg.typescript,
      dependencyBoundaries: [{ from: "src/app", allow: [] }]
    }
  };
}
