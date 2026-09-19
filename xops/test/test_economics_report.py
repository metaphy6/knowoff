"""Business-plan arithmetic and private CLI proofs using invented inputs."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[2] / "tools/economics_report.py"
SPEC = importlib.util.spec_from_file_location("economics_report", SCRIPT)
reporter = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(reporter)


def fixture():
    return {
        "schema_version": "economics-v1", "currency": "USD", "month": "2026-08",
        "monthly": {
            "active_humans": 1000, "player_matches_per_active": "8",
            "premium_share": "0.01", "net_subscription_per_payer_month": "3.50",
            "net_bulk_per_active": "0.02", "verified_ads_per_nonpremium_active": "1",
            "net_per_thousand_ads": "1", "runtime_per_player_match": "0.005",
            "content_hours": "10", "content_hourly_cost": "30", "content_providers": "30",
            "support_hours": "5", "support_hourly_cost": "30", "hosting_backups_monitoring": "50",
            "acquisition_expense": "100", "attributable_acquisition_spend": "60",
            "new_activated_humans": 20,
        },
        "cohort": {"activated_humans": 10, "net_receipts": "40", "attributable_costs": "70",
                   "observation_start": "2026-07-01", "observation_end_exclusive": "2026-08-01"},
        "cash": {"available": "1000", "protected_reserve": "200", "monthly_burn": "100"},
    }


class EconomicsTests(unittest.TestCase):
    def test_three_documented_examples(self):
        for share, bulk, ads, rate, receipts, contribution in [
            ("0.01", "0.02", "1", "1", "55.99", "-514.01"),
            ("0.03", "0.07", "3", "3", "183.73", "-386.27"),
            ("0.06", "0.15", "5", "5", "383.5", "-186.5"),
        ]:
            with self.subTest(share=share):
                data = fixture()
                data["monthly"].update(premium_share=share, net_bulk_per_active=bulk,
                                       verified_ads_per_nonpremium_active=ads, net_per_thousand_ads=rate)
                result = reporter.calculate(data)
                self.assertEqual(result["monthly"]["net_receipts"], receipts)
                self.assertEqual(result["monthly"]["operating_contribution"], contribution)
                self.assertEqual(result["monthly"]["runtime_cost"], "40")
                self.assertEqual(result["monthly"]["completed_player_matches"], "8000")

    def test_cash_is_separate_from_accounting_loss(self):
        result = reporter.calculate(fixture())
        self.assertEqual(result["monthly"]["acquisition_contribution"], "-614.01")
        self.assertEqual(result["cac"], {"value": "3", "unavailable_reason": None})
        self.assertEqual(result["runway_months"], {"value": "8", "unavailable_reason": None})
        self.assertEqual(result["cohort"]["observed_contribution_ltv"], {"value": "-3", "unavailable_reason": None})
        self.assertEqual(result["cohort"]["observation_end_exclusive"], "2026-08-01")
        self.assertEqual(result["evidence_status"], "unverified_arithmetic")
        self.assertFalse(result["launch_approval"])

    def test_zero_activity_keeps_fixed_and_labor_costs(self):
        data = fixture()
        data["monthly"]["active_humans"] = 0
        data["monthly"]["new_activated_humans"] = 0
        result = reporter.calculate(data)
        self.assertEqual(result["monthly"]["net_receipts"], "0")
        self.assertEqual(result["monthly"]["operating_contribution"], "-530")
        self.assertIsNone(result["cac"]["value"])

    def test_no_ads_and_all_premium(self):
        for update in ({"premium_share": "1"}, {"verified_ads_per_nonpremium_active": "0"}):
            data = fixture()
            data["monthly"].update(update)
            self.assertEqual(reporter.calculate(data)["monthly"]["ad_receipts"], "0")

    def test_unavailable_inputs_are_explicit(self):
        for cash in (None, {"available": None, "protected_reserve": "0", "monthly_burn": "1"},
                     {"available": "1", "protected_reserve": None, "monthly_burn": "1"},
                     {"available": "1", "protected_reserve": "0", "monthly_burn": None},
                     {"available": "1", "protected_reserve": "0", "monthly_burn": "0"}):
            data = fixture()
            data.update(cash=cash, cohort=None)
            result = reporter.calculate(data)
            self.assertIsNone(result["runway_months"]["value"])
            self.assertTrue(result["runway_months"]["unavailable_reason"])
            self.assertIsNone(result["cohort"])

    def test_reserve_exhaustion_and_zero_cohort(self):
        data = fixture()
        data["cash"].update(available="100", protected_reserve="200")
        data["cohort"]["activated_humans"] = 0
        result = reporter.calculate(data)
        self.assertEqual(result["runway_months"]["value"], "0")
        self.assertEqual(result["cash_shortfall"], "100")
        self.assertIsNone(result["cohort"]["observed_contribution_ltv"]["value"])

    def test_rounding_and_input_order(self):
        data = fixture()
        data["monthly"].update(attributable_acquisition_spend="1", new_activated_humans=3)
        self.assertEqual(reporter.calculate(data)["cac"]["value"], "0.333333")
        self.assertEqual(reporter.calculate(data), reporter.calculate(dict(reversed(list(data.items())))))

    def test_half_even_boundary_and_maximum_magnitude(self):
        data = fixture()
        data["monthly"].update(attributable_acquisition_spend="0.000001", new_activated_humans=2)
        self.assertEqual(reporter.calculate(data)["cac"]["value"], "0")
        data["monthly"]["attributable_acquisition_spend"] = "0.000003"
        self.assertEqual(reporter.calculate(data)["cac"]["value"], "0.000002")
        data["monthly"].update(active_humans=1_000_000_000, premium_share="0",
                               verified_ads_per_nonpremium_active="999999999999.999999",
                               net_per_thousand_ads="999999999999.999999")
        self.assertEqual(reporter.calculate(data)["monthly"]["ad_receipts"],
                         "999999999999999998000000000000.000001")

    def test_private_cli_errors_do_not_echo_arguments_or_input(self):
        done = subprocess.run([sys.executable, str(SCRIPT), "--secret-private-value"], capture_output=True, text=True)
        self.assertNotEqual(done.returncode, 0)
        self.assertEqual(done.stderr, "economics_report_refused\n")
        with tempfile.TemporaryDirectory() as tmp:
            source, output = Path(tmp) / "private-input", Path(tmp) / "out"
            source.write_text('{"private-identity":"sensitive-value"}')
            done = subprocess.run([sys.executable, str(SCRIPT), "--input", str(source), "--output", str(output)], capture_output=True, text=True)
            self.assertNotEqual(done.returncode, 0)
            self.assertEqual(done.stderr, "economics_report_refused\n")
            self.assertFalse(output.exists())

    def test_invalid_values_and_counts_refuse(self):
        for field, value in [("premium_share", "1.1"), ("premium_share", True),
                             ("active_humans", True), ("active_humans", -1),
                             ("active_humans", 1.5), ("content_hours", -1),
                             ("content_hours", "-1"), ("content_hours", "NaN"),
                             ("content_hours", "Infinity"), ("content_hours", "1e9"),
                             ("content_hours", "0.0000001"), ("content_hours", "9" * 13)]:
            with self.subTest(field=field, value=value):
                data = fixture()
                data["monthly"][field] = value
                with self.assertRaises(reporter.InputError):
                    reporter.calculate(data)

    def test_unknown_missing_and_impossible_metadata_refuse(self):
        for mutation in (lambda d: d.update(account_id="private-value"),
                         lambda d: d.pop("cash"), lambda d: d.update(currency="mixed USD/EUR"),
                         lambda d: d.update(month="2026-13"),
                         lambda d: d["monthly"].update(new_activated_humans=1001),
                         lambda d: d["monthly"].update(attributable_acquisition_spend="101"),
                         lambda d: d["cohort"].update(observation_start="2026-08-02"),
                         lambda d: d["cohort"].update(observation_start="2026-02-30")):
            data = fixture()
            mutation(data)
            with self.assertRaises(reporter.InputError):
                reporter.calculate(data)

    def test_cli_private_output_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            source, output = Path(tmp) / "in.json", Path(tmp) / "out.json"
            source.write_text(json.dumps(fixture()))
            command = [sys.executable, str(SCRIPT), "--input", str(source), "--output", str(output)]
            done = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(done.returncode, 0, done.stderr)
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            original = output.read_bytes()
            self.assertNotIn("1000", done.stdout)
            self.assertNotIn(tmp, done.stdout + done.stderr)
            again = subprocess.run(command, capture_output=True, text=True)
            self.assertNotEqual(again.returncode, 0)
            self.assertEqual(output.read_bytes(), original)

    def test_duplicate_nonregular_and_oversized_input_refuse(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / "in.json"
            for raw in ('{"schema_version":"economics-v1","schema_version":"economics-v1"}',
                        '{"value":NaN}', 'x' * (reporter.MAX_INPUT_BYTES + 1)):
                source.write_text(raw)
                with self.assertRaises(reporter.InputError):
                    reporter.load_input(source)
            link = Path(tmp) / "link"
            link.symlink_to(source)
            with self.assertRaises((reporter.InputError, OSError)):
                reporter.load_input(link)
            fifo = Path(tmp) / "fifo"
            os.mkfifo(fifo)
            with self.assertRaises(reporter.InputError):
                reporter.load_input(fifo)


if __name__ == "__main__":
    unittest.main()
