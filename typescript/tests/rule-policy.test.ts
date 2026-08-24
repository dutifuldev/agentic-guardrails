import { describe, expect, it } from "vitest";

import { emptyConfig, loadConfig, type Config, type RuleConfig } from "../src/config/config.js";
import { newSnapshot } from "../src/repo/repo.js";
import { RulePolicy } from "../src/rules/policy.js";
import { runRules } from "../src/rules/rules.js";
import type { Finding } from "../src/rules/types.js";

const readmeRule = "repo.readme-required";

describe("RulePolicy", () => {
  it("keeps absent and false disablement active", () => {
    expect(new RulePolicy(new Map()).active(readmeRule)).toBe(true);
    expect(
      new RulePolicy(ruleConfigs([[readmeRule, { disabled: false }]])).active(readmeRule)
    ).toBe(true);
  });

  it("intersects selection with enabled rules", () => {
    const policy = new RulePolicy(
      ruleConfigs([[readmeRule, { disabled: true, reason: "not required here" }]]),
      [readmeRule]
    );

    expect(policy.active(readmeRule)).toBe(false);
    expect(policy.active("repo.agents-required")).toBe(false);
  });

  it("drops disabled findings and applies enabled severity", () => {
    const rules = ruleConfigs([
      [readmeRule, { disabled: true, reason: "not required here", severity: "warn" }],
      ["repo.agents-required", { severity: "warn" }]
    ]);
    const policy = new RulePolicy(rules);
    const admitted = policy.admitAll([finding(readmeRule), finding("repo.agents-required")]);

    expect(admitted).toEqual([
      expect.objectContaining({ rule_id: "repo.agents-required", severity: "warn" })
    ]);
  });

  it("does not evaluate a disabled static rule", () => {
    const config = configWithRules(
      ruleConfigs([[readmeRule, { disabled: true, reason: "not required here" }]])
    );
    const report = runRules(
      newSnapshot("/repo", []),
      config,
      new RulePolicy(config.rules, [readmeRule])
    );

    expect(report).toEqual({ ok: true, findings: [] });
  });
});

describe("rule policy config", () => {
  it("rejects non-boolean disablement", () => {
    const snapshot = newSnapshot("/repo", [
      {
        path: "slophammer.yml",
        content: 'rules:\n  repo.readme-required:\n    disabled: "true"\n'
      }
    ]);

    expect(() => loadConfig(snapshot)).toThrow(
      "rules.repo.readme-required.disabled must be a boolean"
    );
  });

  it("rejects whitespace-only reasons", () => {
    const snapshot = newSnapshot("/repo", [
      {
        path: "slophammer.yml",
        content: 'rules:\n  repo.readme-required:\n    disabled: true\n    reason: " "\n'
      }
    ]);

    expect(() => loadConfig(snapshot)).toThrow("reason is required");
  });
});

function ruleConfigs(
  entries: readonly (readonly [string, RuleConfig])[]
): ReadonlyMap<string, RuleConfig> {
  return new Map(entries);
}

function configWithRules(rules: Config["rules"]): Config {
  return { ...emptyConfig(), rules };
}

function finding(ruleID: string): Finding {
  return {
    rule_id: ruleID,
    severity: "error",
    path: "README.md",
    message: "missing"
  };
}
