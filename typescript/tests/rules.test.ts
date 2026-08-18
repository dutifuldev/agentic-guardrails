import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";
import { bindingScriptWorkflow } from "./helpers.js";

const agentInstructions =
  "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n";

describe("TypeScript rules", () => {
  it("accepts case-insensitive shared repo filenames", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "readme.md", content: "# Repo\n" },
        { path: "agents.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings).toEqual([]);
  });

  it("requires shared repo filenames at the root", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "docs/README.md", content: "# Repo\n" },
        {
          path: "packages/app/AGENTS.md",
          content:
            "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
        },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["repo.readme-required", "repo.agents-required"])
    );
  });

  it("requires workflow files to be direct children", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/archive/ci.yml", content: "name: ci\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("repo.ci-required");
  });

  it("ignores fixture-only TypeScript signals", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" },
        { path: "fixtures/repos/example/package.json", content: "{}" },
        { path: "fixtures/repos/example/src/example.ts", content: "export const x = 1;\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings).toEqual([]);
  });

  it("does not treat generic JavaScript package data as TypeScript", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" },
        {
          path: "package.json",
          content: JSON.stringify({
            scripts: { format: "prettier ." },
            devDependencies: { "@types/node": "^22.0.0" }
          })
        },
        { path: "src/index.js", content: "export const value = 1;\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings).toEqual([]);
  });

  it("treats TypeScript package signals as TypeScript", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" },
        {
          path: "package.json",
          content: JSON.stringify({ devDependencies: { typescript: "^5.0.0" } })
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.strict-required");
  });
});

describe("TypeScript command rules", () => {
  it("requires production package metadata", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: "fixtures/repos/example/package.json",
          content: JSON.stringify({ scripts: packageScripts() })
        },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.package-required");
  });

  it("does not accept Go mutation commands as TypeScript mutation evidence", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "run: go run ./cmd/slophammer go mutate\n" },
        {
          path: "package.json",
          content: JSON.stringify({
            scripts: {
              format: "prettier --check .",
              lint: "eslint .",
              typecheck: "tsc --noEmit",
              test: "vitest run",
              coverage:
                "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
              dry: "slophammer typescript dry ."
            }
          })
        },
        {
          path: "tsconfig.json",
          content: JSON.stringify({
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
          })
        },
        {
          path: "eslint.config.mjs",
          content: [
            "no-explicit-any",
            "no-unsafe-assignment",
            "no-unsafe-call",
            "no-unsafe-member-access",
            "no-unsafe-return",
            "complexity",
            "8"
          ].join("\n")
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.mutation-required");
  });

  it("does not accept npm dry wrappers without Slophammer package scripts", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: ".github/workflows/ci.yml",
          content: "name: ci\non: [push]\njobs:\n  test:\n    steps:\n      - run: npm run dry\n"
        },
        packageWithScripts({ dry: "echo dry" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.dry-required");
  });
});

describe("TypeScript rule parsing", () => {
  it("does not treat package dependency names as command declarations", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: "package.json",
          content: JSON.stringify({
            devDependencies: {
              eslint: "^9",
              prettier: "^3",
              vitest: "^3"
            }
          })
        },
        {
          path: "build.config.mjs",
          content: 'console.log("eslint prettier vitest tsc --noEmit");\n'
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining([
        "ts.format-required",
        "ts.lint-required",
        "ts.test-required",
        "ts.typecheck-required"
      ])
    );
  });

  it("does not accept no-op lint or format scripts", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          lint: "echo lint",
          format: "echo format"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.lint-required", "ts.format-required"])
    );
  });
});

