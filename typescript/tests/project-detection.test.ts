import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { runRules } from "../src/rules/rules.js";

const agentInstructions =
  "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n";

describe("TypeScript project detection", () => {
  it("does not treat TypeScript-only tooling files as production TypeScript", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" },
        {
          path: "package.json",
          content: JSON.stringify({
            scripts: { test: "vitest run" },
            devDependencies: { vitest: "^3.0.0" }
          })
        },
        { path: "vitest.config.ts", content: "export default {};\n" },
        { path: "tests/example.test.ts", content: "export const testOnly = true;\n" },
        { path: "src/index.d.ts", content: "export declare const value: number;\n" },
        { path: "src/index.js", content: "export const value = 1;\n" }
      ]),
      emptyConfig()
    );

    expect(report.findings).toEqual([]);
  });

  it("evaluates TypeScript rules for each detected package root", () => {
    const report = runRules(
      newSnapshot("/repo", [
        { path: "README.md", content: "# Repo\n" },
        { path: "AGENTS.md", content: agentInstructions },
        { path: ".github/workflows/ci.yml", content: "name: ci\n" },
        {
          path: "packages/a/package.json",
          content: JSON.stringify({ scripts: packageScripts() })
        },
        {
          path: "packages/a/tsconfig.json",
          content: strictTSConfig()
        },
        {
          path: "packages/a/eslint.config.mjs",
          content: eslintConfig()
        },
        {
          path: "packages/a/vitest.config.ts",
          content: coverageConfig()
        },
        {
          path: "packages/b/src/index.ts",
          content: "export const value: number = 1;\n"
        }
      ]),
      {
        ...emptyConfig(),
        typescript: {
          ...emptyConfig().typescript,
          dependencyBoundaries: [{ from: "pkg/src", allow: [] }]
        }
      }
    );

    expect(report.findings).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          rule_id: "ts.package-required",
          path: "packages/b/package.json"
        }),
        expect.objectContaining({
          rule_id: "ts.strict-required",
          path: "packages/b/tsconfig.json"
        }),
        expect.objectContaining({
          rule_id: "ts.typecheck-required",
          path: "packages/b/.github/workflows"
        })
      ])
    );
  });

  it("preserves root evidence for nested TypeScript packages", () => {
    const report = runRules(newSnapshot("/repo", nestedPackageFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "pkg/src", allow: [] }]
      }
    });

    expect(report.findings).toEqual([]);
  });

  it("treats package-less TypeScript below the root as root-owned", () => {
    const report = runRules(newSnapshot("/repo", nestedPackageWithoutPackageFile()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "packages/app/src", allow: [] }]
      }
    });

    expect(report.findings).toEqual([
      expect.objectContaining({
        rule_id: "ts.strict-required",
        path: "tsconfig.json"
      })
    ]);
  });
});

describe("TypeScript package boundaries", () => {
  it("does not accept nested JavaScript package metadata as root TypeScript metadata", () => {
    const report = runRules(newSnapshot("/repo", rootTypeScriptWithNestedJavaScriptPackage()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "src", allow: [] }]
      }
    });

    expect(report.findings).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          rule_id: "ts.package-required",
          path: "package.json"
        }),
        expect.objectContaining({
          rule_id: "ts.typecheck-required",
          path: ".github/workflows"
        }),
        expect.objectContaining({
          rule_id: "ts.lint-required",
          path: ".github/workflows"
        })
      ])
    );
  });
});

