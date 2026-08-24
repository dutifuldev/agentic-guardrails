import { describe, expect, it } from "vitest";

import { emptyConfig, type Config } from "../src/config/config.js";
import { newSnapshot, type Snapshot } from "../src/repo/repo.js";
import { RulePolicy } from "../src/rules/policy.js";
import { runRules } from "../src/rules/rules.js";
import { scopeCounts } from "../src/rules/scope.js";

const ruleID = "ts.scope-incomplete";

describe("ts.scope-incomplete", () => {
  it("is silent when no scope paths are configured", () => {
    const report = runScope(snapshot("lib/extra.ts"), emptyConfig());

    expect(report.findings).toEqual([]);
    expect(scopeCounts(snapshot("lib/extra.ts"), emptyConfig())).toBeUndefined();
  });

  it("names uncovered production directories once, sorted", () => {
    const report = runScope(
      snapshot("lib/extra.ts", "lib/other.ts", "cmd/main.ts"),
      dryScopedConfig(["src"])
    );

    expect(report.findings).toHaveLength(1);
    expect(report.findings[0]?.path).toBe("slophammer.yml");
    expect(report.findings[0]?.message).toMatch(/: cmd, lib$/u);
  });

  it("accepts production files inside any configured scope", () => {
    const cfg = withCoveragePaths(dryScopedConfig(["src"]), ["lib"]);
    const report = runScope(snapshot("lib/extra.ts"), cfg);

    expect(report.findings).toEqual([]);
  });

  it("treats a dot scope as covering everything", () => {
    const report = runScope(snapshot("lib/extra.ts"), dryScopedConfig(["."]));

    expect(report.findings).toEqual([]);
  });

  it("accepts files matched by exclude patterns", () => {
    const cfg = withDryExclude(dryScopedConfig(["src"]), ["lib/vendored/**", "**/*.gen.ts", "cmd"]);
    const report = runScope(
      snapshot("lib/vendored/parser.ts", "lib/codec.gen.ts", "cmd/main.ts"),
      cfg
    );

    expect(report.findings).toEqual([]);
  });

  it("ignores conventional non-production paths", () => {
    const report = runScope(
      snapshot(
        "scripts/release.ts",
        "vendor/lib.ts",
        "tools/generated/api.ts",
        "tests/app.test.ts",
        "lib/util_test.ts",
        "lib/types.d.ts",
        "vitest.config.ts"
      ),
      dryScopedConfig(["src"])
    );

    expect(report.findings).toEqual([]);
  });

  it("counts scanned and production files for the report", () => {
    const counts = scopeCounts(
      snapshot("lib/extra.ts", "scripts/release.ts"),
      dryScopedConfig(["src"])
    );

    expect(counts).toEqual({ scanned: 1, production_files: 2 });
  });
});

function runScope(target: Snapshot, cfg: Config): ReturnType<typeof runRules> {
  return runRules(target, cfg, new RulePolicy(cfg.rules, [ruleID]));
}

function snapshot(...extraPaths: readonly string[]): Snapshot {
  return newSnapshot("/repo", [
    { path: "tsconfig.json", content: "{}" },
    { path: "src/index.ts", content: "export const value = 1;\n" },
    ...extraPaths.map((path) => ({ path, content: "export const value = 1;\n" }))
  ]);
}

function dryScopedConfig(paths: readonly string[]): Config {
  const base = emptyConfig();
  return {
    ...base,
    typescript: { ...base.typescript, dry: { ...base.typescript.dry, paths } }
  };
}

function withDryExclude(cfg: Config, exclude: readonly string[]): Config {
  return {
    ...cfg,
    typescript: { ...cfg.typescript, dry: { ...cfg.typescript.dry, exclude } }
  };
}

function withCoveragePaths(cfg: Config, paths: readonly string[]): Config {
  return {
    ...cfg,
    typescript: { ...cfg.typescript, coverage: { ...cfg.typescript.coverage, paths } }
  };
}