describe("TypeScript rule parsing", () => {
  it("accepts compact valid tsconfig JSON", () => {
    const report = runRules(
      newSnapshot("/repo", [...baseTypeScriptFiles(), packageWithScripts(), enabledESLintConfig()]),
      emptyConfig()
    );

    const ruleIDs = report.findings.map((finding) => finding.rule_id);
    expect(ruleIDs).not.toContain("ts.strict-required");
    expect(ruleIDs).not.toContain("ts.typecheck-required");
  });

  it("accepts multi-line ESLint rule values", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content: [
            "export default [{",
            "  rules: {",
            '    "@typescript-eslint/no-explicit-any": [',
            '      "error",',
            "      { ignoreRestArgs: false }",
            "    ],",
            '    "@typescript-eslint/no-unsafe-assignment": [',
            '      "error"',
            "    ],",
            '    "@typescript-eslint/no-unsafe-call": "error",',
            '    "@typescript-eslint/no-unsafe-member-access": "error",',
            '    "@typescript-eslint/no-unsafe-return": "error",',
            '    "complexity": [',
            '      "error",',
            "      8",
            "    ]",
            "  }",
            "}];"
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

  it("accepts stricter configured complexity limits", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"error","@typescript-eslint/no-unsafe-assignment":"error","@typescript-eslint/no-unsafe-call":"error","@typescript-eslint/no-unsafe-member-access":"error","@typescript-eslint/no-unsafe-return":"error",complexity:["error",5]}}];'
        },
        { path: "slophammer.yml", content: "typescript:\n  complexity:\n    max: 5\n" }
      ]),
      {
        ...emptyConfig(),
        typescript: {
          ...emptyConfig().typescript,
          complexityMax: 5
        }
      }
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain(
      "ts.complexity-required"
    );
  });

  it("does not treat numeric ESLint severity as the complexity limit", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "eslint.config.mjs",
          content:
            'export default [{rules:{"@typescript-eslint/no-explicit-any":"error","@typescript-eslint/no-unsafe-assignment":"error","@typescript-eslint/no-unsafe-call":"error","@typescript-eslint/no-unsafe-member-access":"error","@typescript-eslint/no-unsafe-return":"error",complexity:[2,12]}}];'
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.complexity-required");
  });
});

describe("TypeScript rule parsing", () => {
  it("does not accept placeholder npm test scripts", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: "package.json",
          content: JSON.stringify({
            scripts: {
              ...packageScripts(),
              test: 'echo "Error: no test specified" && exit 1'
            }
          })
        },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.test-required");
  });

  it("accepts real test scripts alongside npm placeholders", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: "package.json",
          content: JSON.stringify({
            scripts: {
              ...packageScripts(),
              test: 'echo "Error: no test specified" && exit 1',
              "test:unit": "vitest run"
            }
          })
        },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.test-required");
  });

  it("does not accept test script labels without a test runner", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ test: "echo ok" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.test-required");
  });

  it("checks every production tsconfig", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        enabledESLintConfig(),
        {
          path: "packages/api/package.json",
          content: JSON.stringify({ scripts: packageScripts() })
        },
        {
          path: "packages/api/tsconfig.json",
          content: JSON.stringify({ compilerOptions: { strict: false } })
        },
        {
          path: "fixtures/repos/weak-fixture/tsconfig.json",
          content: JSON.stringify({ compilerOptions: { strict: false } })
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.strict-required");
  });
});

describe("TypeScript command parsing", () => {
  it("ignores fixture and template command evidence", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        { path: "package.json", content: JSON.stringify({ scripts: {} }) },
        enabledESLintConfig(),
        {
          path: "fixtures/repos/example/package.json",
          content: JSON.stringify({ scripts: packageScripts() })
        },
        {
          path: "templates/typescript/package.json",
          content: JSON.stringify({ scripts: packageScripts() })
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining([
        "ts.typecheck-required",
        "ts.lint-required",
        "ts.dry-required",
        "ts.mutation-required"
      ])
    );
  });

  it("accepts tsc project flags before noEmit", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ typecheck: "tsc -p tsconfig.json --noEmit" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain(
      "ts.typecheck-required"
    );
  });

  it("does not accept a no-op typecheck script", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ typecheck: "echo ok" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.typecheck-required");
  });

  it("ignores comments and non-run workflow text as command evidence", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        {
          path: ".github/workflows/ci.yml",
          content: [
            "name: CI",
            "on: [push]",
            "env:",
            "  NOTE: eslint tsc --noEmit slophammer typescript dry vitest run --coverage stryker",
            "jobs:",
            "  test:",
            "    steps:",
            '      - run: "# eslint . && tsc --noEmit"',
            "      - run: |",
            "          # vitest run --coverage",
            "          echo ok # slophammer typescript dry .",
            "      - name: mutation note",
            "        env:",
            "          MUTATE: stryker run"
          ].join("\n")
        },
        { path: "package.json", content: JSON.stringify({ scripts: {} }) },
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining([
        "ts.typecheck-required",
        "ts.lint-required",
        "ts.coverage-required",
        "ts.dry-required",
        "ts.mutation-required"
      ])
    );
  });
});

describe("TypeScript config evidence", () => {
  it("ignores fixture ESLint configs", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts(),
        {
          path: "fixtures/repos/example/eslint.config.mjs",
          content: enabledESLintConfig().content
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toEqual(
      expect.arrayContaining(["ts.no-explicit-any", "ts.no-unsafe-types", "ts.complexity-required"])
    );
  });
});