describe("TypeScript project scoping", () => {
  it("does not report nested package config failures against the root project", () => {
    const report = runRules(newSnapshot("/repo", rootAndNestedPackageFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "src", allow: [] },
          { from: "packages/app/src", allow: [] }
        ]
      }
    });

    const strictPaths = report.findings
      .filter((finding) => finding.rule_id === "ts.strict-required")
      .map((finding) => finding.path);

    expect(strictPaths).toEqual(["packages/app/tsconfig.json"]);
  });

  it("uses root shared tsconfig bases for nested packages", () => {
    const report = runRules(newSnapshot("/repo", nestedPackageWithRootTSConfigBase()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "packages/app/src", allow: [] }]
      }
    });

    expect(report.findings).not.toContainEqual(
      expect.objectContaining({
        rule_id: "ts.strict-required"
      })
    );
  });

  it("does not leak root commands from one package to another", () => {
    const report = runRules(newSnapshot("/repo", multiPackageRootCommandFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "packages/a/src", allow: [] },
          { from: "packages/b/src", allow: [] }
        ]
      }
    });

    expect(report.findings).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          rule_id: "ts.typecheck-required",
          path: "packages/b/.github/workflows"
        }),
        expect.objectContaining({
          rule_id: "ts.lint-required",
          path: "packages/b/.github/workflows"
        })
      ])
    );
  });

  it("does not leak nested workflow or package-script commands into the root project", () => {
    const report = runRules(newSnapshot("/repo", rootProjectWithNestedOnlyCommands()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "src", allow: [] },
          { from: "packages/app/src", allow: [] }
        ]
      }
    });

    const rootWorkflowRules = report.findings
      .filter((finding) => finding.path === ".github/workflows")
      .map((finding) => finding.rule_id);

    expect(rootWorkflowRules).toEqual([
      "ts.coverage-required",
      "ts.dry-required",
      "ts.format-required",
      "ts.lint-required",
      "ts.mutation-required",
      "ts.test-required",
      "ts.typecheck-required"
    ]);
  });

  it("does not leak root workflow commands into a single nested package", () => {
    const report = runRules(newSnapshot("/repo", rootAndNestedWithRootOnlyCommands()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "src", allow: [] },
          { from: "packages/app/src", allow: [] }
        ]
      }
    });

    const nestedWorkflowRules = report.findings
      .filter((finding) => finding.path === "packages/app/.github/workflows")
      .map((finding) => finding.rule_id);

    expect(nestedWorkflowRules).toEqual([
      "ts.coverage-required",
      "ts.dry-required",
      "ts.format-required",
      "ts.lint-required",
      "ts.mutation-required",
      "ts.test-required",
      "ts.typecheck-required"
    ]);
  });
});

describe("TypeScript project scoping workflows", () => {
  it("does not leak mixed workflow block commands between packages", () => {
    const report = runRules(newSnapshot("/repo", mixedWorkflowBlockFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "packages/a/src", allow: [] },
          { from: "packages/b/src", allow: [] }
        ]
      }
    });

    const packageAWorkflowRules = report.findings
      .filter((finding) => finding.path === "packages/a/.github/workflows")
      .map((finding) => finding.rule_id);
    const packageBWorkflowRules = report.findings
      .filter((finding) => finding.path === "packages/b/.github/workflows")
      .map((finding) => finding.rule_id);

    expect(packageAWorkflowRules).toContain("ts.lint-required");
    expect(packageAWorkflowRules).not.toContain("ts.typecheck-required");
    expect(packageBWorkflowRules).toContain("ts.typecheck-required");
    expect(packageBWorkflowRules).not.toContain("ts.lint-required");
  });

  it("keeps workflow commands under matching working-directory defaults", () => {
    const report = runRules(newSnapshot("/repo", multiPackageDefaultWorkflowFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "packages/a/src", allow: [] },
          { from: "packages/b/src", allow: [] }
        ]
      }
    });

    expect(report.findings).toEqual([]);
  });

  it("keeps workflow commands under workflow-level working-directory defaults", () => {
    const report = runRules(newSnapshot("/repo", workflowDefaultPackageFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "packages/app/src", allow: [] },
          { from: "packages/lib/src", allow: [] }
        ]
      }
    });

    const appFindingPaths = report.findings
      .filter((finding) => finding.path.startsWith("packages/app/"))
      .map((finding) => finding.rule_id);

    expect(appFindingPaths).toEqual([]);
  });

  it("does not use prefix package workflow evidence for sibling packages", () => {
    const report = runRules(newSnapshot("/repo", prefixPackageWorkflowFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "packages/app/src", allow: [] },
          { from: "packages/app2/src", allow: [] }
        ]
      }
    });

    const typecheckPaths = report.findings
      .filter((finding) => finding.rule_id === "ts.typecheck-required")
      .map((finding) => finding.path);

    expect(typecheckPaths).toEqual(["packages/app/.github/workflows"]);
  });

  it("uses the nearest package marker before a deeper src directory", () => {
    const report = runRules(newSnapshot("/repo", nestedSourcePackageFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "packages/app/functions/src", allow: [] }]
      }
    });

    expect(report.findings).toEqual([]);
  });

  it("keeps workflow matrix commands under matching package scopes", () => {
    const report = runRules(newSnapshot("/repo", matrixCommandPackageFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [
          { from: "packages/app/src", allow: [] },
          { from: "packages/lib/src", allow: [] }
        ]
      }
    });

    expect(report.findings).toEqual([]);
  });

  it("preserves workflow matrix command wrappers while scoping packages", () => {
    const report = runRules(newSnapshot("/repo", matrixWrapperPackageFiles()), {
      ...emptyConfig(),
      typescript: {
        ...emptyConfig().typescript,
        dependencyBoundaries: [{ from: "packages/app/src", allow: [] }]
      }
    });

    expect(report.findings).toEqual([]);
  });
});

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

