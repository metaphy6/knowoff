#!/usr/bin/env python3
"""Bounded offline Business Plan arithmetic; inputs are not verified evidence."""
import argparse
from datetime import date
from decimal import Decimal, ROUND_HALF_EVEN, localcontext
import hashlib
import json
import os
import re
import stat
import sys


MAX_INPUT_BYTES = 64 * 1024
MONTHLY_FIELDS = """active_humans player_matches_per_active premium_share
net_subscription_per_payer_month net_bulk_per_active verified_ads_per_nonpremium_active
net_per_thousand_ads runtime_per_player_match content_hours content_hourly_cost
content_providers support_hours support_hourly_cost hosting_backups_monitoring
acquisition_expense attributable_acquisition_spend new_activated_humans""".split()


class InputError(ValueError):
    pass


def require(condition):
    if not condition:
        raise InputError("invalid economics input")


def fields(value, expected):
    require(type(value) is dict and set(value) == set(expected))


def number(value):
    require(type(value) is str and re.fullmatch(r"(?:0|[1-9][0-9]{0,11})(?:\.[0-9]{1,6})?", value) is not None)
    return Decimal(value)


def count(value):
    require(type(value) is int and 0 <= value <= 1_000_000_000)
    return Decimal(value)


def day(value):
    require(type(value) is str and re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}", value) is not None)
    try:
        return date.fromisoformat(value)
    except ValueError:
        raise InputError("invalid economics date") from None


def render(value):
    rounded = value.quantize(Decimal("0.000001"), rounding=ROUND_HALF_EVEN)
    return "0" if not rounded else format(rounded, "f").rstrip("0").rstrip(".")


def ratio(numerator, denominator, reason="zero_denominator"):
    return {"value": render(numerator / denominator) if denominator else None,
            "unavailable_reason": None if denominator else reason}


def calculate(data):
    fields(data, ["schema_version", "currency", "month", "monthly", "cohort", "cash"])
    require(data["schema_version"] == "economics-v1")
    require(type(data["currency"]) is str and re.fullmatch(r"[A-Z]{3}", data["currency"]) is not None)
    require(type(data["month"]) is str and re.fullmatch(r"[0-9]{4}-[0-9]{2}", data["month"]) is not None)
    day(data["month"] + "-01")
    fields(data["monthly"], MONTHLY_FIELDS)
    m = {k: (count(v) if k in {"active_humans", "new_activated_humans"} else number(v))
         for k, v in data["monthly"].items()}
    require(m["premium_share"] <= 1 and m["new_activated_humans"] <= m["active_humans"])
    require(m["attributable_acquisition_spend"] <= m["acquisition_expense"])
    # Bounded input magnitudes and at most four multiplied factors fit exactly.
    # Use an isolated context so callers cannot change arithmetic precision.
    with localcontext() as context:
        context.prec = 80
        a, p = m["active_humans"], m["premium_share"]
        values = {
            "subscription_receipts": a * p * m["net_subscription_per_payer_month"],
            "bulk_receipts": a * m["net_bulk_per_active"],
            "ad_receipts": a * (1-p) * m["verified_ads_per_nonpremium_active"] * m["net_per_thousand_ads"] / 1000,
            "completed_player_matches": a * m["player_matches_per_active"],
            "content_labor": m["content_hours"] * m["content_hourly_cost"],
            "support_labor": m["support_hours"] * m["support_hourly_cost"],
            "content_providers": m["content_providers"],
            "hosting_backups_monitoring": m["hosting_backups_monitoring"],
            "acquisition_expense": m["acquisition_expense"],
        }
        values["net_receipts"] = sum(values[k] for k in ("subscription_receipts", "bulk_receipts", "ad_receipts"))
        values["runtime_cost"] = values["completed_player_matches"] * m["runtime_per_player_match"]
        values["operating_contribution"] = values["net_receipts"] - sum(values[k] for k in (
            "runtime_cost", "content_labor", "support_labor", "content_providers", "hosting_backups_monitoring"))
        values["acquisition_contribution"] = values["operating_contribution"] - m["acquisition_expense"]
        result = {
            "schema_version": "economics-report-v1", "formula_version": "business-plan-v1",
            "currency": data["currency"], "month": data["month"],
            "evidence_status": "unverified_arithmetic", "launch_approval": False,
            "decimal_places": 6, "rounding": "half_even",
            "monthly": {k: render(v) for k, v in values.items()},
            "cac": ratio(m["attributable_acquisition_spend"], m["new_activated_humans"]),
            "cohort": None, "cash_shortfall": None,
            "runway_months": {"value": None, "unavailable_reason": "missing_cash_inputs"},
        }
        cohort = data["cohort"]
        if cohort is not None:
            fields(cohort, ["activated_humans", "net_receipts", "attributable_costs", "observation_start", "observation_end_exclusive"])
            require(day(cohort["observation_start"]) < day(cohort["observation_end_exclusive"]))
            result["cohort"] = {
                "activated_humans": int(count(cohort["activated_humans"])),
                "observation_start": cohort["observation_start"],
                "observation_end_exclusive": cohort["observation_end_exclusive"],
                "observed_contribution_ltv": ratio(number(cohort["net_receipts"]) - number(cohort["attributable_costs"]), count(cohort["activated_humans"])),
            }
        cash = data["cash"]
        if cash is not None:
            fields(cash, ["available", "protected_reserve", "monthly_burn"])
            amounts = {k: None if v is None else number(v) for k, v in cash.items()}
            if amounts["available"] is not None and amounts["protected_reserve"] is not None:
                usable = amounts["available"] - amounts["protected_reserve"]
                result["cash_shortfall"] = render(max(-usable, Decimal(0)))
                if amounts["monthly_burn"] is not None:
                    result["runway_months"] = ratio(max(usable, Decimal(0)), amounts["monthly_burn"], "nonpositive_burn")
        return result


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def reject_constant(_value):
    raise InputError("invalid JSON constant")


def load_input(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        require(stat.S_ISREG(info.st_mode) and info.st_size <= MAX_INPUT_BYTES)
        raw = stream.read(MAX_INPUT_BYTES + 1)
    require(len(raw) <= MAX_INPUT_BYTES)
    try:
        return json.loads(raw, object_pairs_hook=unique_object, parse_constant=reject_constant)
    except (ValueError, UnicodeError, RecursionError):
        raise InputError("invalid JSON input") from None


def write_report(path, result):
    raw = (json.dumps(result, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n").encode()
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as stream:
        os.fchmod(stream.fileno(), 0o600)
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())
    return hashlib.sha256(raw).hexdigest()


class PrivateParser(argparse.ArgumentParser):
    def error(self, message):
        self.exit(2, "economics_report_refused\n")


def main():
    parser = PrivateParser(description=__doc__, allow_abbrev=False)
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        digest = write_report(args.output, calculate(load_input(args.input)))
    except (InputError, OSError):
        print("economics_report_refused", file=sys.stderr)
        return 1
    print("economics_report_written sha256=" + digest)
    return 0


if __name__ == "__main__":
    sys.exit(main())
