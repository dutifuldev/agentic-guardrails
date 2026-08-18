import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { check } from "../src/app/app.js";
import { applyBaselineCheck, baselineFileName, writeBaseline } from "../src/app/baseline.js";
import { newReport } from "../src/report/report.js";
import type { Finding } from "../src/rules/types.js";
import { parseReport } from "./helpers.js";

describe("check --baseline", () => {
  it("fails with exit 2 when the baseline file is missing", async () => {
    const root = await emptyRepo();

    const result = await check({ root, format: "text", execute: false, baseline: "check" });

    expect(result.code).toBe(2);
    expect(result.stderr).toContain(`baseline file ${baselineFileName} is missing`);
  });

  it("fails with exit 2 on invalid baseline content", async () => {
    const root = await emptyRepo();
    await writeFile(path.join(root, baselineFileName), "not json\n");

    const result = await check({ root, format: "text", execute: false, baseline: "check" });

    expect(result.code).toBe(2);
    expect(result.stderr).toContain("baseline parse failed");
  });

  it("fails with exit 2 on unsupported baseline versions and unknown keys", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, { version: 2, findings: [] });
    const versioned = await check({ root, format: "text", execute: false, baseline: "check" });
    expect(versioned.code).toBe(2);
    expect(versioned.stderr).toContain("baseline version must be 1");

    await writeBaselineFile(root, { version: 1, findings: [], extra: true });
    const unknownKey = await check({ root, format: "text", execute: false, baseline: "check" });
    expect(unknownKey.code).toBe(2);
    expect(unknownKey.stderr).toContain("baseline parse failed");
  });

  it("passes when every finding is baselined and reports the debt", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, { version: 1, findings: emptyRepoEntries() });

    const result = await check({ root, format: "text", execute: false, baseline: "check" });

    expect(result.code).toBe(0);
    expect(result.stdout).toContain("3 findings baselined; 0 new\n");
  });

  it("keeps failing on findings outside the baseline", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, {
      version: 1,
      findings: [{ rule_id: "repo.readme-required", path: "README.md" }]
    });
    await writeFile(
      path.join(root, "AGENTS.md"),
      "# Agents\n\nRun `npm run check`, `npm test`, or `npm run build` before finishing.\n"
    );

    const result = await check({ root, format: "json", execute: false, baseline: "check" });

    expect(result.code).toBe(1);
    expect(parseReport(result.stdout).ok).toBe(false);
    const parsed = JSON.parse(result.stdout) as {
      readonly findings: readonly { readonly rule_id: string; readonly baselined?: boolean }[];
    };
    expect(parsed.findings.find((item) => item.rule_id === "repo.readme-required")?.baselined).toBe(
      true
    );
    expect(
      parsed.findings.find((item) => item.rule_id === "repo.ci-required")?.baselined
    ).toBeUndefined();
  });

  it("rejects stale baselines with exit 2", async () => {
    const root = await emptyRepo();
    await writeFile(path.join(root, "README.md"), "# Repo\n");
    await writeBaselineFile(root, { version: 1, findings: emptyRepoEntries() });

    const result = await check({ root, format: "text", execute: false, baseline: "check" });

    expect(result.code).toBe(2);
    expect(result.stderr).toContain("baseline contains resolved findings; rewrite it:");
    expect(result.stderr).toContain("repo.readme-required at README.md");
  });

  it("marks baselined results as suppressed in SARIF", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, { version: 1, findings: emptyRepoEntries() });

    const result = await check({ root, format: "sarif", execute: false, baseline: "check" });
    const sarif = JSON.parse(result.stdout) as {
      readonly runs: readonly [{ readonly results: readonly { suppressions?: unknown }[] }];
    };

    expect(result.code).toBe(0);
    expect(sarif.runs[0].results[0]?.suppressions).toEqual([{ kind: "external" }]);
  });
});

describe("check --baseline-write", () => {
  it("writes sorted unique findings with a trailing newline", async () => {
    const root = await emptyRepo();

    const result = await check({ root, format: "text", execute: false, baseline: "write" });

    expect(result.code).toBe(0);
    expect(result.stdout).toContain("baseline written: 3 finding(s)\n");
    expect(result.stdout).toContain("added: repo.agents-required at AGENTS.md");
    const content = await readFile(path.join(root, baselineFileName), "utf8");
    expect(content.endsWith("\n")).toBe(true);
    expect(JSON.parse(content)).toEqual({ version: 1, findings: emptyRepoEntries() });
  });

  it("refuses to grow an existing baseline", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, {
      version: 1,
      findings: [{ rule_id: "repo.readme-required", path: "README.md" }]
    });

    const result = await check({ root, format: "text", execute: false, baseline: "write" });

    expect(result.code).toBe(2);
    expect(result.stderr).toContain("grow the baseline");
    expect(result.stderr).toContain("repo.agents-required at AGENTS.md");
  });

  it("refuses to replace a malformed existing baseline", async () => {
    const root = await emptyRepo();
    await writeFile(path.join(root, baselineFileName), "not json\n");

    const result = await check({ root, format: "text", execute: false, baseline: "write" });

    expect(result.code).toBe(2);
    expect(result.stderr).toContain("baseline parse failed");
  });

  it("records removals when the baseline shrinks", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, {
      version: 1,
      findings: [...emptyRepoEntries(), { rule_id: "repo.readme-required", path: "old/README.md" }]
    });

    const result = await check({ root, format: "text", execute: false, baseline: "write" });

    expect(result.code).toBe(0);
    expect(result.stdout).toContain("removed: repo.readme-required at old/README.md");
  });
});

describe("baseline module", () => {
  it("deduplicates matching entries on write", async () => {
    const root = await emptyRepo();
    const report = newReport([finding("a.rule", "x"), finding("a.rule", "x")]);

    const summary = await writeBaseline(root, report);

    expect(summary).toContain("baseline written: 1 finding(s)");
  });

  it("preserves report extras when applying a baseline", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, { version: 1, findings: [{ rule_id: "a.rule", path: "x" }] });
    const report = { ...newReport([finding("a.rule", "x")]), scope: scopeBlock() };

    const checked = await applyBaselineCheck(root, report);

    expect(checked.ok).toBe(true);
    expect(checked.scope).toEqual(scopeBlock());
    expect(checked.findings[0]?.baselined).toBe(true);
  });

  it("rejects baselines whose entries are not strings", async () => {
    const root = await emptyRepo();
    await writeBaselineFile(root, { version: 1, findings: [{ rule_id: 1, path: "x" }] });

    await expect(applyBaselineCheck(root, newReport([]))).rejects.toThrow(
      "rule_id and path strings"
    );
  });
});

function finding(ruleID: string, filePath: string): Finding {
  return { rule_id: ruleID, severity: "error", path: filePath, message: "missing" };
}

function scopeBlock(): { readonly scanned: number; readonly production_files: number } {
  return { scanned: 1, production_files: 2 };
}

async function emptyRepo(): Promise<string> {
  return await mkdtemp(path.join(tmpdir(), "slophammer-baseline-"));
}

async function writeBaselineFile(root: string, content: unknown): Promise<void> {
  await writeFile(path.join(root, baselineFileName), `${JSON.stringify(content)}\n`);
}

function emptyRepoEntries(): readonly { readonly rule_id: string; readonly path: string }[] {
  return [
    { rule_id: "repo.agents-required", path: "AGENTS.md" },
    { rule_id: "repo.ci-required", path: ".github/workflows" },
    { rule_id: "repo.readme-required", path: "README.md" }
  ];
}