function nestedPackageFiles(): readonly { readonly path: string; readonly content: string }[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: nestedPackageWorkflow() },
    {
      path: "pkg/package.json",
      content: JSON.stringify({ devDependencies: { typescript: "^5.0.0" } })
    },
    { path: "pkg/tsconfig.json", content: strictTSConfig() },
    { path: "pkg/stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "pkg/eslint.config.mjs", content: eslintConfig() },
    { path: "pkg/vitest.config.ts", content: coverageConfig() }
  ];
}

function nestedPackageWithoutPackageFile(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: singlePackageDefaultWorkflow("packages/app") },
    { path: "package.json", content: JSON.stringify({ scripts: { test: "echo ok" } }) },
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "packages/app/src/index.ts", content: "export const value: number = 1;\n" },
    { path: "packages/app/tsconfig.json", content: strictTSConfig() },
    { path: "packages/app/eslint.config.mjs", content: eslintConfig() },
    { path: "packages/app/vitest.config.ts", content: coverageConfig() }
  ];
}

function multiPackageRootCommandFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: packageAWorkflow() },
    ...packageFiles("packages/a"),
    ...packageFiles("packages/b")
  ];
}

function mixedWorkflowBlockFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: mixedWorkflowBlock() },
    ...packageFiles("packages/a"),
    ...packageFiles("packages/b")
  ];
}

function nestedSourcePackageFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: nestedPackageWorkflow() },
    ...packageFiles("packages/app"),
    {
      path: "packages/app/functions/src/handler.ts",
      content: "export const handler = () => 1;\n"
    }
  ];
}

function multiPackageDefaultWorkflowFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: packageDefaultWorkflow() },
    ...packageFiles("packages/a"),
    ...packageFiles("packages/b")
  ];
}

function workflowDefaultPackageFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: workflowDefaultWorkflow("packages/app") },
    ...packageFiles("packages/app"),
    ...packageFiles("packages/lib")
  ];
}

function rootAndNestedPackageFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: "name: CI\n" },
    { path: "package.json", content: JSON.stringify({ scripts: packageScripts() }) },
    { path: "tsconfig.json", content: strictTSConfig() },
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "eslint.config.mjs", content: eslintConfig() },
    { path: "vitest.config.ts", content: coverageConfig() },
    ...packageFilesWithTSConfig("packages/app", weakTSConfig())
  ];
}

