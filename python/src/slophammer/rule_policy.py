"""Rule selection, disablement, and severity for one check."""

from __future__ import annotations

from dataclasses import dataclass, replace

from slophammer.config import Config
from slophammer.core import Finding, Severity


@dataclass(frozen=True)
class RulePolicy:
    """Own the effective rule policy for one check invocation."""

    config: Config
    selected: frozenset[str] | None
    honor_disabled: bool = True

    @classmethod
    def for_check(
        cls,
        config: Config,
        only_rule_ids: list[str] | None = None,
    ) -> RulePolicy:
        selected = None if not only_rule_ids else frozenset(only_rule_ids)
        return cls(config=config, selected=selected)

    @classmethod
    def for_direct(cls, config: Config, rule_ids: list[str]) -> RulePolicy:
        return cls(config=config, selected=frozenset(rule_ids), honor_disabled=False)

    def active(self, rule_id: str) -> bool:
        selected = self.selected is None or rule_id in self.selected
        rule = self.config.rules.get(rule_id)
        enabled = not self.honor_disabled or rule is None or not rule.disabled
        return selected and enabled

    def severity(self, rule_id: str, default: Severity) -> Severity:
        rule = self.config.rules.get(rule_id)
        if rule is not None and rule.severity in ("error", "warn"):
            return rule.severity  # type: ignore[return-value]  # ty: ignore[invalid-return-type] -- narrowed above
        return default

    def admit(self, finding: Finding) -> Finding | None:
        if not self.active(finding.rule_id):
            return None
        return replace(finding, severity=self.severity(finding.rule_id, finding.severity))

    def admit_all(self, findings: list[Finding]) -> list[Finding]:
        return [admitted for finding in findings if (admitted := self.admit(finding)) is not None]
