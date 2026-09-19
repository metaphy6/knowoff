"""Synthetic boundary proofs for the offline calculator; no participant data."""

import copy
import importlib.util
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[2] / "tools/cohort_report.py"
SPEC = importlib.util.spec_from_file_location("cohort_report", SCRIPT)
reporter = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(reporter)


def fixture():
    return {
        "schema_version": "synthetic-cohort-v1", "synthetic": True,
        "manifest": {
            "formula_version": "cohort-v1", "experiment": "fixture-experiment",
            "cohort_start": "2024-02-28T00:00:00Z",
            "cohort_end": "2024-02-29T00:00:00Z",
            "observation_complete_through": "2024-03-07T00:00:00Z",
            "week_start": "2024-02-26", "queue_timeout_seconds": 30,
            "second_match_cutoff": "inclusive_7_days",
            "cells": [{"id": "fixture-cell", "mode": "missed_the_briefing",
                       "size": 4, "language": "en", "access": "free",
                       "exposure": "prototype", "hosted": False,
                       "rules": "fixture-rules", "pack": "fixture-pack",
                       "build": "fixture-build"}],
        },
        "subjects": [{"id": "fixture-human", "group": "fixture-group",
                      "cell": "fixture-cell", "arrived_at": "2024-02-28T23:00:00Z",
                      "eligibility": "new_eligible_human", "observation": "complete",
                      "first_session": "fixture-session"}],
        "sessions": [{"id": "fixture-session", "subject": "fixture-human",
                      "started_at": "2024-02-28T23:00:00Z", "ended_at": "2024-02-29T00:30:00Z"}],
        "matches": [], "participations": [], "joins": [],
        "intentions": None, "deleted_subjects": [],
    }


def match(data, suffix="one", start="2024-02-28T23:10:00Z",
          end="2024-02-28T23:20:00Z", outcome="normal", voluntary=True,
          session="fixture-session", subject="fixture-human", cell="fixture-cell"):
    ident = "fixture-" + suffix
    data["matches"].append({"id": ident, "cell": cell, "started_at": start,
                            "ended_at": end, "outcome": outcome})
    data["participations"].append({"subject": subject, "match": ident,
                                   "session": session, "voluntary": voluntary})


def cell_report(data):
    return reporter.calculate(data)["cells"][0]


