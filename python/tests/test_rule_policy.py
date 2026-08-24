"""Rule-policy truth table and static admission tests."""

from slophammer.config import Config, RuleConfig, parse_config
from slophammer.core import Finding
from slophammer.repo import new_snapshot
from slophammer.rule_policy import RulePolicy
from slophammer.rules import run_rules

README_RULE = "repo.readme-required"


def test_absent_and_false_disablement_stay_active() -> None:
    assert RulePolicy.for_check(Config()).active(README_RULE)
    config = Config(rules={README_RULE: RuleConfig(disabled=False)})
    assert RulePolicy.for_check(config).active(README_RULE)


def test_selection_intersects_with_disablement() -> None:
    config = Config(rules={README_RULE: RuleConfig(disabled=True, reason="not required here")})
    policy = RulePolicy.for_check(config, [README_RULE])
    assert not policy.active(README_RULE)
    assert not policy.active("repo.agents-required")


def test_admission_drops_disabled_and_applies_enabled_severity() -> None:
    config = Config(
        rules={
            README_RULE: RuleConfig(disabled=True, reason="not required here", severity="warn"),
            "repo.agents-required": RuleConfig(severity="warn"),
        }
    )
    findings = [finding(README_RULE), finding("repo.agents-required")]

    admitted = RulePolicy.for_check(config).admit_all(findings)

    assert [item.rule_id for item in admitted] == ["repo.agents-required"]
    assert admitted[0].severity == "warn"


def test_disabled_static_rule_is_not_evaluated(monkeypatch) -> None:
    config = parse_config(
        "rules:\n  repo.readme-required:\n    disabled: true\n    reason: not required here\n"
    )
    policy = RulePolicy.for_check(config, [README_RULE])

    def fail_if_called(*_args: object) -> list[Finding]:
        raise AssertionError("disabled evaluator ran")

    monkeypatch.setattr("slophammer.rules.check_definition", fail_if_called)
    report = run_rules(new_snapshot("/repo", []), config, policy)

    assert report.ok
    assert report.findings == []


def finding(rule_id: str) -> Finding:
    return Finding(
        rule_id=rule_id,
        severity="error",
        path="README.md",
        message="missing",
    )
