import { describe, expect, it } from "vitest";

import { emptyConfig } from "../src/config/config.js";
import { newSnapshot, type Snapshot } from "../src/repo/repo.js";
import { RulePolicy } from "../src/rules/policy.js";
import { runRules } from "../src/rules/rules.js";
import type { Finding } from "../src/rules/types.js";

const ruleID = "ts.suppressions-justified";

describe("ts.suppressions-justified", () => {
  it("flags bare eslint-disable directives with the offending line", () => {
    const findings = suppressionFindings("// eslint-disable-next-line no-console\nrun();\n");

    expect(findings).toHaveLength(1);
    expect(findings[0]?.path).toBe("src/index.ts");
    expect(findings[0]?.message).toContain("(line 1)");
  });

  it("accepts eslint and oxlint directives with a -- description", () => {
    const content = [
      "// eslint-disable-next-line no-console -- CLI entry point logs by design",
      "console.log(1);",
      "/* eslint-disable complexity -- parser table, regenerated */",
      "// oxlint-disable-next-line no-console -- CLI output",
      "console.log(2);",
      ""
    ].join("\n");

    expect(suppressionFindings(content)).toEqual([]);
  });

  it("flags bare oxlint-disable directives", () => {
    const findings = suppressionFindings("export const a = 1;\n// oxlint-disable no-console\n");

    expect(findings[0]?.message).toContain("(line 2)");
  });

  it("flags @ts-ignore even with trailing text", () => {
    const findings = suppressionFindings("// @ts-ignore this never helps\nbroken();\n");

    expect(findings).toHaveLength(1);
  });

  it("accepts @ts-expect-error with a description and flags it bare", () => {
    expect(suppressionFindings("// @ts-expect-error untyped legacy global\nuse();\n")).toEqual([]);
    expect(suppressionFindings("// @ts-expect-error\nuse();\n")).toHaveLength(1);
    expect(suppressionFindings("/* @ts-expect-error */\nuse();\n")).toHaveLength(1);
  });

  it("accepts biome-ignore with an explanation and flags it without one", () => {
    expect(
      suppressionFindings("// biome-ignore lint/suspicious/noExplicitAny: boundary input\n")
    ).toEqual([]);
    expect(suppressionFindings("// biome-ignore lint/suspicious/noExplicitAny\n")).toHaveLength(1);
  });

  it("accepts directives under a preceding explanatory comment", () => {
    const content = [
      "// the sdk ships no types for this hook",
      "// @ts-ignore",
      "register();",
      ""
    ].join("\n");

    expect(suppressionFindings(content)).toEqual([]);
  });

  it("does not let a bare directive justify the next directive", () => {
    const content = "// @ts-ignore\n// eslint-disable-next-line no-console\nconsole.log(1);\n";

    expect(suppressionFindings(content)).toHaveLength(1);
  });

  it("reports only the first offense per file", () => {
    const content = "// @ts-ignore\nbad();\n// @ts-ignore\nworse();\n";
    const findings = suppressionFindings(content);

    expect(findings).toHaveLength(1);
    expect(findings[0]?.message).toContain("(line 1)");
  });

  it("ignores directive names outside comments", () => {
    const content = [
      'const markers = ["eslint-disable", "@ts-ignore", "biome-ignore"];',
      "export const description = `oxlint-disable and @ts-expect-error accumulate`;",
      ""
    ].join("\n");

    expect(suppressionFindings(content)).toEqual([]);
  });

  it("ignores comment markers inside string literals", () => {
    const content = [
      'const directive = "// @ts-ignore";',
      "const banner = `/* eslint-disable */`;",
      "const escaped = 'quote \\' then // @ts-ignore stays quoted';",
      'const flagged = "real"; // @ts-ignore',
      ""
    ].join("\n");

    const findings = suppressionFindings(content);

    expect(findings).toHaveLength(1);
    expect(findings[0]?.message).toContain("(line 4)");
  });

  it("exempts test files, declarations, and project data", () => {
    const report = runSuppressionRules(
      newSnapshot("/repo", [
        ...typeScriptProjectMarkers(),
        { path: "tests/app.test.ts", content: "// @ts-ignore\n" },
        { path: "src/util.spec.ts", content: "// eslint-disable no-console\n" },
        { path: "src/global.d.ts", content: "// @ts-ignore\n" },
        { path: "fixtures/sample.ts", content: "// @ts-ignore\n" },
        { path: "vitest.config.ts", content: "// eslint-disable no-console\n" }
      ])
    );

    expect(report.findings).toEqual([]);
  });
});

function suppressionFindings(content: string): readonly Finding[] {
  return runSuppressionRules(
    newSnapshot("/repo", [...typeScriptProjectMarkers(), { path: "src/index.ts", content }])
  ).findings;
}

function runSuppressionRules(snapshot: Snapshot): ReturnType<typeof runRules> {
  const config = emptyConfig();
  return runRules(snapshot, config, new RulePolicy(config.rules, [ruleID]));
}

function typeScriptProjectMarkers(): readonly {
  readonly path: string;
  readonly content: string;
}[] {
  return [{ path: "tsconfig.json", content: "{}" }];
}