function rootProjectWithNestedOnlyCommands(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: singlePackageDefaultWorkflow("packages/app") },
    {
      path: "package.json",
      content: JSON.stringify({
        scripts: {
          check:
            "cd packages/app && prettier --check . && eslint . && tsc --noEmit && vitest run && vitest run --coverage && slophammer typescript dry . && stryker run"
        },
        devDependencies: { typescript: "^5.0.0" }
      })
    },
    { path: "src/index.ts", content: "export const value: number = 1;\n" },
    { path: "tsconfig.json", content: strictTSConfig() },
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "eslint.config.mjs", content: eslintConfig() },
    { path: "vitest.config.ts", content: coverageConfig() },
    ...packageFiles("packages/app")
  ];
}

function rootAndNestedWithRootOnlyCommands(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: singlePackageDefaultWorkflow(".") },
    { path: "package.json", content: JSON.stringify({ scripts: packageScripts() }) },
    { path: "src/index.ts", content: "export const value: number = 1;\n" },
    { path: "tsconfig.json", content: strictTSConfig() },
    { path: "stryker.conf.json", content: '{"thresholds":{"high":70,"low":50,"break":50}}' },
    { path: "eslint.config.mjs", content: eslintConfig() },
    { path: "vitest.config.ts", content: coverageConfig() },
    ...packageFiles("packages/app")
  ];
}

function prefixPackageWorkflowFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: singlePackageDefaultWorkflow("packages/app2") },
    ...packageFiles("packages/app"),
    ...packageFiles("packages/app2")
  ];
}

function matrixCommandPackageFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: matrixCommandWorkflow() },
    ...packageFiles("packages/app"),
    ...packageFiles("packages/lib")
  ];
}

function matrixWrapperPackageFiles(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: matrixWrapperWorkflow() },
    ...packageFiles("packages/app")
  ];
}

function nestedPackageWithRootTSConfigBase(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: "name: CI\n" },
    { path: "tsconfig.base.json", content: strictTSConfig() },
    {
      path: "packages/app/package.json",
      content: JSON.stringify({ scripts: packageScripts() })
    },
    {
      path: "packages/app/tsconfig.json",
      content: JSON.stringify({ extends: "../../tsconfig.base.json" })
    },
    { path: "packages/app/eslint.config.mjs", content: eslintConfig() },
    { path: "packages/app/vitest.config.ts", content: coverageConfig() }
  ];
}

function rootTypeScriptWithNestedJavaScriptPackage(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [
    { path: "README.md", content: "# Repo\n" },
    { path: "AGENTS.md", content: agentInstructions },
    { path: ".github/workflows/ci.yml", content: "name: CI\n" },
    { path: "src/index.ts", content: "export const value: number = 1;\n" },
    {
      path: "examples/js/package.json",
      content: JSON.stringify({ scripts: packageScripts() })
    }
  ];
}

function packageFiles(
  root: string
): readonly { readonly path: string; readonly content: string }[] {
  return packageFilesWithTSConfig(root, strictTSConfig());
}

function packageFilesWithTSConfig(
  root: string,
  tsconfig: string
): readonly { readonly path: string; readonly content: string }[] {
  return [
    {
      path: `${root}/package.json`,
      content: JSON.stringify({ devDependencies: { typescript: "^5.0.0" } })
    },
    {
      path: `${root}/stryker.conf.json`,
      content: '{"thresholds":{"high":70,"low":50,"break":50}}'
    },
    { path: `${root}/src/index.ts`, content: "export const value: number = 1;\n" },
    { path: `${root}/tsconfig.json`, content: tsconfig },
    { path: `${root}/eslint.config.mjs`, content: eslintConfig() },
    { path: `${root}/vitest.config.ts`, content: coverageConfig() }
  ];
}

function packageAWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  a:",
    "    steps:",
    "      - run: cd packages/a && prettier --check .",
    "      - run: cd packages/a && eslint .",
    "      - run: cd packages/a && tsc --noEmit",
    "      - run: cd packages/a && vitest run",
    "      - run: cd packages/a && vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "      - run: cd packages/a && slophammer typescript dry .",
    "      - run: cd packages/a && stryker run"
  ].join("\n");
}