class CohortTests(unittest.TestCase):
    def test_activation_keeps_abandoners_and_unknowns_explicit(self):
        data = fixture()
        match(data)
        for suffix, eligibility in [("abandon", "new_eligible_human"), ("unknown", "unknown")]:
            subject = dict(data["subjects"][0], id="fixture-" + suffix,
                           first_session=None, eligibility=eligibility)
            data["subjects"].append(subject)
        data["joins"] = [{"id": "fixture-join", "subject": "fixture-abandon",
                          "cell": "fixture-cell", "joined_at": "2024-02-28T23:01:00Z",
                          "terminal_at": "2024-02-28T23:01:10Z", "outcome": "left",
                          "reason": "voluntary", "match": None}]
        result = cell_report(data)
        self.assertEqual(result["activation"]["arrivals"], {"numerator": 1, "denominator": 2, "rate": 0.5})
        self.assertEqual(result["activation"]["queue_entrants"]["denominator"], 1)
        self.assertEqual(result["activation"]["queue_entrants"]["numerator"], 0)
        self.assertEqual(result["exclusions"]["unknown"], 1)
        self.assertEqual(result["activation"]["missing_first_session"], 1)

    def test_first_session_must_contain_start_and_end(self):
        data = fixture()
        match(data, end="2024-02-29T00:30:01Z")
        self.assertEqual(cell_report(data)["activation"]["arrivals"]["numerator"], 0)
        data["matches"][0]["ended_at"] = "2024-02-29T00:30:00Z"
        self.assertEqual(cell_report(data)["activation"]["arrivals"]["numerator"], 1)
        data["sessions"][0]["ended_at"] = None
        self.assertEqual(cell_report(data)["activation"]["missing_first_session"], 1)
        self.assertEqual(cell_report(data)["activation"]["arrivals"]["numerator"], 0)

    def test_retention_leap_midnight_and_whole_day_maturity(self):
        data = fixture()
        match(data)
        match(data, "return", "2024-02-28T23:59:00Z", "2024-02-29T00:00:00Z")
        result = cell_report(data)
        self.assertEqual(result["d1"]["numerator"], 1)
        self.assertEqual(result["d7"]["denominator"], 1)
        self.assertEqual(result["weekly"]["humans"], 1)
        data["manifest"]["observation_complete_through"] = "2024-03-06T23:59:59Z"
        result = cell_report(data)
        self.assertEqual(result["d7"]["immature"], 1)
        self.assertIsNone(result["d7"]["rate"])
        self.assertEqual(result["d7"]["coverage"], 0)

    def test_second_cutoff_is_inclusive_and_forced_repeat_never_counts(self):
        data = fixture()
        match(data)
        data["sessions"].append({"id": "fixture-later", "subject": "fixture-human",
                                 "started_at": "2024-03-06T23:00:00Z", "ended_at": "2024-03-06T23:30:00Z"})
        match(data, "second", "2024-03-06T23:10:00Z", "2024-03-06T23:20:00Z", session="fixture-later")
        self.assertEqual(cell_report(data)["second_match"]["numerator"], 1)
        data["participations"][1]["voluntary"] = False
        self.assertEqual(cell_report(data)["second_match"]["numerator"], 0)
        data["participations"][1]["voluntary"] = True
        data["matches"][1]["ended_at"] = "2024-03-06T23:20:01Z"
        self.assertEqual(cell_report(data)["second_match"]["numerator"], 0)

    def test_completion_counts_matches_once_and_keeps_abnormal_and_pending(self):
        data = fixture()
        for i, outcome in enumerate(["normal", "forfeit", "low_population", "infrastructure", "active", "unknown"]):
            match(data, str(i), outcome=outcome, end=None if outcome in ("active", "unknown") else "2024-02-28T23:20:00Z")
        data["subjects"].append(dict(data["subjects"][0], id="fixture-peer", first_session=None))
        data["participations"].append({"subject": "fixture-peer", "match": "fixture-0", "session": None, "voluntary": True})
        result = cell_report(data)["completion"]
        self.assertEqual((result["numerator"], result["denominator"], result["incomplete"]), (1, 6, 2))

    def test_missing_observation_and_hosted_cells_stay_separate(self):
        data = fixture()
        match(data)
        data["subjects"][0]["observation"] = "missing"
        data["manifest"]["cells"].append(dict(data["manifest"]["cells"][0], id="fixture-hosted", hosted=True))
        result = reporter.calculate(data)
        self.assertEqual(len(result["cells"]), 2)
        self.assertEqual(result["cells"][0]["d1"]["missing"], 1)
        self.assertIsNone(result["cells"][0]["d1"]["rate"])
        self.assertEqual(result["cells"][1]["activation"]["arrivals"]["denominator"], 0)
        self.assertNotIn("passed", json.dumps(result))

    def test_queue_all_joins_timeout_ties_reasons_and_unresolved(self):
        data = fixture()
        match(data, start="2024-02-28T23:01:30Z")
        for i, outcome in enumerate(["started", "left", "unresolved"]):
            data["joins"].append({"id": "fixture-join" + str(i), "subject": "fixture-human",
                                  "cell": "fixture-cell", "joined_at": "2024-02-28T23:01:00Z",
                                  "terminal_at": None if outcome == "unresolved" else "2024-02-28T23:01:30Z",
                                  "outcome": outcome, "reason": "voluntary" if outcome == "left" else None,
                                  "match": "fixture-one" if outcome == "started" else None})
        result = cell_report(data)["queue"]
        self.assertEqual(result["start_within_timeout"]["numerator"], 1)
        self.assertEqual(result["start_within_timeout"]["denominator"], 3)
        self.assertEqual(result["prestart_leave"]["numerator"], 1)
        self.assertEqual(result["unresolved"], 1)
        self.assertEqual(result["terminal_wait_seconds"], {"count": 2, "p50": 30, "p95": 30})
        self.assertEqual(result["leave_reasons"], {"voluntary": 1})

    def test_deletion_recomputation_ignores_replayed_stale_facts(self):
        data = fixture()
        match(data)
        data["deleted_subjects"] = ["fixture-human"]
        stale = reporter.calculate(data)
        clean = copy.deepcopy(data)
        clean["subjects"] = clean["sessions"] = clean["participations"] = []
        self.assertEqual(stale, reporter.calculate(clean))
        self.assertNotIn("fixture-human", json.dumps(stale))
        self.assertEqual(stale["cells"][0]["completion"]["denominator"], 0)

    def test_deterministic_order_and_unavailable_intention_gate(self):
        data = fixture()
        match(data)
        match(data, "two")
        expected = reporter.calculate(data)
        data["matches"].reverse()
        data["participations"].reverse()
        self.assertEqual(expected, reporter.calculate(data))
        self.assertEqual(expected["cells"][0]["intentions"]["conversion"], "unavailable")

    def test_known_voluntary_success_is_not_hidden_by_other_unknown_intent(self):
        data = fixture()
        match(data)
        match(data, "unknown", "2024-02-28T23:21:00Z", "2024-02-28T23:25:00Z", voluntary=None)
        match(data, "known", "2024-02-28T23:26:00Z", "2024-02-28T23:30:00Z")
        result = cell_report(data)["second_match"]
        self.assertEqual((result["numerator"], result["denominator"], result["missing"]), (1, 1, 0))

    def test_other_cell_cannot_improve_acquisition_retention(self):
        data = fixture()
        match(data)
        data["manifest"]["cells"].append(dict(data["manifest"]["cells"][0], id="fixture-other", mode="secret_scale"))
        match(data, "return", "2024-02-28T23:59:00Z", "2024-02-29T00:00:00Z", cell="fixture-other")
        result = reporter.calculate(data)["cells"]
        self.assertEqual(result[0]["d1"]["numerator"], 0)
        self.assertEqual(result[0]["second_match"]["numerator"], 0)
        self.assertEqual(result[1]["completion"]["numerator"], 1)
        self.assertEqual(result[1]["activation"]["arrivals"]["denominator"], 0)
        self.assertEqual(result[1]["sample_humans"], 1)
        self.assertEqual(result[1]["sample_groups"], 1)

    def test_missing_session_join_and_unknown_outcome_are_explicit(self):
        data = fixture()
        match(data, session=None)
        result = cell_report(data)["activation"]
        self.assertEqual(result["arrivals"]["numerator"], 0)
        self.assertEqual(result["missing_first_session"], 1)
        data["participations"][0]["session"] = "fixture-session"
        data["matches"][0].update(outcome="unknown", ended_at=None)
        result = cell_report(data)["activation"]
        self.assertEqual(result["missing_match_outcome"], 1)
        self.assertEqual(result["arrivals"]["numerator"], 0)

    def test_unknown_return_outcome_is_missing_but_known_return_still_counts(self):
        data = fixture()
        match(data)
        match(data, "unknown", "2024-02-28T23:30:00Z", None, outcome="unknown")
        result = cell_report(data)
        for metric in ("d1", "d7", "second_match"):
            self.assertEqual(result[metric]["missing"], 1)
            self.assertIsNone(result[metric]["rate"])
        match(data, "return", "2024-02-28T23:59:00Z", "2024-02-29T00:00:00Z")
        result = cell_report(data)
        for metric in ("d1", "second_match"):
            self.assertEqual((result[metric]["numerator"], result[metric]["missing"]), (1, 0))

    def test_unknown_earlier_completion_cannot_invent_activation_day(self):
        data = fixture()
        match(data, "unknown", "2024-02-28T23:10:00Z", None, outcome="unknown")
        match(data, "known", "2024-02-28T23:59:00Z", "2024-02-29T00:01:00Z")
        result = cell_report(data)
        self.assertEqual(result["activation"]["arrivals"]["numerator"], 1)
        self.assertEqual(result["activation"]["missing_activation_time"], 1)
        self.assertEqual(result["activation"]["missing_match_outcome"], 1)
        for metric in ("d1", "d7", "second_match"):
            self.assertEqual(result[metric]["missing"], 1)
            self.assertIsNone(result[metric]["rate"])
        data["matches"][0].update(outcome="normal", ended_at="2024-02-28T23:20:00Z")
        data["participations"][0]["session"] = None
        self.assertEqual(cell_report(data)["activation"]["missing_activation_time"], 1)
        data["participations"][0]["session"] = "fixture-session"
        result = cell_report(data)
        self.assertEqual(result["activation"]["missing_activation_time"], 0)
        self.assertEqual(result["d1"]["numerator"], 1)

    def test_arrival_cohort_is_half_open_and_week_is_separate(self):
        data = fixture()
        data["subjects"][0]["arrived_at"] = data["manifest"]["cohort_start"]
        match(data)
        self.assertEqual(cell_report(data)["activation"]["arrivals"]["denominator"], 1)
        data["subjects"][0]["arrived_at"] = data["manifest"]["cohort_end"]
        data["subjects"][0]["first_session"] = None
        data["sessions"] = data["matches"] = data["participations"] = []
        result = cell_report(data)
        self.assertEqual(result["activation"]["arrivals"]["denominator"], 0)
        self.assertEqual(result["exclusions"]["outside_cohort"], 1)

    def test_d7_exact_midnight_return_and_week_boundary(self):
        data = fixture()
        match(data)
        match(data, "d7", "2024-03-05T23:59:00Z", "2024-03-06T00:00:00Z", session=None)
        result = cell_report(data)
        self.assertEqual(result["d7"]["numerator"], 1)
        self.assertEqual(result["weekly"]["humans"], 0)
        data["matches"][1]["ended_at"] = "2024-03-05T23:59:59Z"
        self.assertEqual(cell_report(data)["d7"]["numerator"], 0)

    def test_queue_over_timeout_and_nearest_rank_waits_keep_unknown_session(self):
        data = fixture()
        data["sessions"][0]["ended_at"] = None
        match(data, start="2024-02-28T23:01:31Z")
        data["joins"] = [{"id": "fixture-join", "subject": "fixture-human",
                          "cell": "fixture-cell", "joined_at": "2024-02-28T23:01:00Z",
                          "terminal_at": "2024-02-28T23:01:31Z", "outcome": "started",
                          "reason": None, "match": "fixture-one"}]
        result = cell_report(data)
        self.assertEqual(result["activation"]["queue_entrants"]["denominator"], 1)
        self.assertEqual(result["queue"]["start_within_timeout"]["numerator"], 0)
        for i, seconds in enumerate([1, 2, 3, 10, 10]):
            data["joins"].append({"id": "fixture-left" + str(i), "subject": "fixture-human",
                                  "cell": "fixture-cell", "joined_at": "2024-02-28T23:02:00Z",
                                  "terminal_at": "2024-02-28T23:02:" + str(seconds).zfill(2) + "Z",
                                  "outcome": "left", "reason": "disconnect", "match": None})
        self.assertEqual(cell_report(data)["queue"]["terminal_wait_seconds"], {"count": 6, "p50": 3, "p95": 31})

    def test_deletion_with_shared_match_preserves_only_surviving_human(self):
        data = fixture()
        match(data)
        data["subjects"].append(dict(data["subjects"][0], id="fixture-peer", first_session=None))
        data["participations"].append({"subject": "fixture-peer", "match": "fixture-one", "session": None, "voluntary": True})
        data["intentions"] = [{"subject": "fixture-human", "stage": "invitation_sent"},
                              {"subject": "fixture-peer", "stage": "opt_in_return"}]
        data["deleted_subjects"] = ["fixture-human"]
        result = cell_report(data)
        self.assertEqual(result["completion"]["denominator"], 1)
        self.assertEqual(result["sample_humans"], 1)
        self.assertEqual(result["intentions"]["stages"], {"opt_in_return": 1})


