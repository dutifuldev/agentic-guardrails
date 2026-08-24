import type { Config } from "../config/config.js";
import type { Finding, Severity } from "./types.js";

/** Owns rule selection, disablement, and effective severity for one check. */
export class RulePolicy {
  private readonly selected: ReadonlySet<string> | undefined;

  public constructor(
    private readonly rules: Config["rules"],
    onlyRuleIDs: readonly string[] = [],
    private readonly honorDisabled = true
  ) {
    this.selected = onlyRuleIDs.length === 0 ? undefined : new Set(onlyRuleIDs);
  }

  public active(ruleID: string): boolean {
    const selected = this.selected === undefined || this.selected.has(ruleID);
    const enabled = !this.honorDisabled || this.rules.get(ruleID)?.disabled !== true;
    return selected && enabled;
  }

  public anyActive(ruleIDs: ReadonlySet<string>): boolean {
    return [...ruleIDs].some((ruleID) => this.active(ruleID));
  }

  public severity(ruleID: string, fallback: Severity): Severity {
    return this.rules.get(ruleID)?.severity ?? fallback;
  }

  public admit(finding: Finding): Finding | undefined {
    if (!this.active(finding.rule_id)) {
      return undefined;
    }
    return {
      ...finding,
      severity: this.severity(finding.rule_id, finding.severity)
    };
  }

  public admitAll(findings: readonly Finding[]): readonly Finding[] {
    return findings.flatMap((finding) => {
      const admitted = this.admit(finding);
      return admitted === undefined ? [] : [admitted];
    });
  }
}