function mixedWorkflowBlock(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  mixed:",
    "    steps:",
    "      - run: |",
    "          cd packages/a && tsc --noEmit",
    "          cd packages/b && eslint ."
  ].join("\n");
}

function packageDefaultWorkflow(): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  a:",
    "    defaults:",
    "      run:",
    "        working-directory: packages/a",
    "    steps:",
    "      - run: prettier --check .",
    "      - run: eslint .",
    "      - run: tsc --noEmit",
    "      - run: vitest run",
    "      - run: vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "      - run: slophammer typescript dry .",
    "      - run: stryker run",
    "  b:",
    "    defaults:",
    "      run:",
    "        working-directory: packages/b",
    "    steps:",
    "      - run: prettier --check .",
    "      - run: eslint .",
    "      - run: tsc --noEmit",
    "      - run: vitest run",
    "      - run: vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "      - run: slophammer typescript dry .",
    "      - run: stryker run"
  ].join("\n");
}

function singlePackageDefaultWorkflow(root: string): string {
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  package:",
    "    defaults:",
    "      run:",
    `        working-directory: ${root}`,
    "    steps:",
    "      - run: prettier --check .",
    "      - run: eslint .",
    "      - run: tsc --noEmit",
    "      - run: vitest run",
    "      - run: vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "      - run: slophammer typescript dry .",
    "      - run: stryker run"
  ].join("\n");
}

function workflowDefaultWorkflow(root: string): string {
  return [
    "name: CI",
    "on: [push]",
    "defaults:",
    "  run:",
    `    working-directory: ${root}`,
    "jobs:",
    "  package:",
    "    steps:",
    "      - run: prettier --check .",
    "      - run: eslint .",
    "      - run: tsc --noEmit",
    "      - run: vitest run",
    "      - run: vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "      - run: slophammer typescript dry .",
    "      - run: stryker run"
  ].join("\n");
}

function matrixCommandWorkflow(): string {
  const commands = [
    "prettier --check .",
    "eslint .",
    "tsc --noEmit",
    "vitest run",
    "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "slophammer typescript dry .",
    "stryker run"
  ];
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  checks:",
    "    strategy:",
    "      matrix:",
    "        include:",
    ...["packages/app", "packages/lib"].flatMap((root) =>
      commands.map((command) => `          - command: cd ${root} && ${command}`)
    ),
    "    steps:",
    "      - run: ${{ matrix.command }}"
  ].join("\n");
}

function matrixWrapperWorkflow(): string {
  const commands = [
    "prettier --check .",
    "eslint .",
    "tsc --noEmit",
    "vitest run",
    "vitest run --coverage --coverage.thresholds.lines=85 --coverage.thresholds.functions=85 --coverage.thresholds.branches=85 --coverage.thresholds.statements=85",
    "slophammer typescript dry .",
    "stryker run"
  ];
  return [
    "name: CI",
    "on: [push]",
    "jobs:",
    "  checks:",
    "    strategy:",
    "      matrix:",
    "        command:",
    ...commands.map((command) => `          - ${command}`),
    "    steps:",
    "      - run: cd packages/app && ${{matrix.command}}"
  ].join("\n");
}

function nestedPackageWorkflow(): string {
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
    "      - run: eslint .",
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

function weakTSConfig(): string {
  return JSON.stringify({ compilerOptions: { strict: false } });
}

function eslintConfig(): string {
  return [
    'export default [{rules:{"@typescript-eslint/no-explicit-any":"error",',
    '"@typescript-eslint/no-unsafe-assignment":"error",',
    '"@typescript-eslint/no-unsafe-call":"error",',
    '"@typescript-eslint/no-unsafe-member-access":"error",',
    '"@typescript-eslint/no-unsafe-return":"error",complexity:["error",8]}}];'
  ].join("");
}

function coverageConfig(): string {
  return "export default {test:{coverage:{thresholds:{lines:85,functions:85,branches:85,statements:85}}}};\n";
}