class ValidationTests(unittest.TestCase):
    def test_all_five_normative_modes_and_unknown_mode_refusal(self):
        for mode in ["missed_the_briefing", "secret_scale", "make_room", "bad_bargains", "top_that"]:
            data = fixture()
            data["manifest"]["cells"][0]["mode"] = mode
            self.assertEqual(cell_report(data)["cell"]["mode"], mode)
        data["manifest"]["cells"][0]["mode"] = "invented"
        with self.assertRaises(reporter.FixtureError):
            reporter.calculate(data)

    def test_temporal_conflicts_record_limits_and_hosted_mixing_refused(self):
        changes = [lambda d: d["subjects"][0].update(arrived_at="2024-03-08T00:00:00Z"),
                   lambda d: d["sessions"][0].update(started_at="2024-02-28T22:00:00Z"),
                   lambda d: d["sessions"][0].update(ended_at="2024-02-28T22:00:00Z"),
                   lambda d: d["manifest"].update(week_start="2024-02-27"),
                   lambda d: d["deleted_subjects"].extend(["fixture-deleted"] * 2),
                   lambda d: d["subjects"].extend([d["subjects"][0]] * reporter.MAX_RECORDS)]
        for change in changes:
            data = fixture()
            change(data)
            with self.subTest(change=change), self.assertRaises(reporter.FixtureError):
                reporter.calculate(data)
        data = fixture()
        data["manifest"]["cells"].append(dict(data["manifest"]["cells"][0], id="fixture-hosted", hosted=True))
        match(data, cell="fixture-hosted")
        with self.assertRaises(reporter.FixtureError):
            reporter.calculate(data)

    def test_rejects_private_unknown_fields_types_duplicate_and_conflict(self):
        bad = []
        for key in ["account_id", "device_id", "name", "chat", "prompt", "hand", "role", "currency"]:
            data = fixture()
            data["subjects"][0][key] = "private-value"
            bad.append(data)
        for mutate in [lambda d: d.update(synthetic=False),
                       lambda d: d["manifest"].update(queue_timeout_seconds=True),
                       lambda d: d["subjects"].append(copy.deepcopy(d["subjects"][0])),
                       lambda d: d["subjects"][0].update(id="raw-account"),
                       lambda d: d["subjects"][0].update(arrived_at="2024-02-28T23:00:00+00:00"),
                       lambda d: d["subjects"][0].update(first_session="fixture-unknown"),
                       lambda d: d["manifest"].update(formula_version="unknown")]:
            data = fixture()
            mutate(data)
            bad.append(data)
        for data in bad:
            with self.subTest(data=data), self.assertRaises(reporter.FixtureError):
                reporter.calculate(data)

    def test_rejects_duplicate_participations_and_conflicting_match_identity(self):
        data = fixture()
        match(data)
        data["participations"].append(dict(data["participations"][0]))
        with self.assertRaises(reporter.FixtureError):
            reporter.calculate(data)
        data["participations"].pop()
        data["matches"].append(dict(data["matches"][0], outcome="forfeit"))
        with self.assertRaises(reporter.FixtureError):
            reporter.calculate(data)

    def test_bounded_regular_strict_json_and_exclusive_private_output(self):
        with tempfile.TemporaryDirectory(dir=SCRIPT.parent) as directory:
            root = Path(directory)
            source = root / "input.json"
            source.write_text(json.dumps(fixture()))
            report = reporter.calculate(reporter.load_fixture(source))
            target = root / "output.json"
            reporter.write_report(target, report)
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o600)
            with self.assertRaises(OSError):
                reporter.write_report(target, report)
            link = root / "link"
            link.symlink_to(source)
            with self.assertRaises((reporter.FixtureError, OSError)):
                reporter.load_fixture(link)
            fifo = root / "fifo"
            os.mkfifo(fifo)
            with self.assertRaises(reporter.FixtureError):
                reporter.load_fixture(fifo)
            with self.assertRaises(reporter.FixtureError):
                reporter.load_fixture(root)
            for text in ['{"synthetic":true,"synthetic":false}', '{"x":NaN}', '[]', '[' * 2000,
                         ' ' * (reporter.MAX_INPUT_BYTES + 1)]:
                source.write_text(text)
                with self.subTest(text=text[:40]), self.assertRaises(reporter.FixtureError):
                    reporter.load_fixture(source)

    def test_cli_emits_only_status_hash_and_sanitizes_errors(self):
        with tempfile.TemporaryDirectory(dir=SCRIPT.parent) as directory:
            root = Path(directory)
            source = root / "input.json"
            source.write_text(json.dumps(fixture()))
            command = [sys.executable, str(SCRIPT), "--input", str(source), "--output", str(root / "out.json")]
            result = subprocess.run(command, capture_output=True, text=True, check=False)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertRegex(result.stdout, r"^synthetic_report_written sha256=[a-f0-9]{64}\n$")
            self.assertEqual(result.stderr, "")
            source.write_text('{"secret-marker": "private"}')
            result = subprocess.run(command, capture_output=True, text=True, check=False)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn("secret-marker", result.stderr)
            self.assertNotIn(str(source), result.stderr)
            self.assertEqual(result.stdout, "")

    def test_cli_unknown_arguments_are_not_echoed(self):
        result = subprocess.run([sys.executable, str(SCRIPT), "--input", "fixture.json",
                                 "--output", "out.json", "--private-marker=secret"],
                                capture_output=True, text=True, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("private-marker", result.stderr)
        self.assertNotIn("secret", result.stderr)
        self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