describe("TypeScript coverage and config inheritance", () => {
  it("does not treat slophammer config as coverage enforcement", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage" }),
        enabledESLintConfig()
      ]),
      {
        ...emptyConfig(),
        typescript: {
          ...emptyConfig().typescript,
          coverage: { threshold: 85, paths: [], exclude: [] }
        }
      }
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("accepts coverage threshold in the test runner config", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage" }),
        enabledESLintConfig(),
        coverageConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("accepts quoted coverage threshold keys in the test runner config", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage" }),
        enabledESLintConfig(),
        {
          path: "vitest.config.ts",
          content:
            'export default {test:{coverage:{thresholds:{"lines":85,"functions":85,"branches":85,"statements":85}}}};\n'
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("requires a runnable coverage command with config thresholds", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "echo no coverage run" }),
        enabledESLintConfig(),
        coverageConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });
});

describe("TypeScript coverage thresholds", () => {
  it("honors stricter configured coverage thresholds", () => {
    const weak = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage:
            "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85"
        }),
        enabledESLintConfig()
      ]),
      configWithCoverageThreshold(90)
    );
    const strict = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage:
            "vitest run --coverage --coverage.thresholds.lines=90 --coverage.thresholds.functions=90 --coverage.thresholds.branches=90 --coverage.thresholds.statements=90"
        }),
        enabledESLintConfig()
      ]),
      configWithCoverageThreshold(90)
    );

    expect(weak.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
    expect(strict.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("does not accept unrelated large numbers as coverage thresholds", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage: "vitest run --coverage --threshold 80 --timeout 100000"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("does not accept unsupported Vitest threshold flags", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage --threshold 85" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });
});

describe("TypeScript coverage tool thresholds", () => {
  it("does not accept disabled coverage flags", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage:
            "vitest run --coverage=false --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("accepts nyc and c8 check-coverage thresholds", () => {
    const nyc = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage:
            "nyc --check-coverage --lines 85 --functions 85 --branches 85 --statements 85 mocha"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );
    const c8 = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage:
            "c8 --check-coverage --lines=85 --functions=85 --branches=85 --statements=85 node --test"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(nyc.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
    expect(c8.findings.map((finding) => finding.rule_id)).not.toContain("ts.coverage-required");
  });

  it("requires all metric-specific coverage thresholds", () => {
    const weak = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage --coverage.thresholds.lines=85" }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );
    const complete = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage:
            "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(weak.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
    expect(complete.findings.map((finding) => finding.rule_id)).not.toContain(
      "ts.coverage-required"
    );
  });

  it("does not accept thresholds from unrelated package scripts", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({
          coverage: "vitest run --coverage",
          unrelated: "custom-tool --threshold 85"
        }),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });
});

describe("TypeScript config inheritance", () => {
  it("requires all coverage metrics in test runner config", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage" }),
        enabledESLintConfig(),
        {
          path: "vitest.config.ts",
          content: "export default {test:{coverage:{thresholds:{branches:85}}}};\n"
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("ignores coverage thresholds in config comments", () => {
    const report = runRules(
      newSnapshot("/repo", [
        ...baseTypeScriptFiles(),
        packageWithScripts({ coverage: "vitest run --coverage" }),
        enabledESLintConfig(),
        {
          path: "vitest.config.ts",
          content:
            "export default {test:{coverage:{// thresholds:{lines:85,functions:85,branches:85,statements:85}\n}}};\n"
        }
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).toContain("ts.coverage-required");
  });

  it("accepts strict options inherited through tsconfig extends", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" },
        { path: "tsconfig.json", content: JSON.stringify({ extends: "./tsconfig.base.json" }) },
        {
          path: "tsconfig.base.json",
          content:
            baseTypeScriptFiles().find((file) => file.path === "tsconfig.json")?.content ?? ""
        },
        packageWithScripts(),
        enabledESLintConfig()
      ]),
      emptyConfig()
    );

    expect(report.findings.map((finding) => finding.rule_id)).not.toContain("ts.strict-required");
  });
});

function baseTypeScriptFiles(): readonly { readonly path: string; readonly content: string }[] {
  return [
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
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
    dry: "slophammer-ts dry .",
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

function coverageConfig(): { readonly path: string; readonly content: string } {
  return {
    path: "vitest.config.ts",
    content:
      "export default {test:{coverage:{thresholds:{lines:85,functions:85,branches:85,statements:85}}}};\n"
  };
}

function configWithCoverageThreshold(threshold: number): ReturnType<typeof emptyConfig> {
  return {
    ...emptyConfig(),
    typescript: {
      ...emptyConfig().typescript,
      coverage: { threshold, paths: [], exclude: [] }
    }
  };
}
