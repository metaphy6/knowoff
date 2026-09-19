#!/usr/bin/env python3
"""Bounded, synthetic-only offline cohort calculations. Never reads live stores."""

import argparse
from collections import Counter, defaultdict
from datetime import date, datetime, time, timedelta, timezone
import hashlib
import json
import math
import os
import re
import stat
import sys


# Resource limits, not product thresholds. All gameplay timeouts are input facts.
MAX_INPUT_BYTES = 4 * 1024 * 1024
MAX_RECORDS = 10000
MODES = {"missed_the_briefing", "secret_scale", "make_room", "bad_bargains", "top_that"}
HUMANS = {"new_eligible_human", "existing_human"}
OUTCOMES = {"normal", "forfeit", "low_population", "infrastructure", "active", "unknown"}


class FixtureError(ValueError):
    """Invalid fixture; messages deliberately contain no supplied data."""


def require(condition):
    if not condition:
        raise FixtureError("invalid synthetic fixture")


def fields(value, expected):
    require(type(value) is dict and set(value) == set(expected.split()))


def identifier(value):
    require(type(value) is str and re.fullmatch(r"fixture-[a-z0-9][a-z0-9_-]{0,55}", value) is not None)


def choice(value, options):
    require(type(value) is str and value in options)


def timestamp(value):
    require(type(value) is str and re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ", value) is not None)
    try:
        return datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
    except ValueError:
        raise FixtureError("invalid timestamp") from None


def unique_records(rows, keys):
    require(type(rows) is list and len(rows) <= MAX_RECORDS)
    found = {}
    for row in rows:
        require(type(row) is dict and all(key in row for key in keys))
        key = tuple(row[field] for field in keys)
        for value in key:
            identifier(value)
        require(key not in found)
        found[key] = row
    return found


def validate(data):
    """Refuse ambiguous identities, private fields and inconsistent fact joins."""
    fields(data, "schema_version synthetic manifest subjects sessions matches participations joins intentions deleted_subjects")
    require(data["synthetic"] is True and data["schema_version"] == "synthetic-cohort-v1")
    manifest = data["manifest"]
    fields(manifest, "formula_version experiment cohort_start cohort_end observation_complete_through week_start queue_timeout_seconds second_match_cutoff cells")
    require(manifest["formula_version"] == "cohort-v1")
    require(manifest["second_match_cutoff"] == "inclusive_7_days")
    identifier(manifest["experiment"])
    start, end, watermark = [timestamp(manifest[key]) for key in
                             ("cohort_start", "cohort_end", "observation_complete_through")]
    require(start < end <= watermark and watermark.year < 9999)
    require(type(manifest["queue_timeout_seconds"]) is int and 0 < manifest["queue_timeout_seconds"] <= 86400)
    require(type(manifest["week_start"]) is str and re.fullmatch(r"\d{4}-\d\d-\d\d", manifest["week_start"]) is not None)
    try:
        week = date.fromisoformat(manifest["week_start"])
    except ValueError:
        raise FixtureError("invalid week") from None
    require(week.weekday() == 0 and week.year < 9999)
    cells = {key[0]: row for key, row in unique_records(manifest["cells"], ("id",)).items()}
    require(0 < len(cells) <= 100)
    configurations = set()
    for cell in cells.values():
        fields(cell, "id mode size language access exposure hosted rules pack build")
        choice(cell["mode"], MODES)
        require(type(cell["size"]) is int and cell["size"] in (4, 6))
        require(type(cell["language"]) is str and re.fullmatch(r"[a-z]{2,3}(?:-[A-Za-z0-9]{2,8}){0,2}", cell["language"]) is not None)
        choice(cell["access"], {"free", "paid"})
        choice(cell["exposure"], {"prototype", "enabled", "held"})
        require(type(cell["hosted"]) is bool)
        for key in ("rules", "pack", "build"):
            identifier(cell[key])
        identity = tuple(cell[key] for key in sorted(cell) if key != "id")
        require(identity not in configurations)
        configurations.add(identity)
    tables = {}
    total = 0
    for name, keys in [("subjects", ("id",)), ("sessions", ("id",)),
                       ("matches", ("id",)), ("participations", ("subject", "match")),
                       ("joins", ("id",))]:
        tables[name] = unique_records(data[name], keys)
        total += len(tables[name])
    deleted = data["deleted_subjects"]
    require(type(deleted) is list and len(deleted) <= MAX_RECORDS)
    for ident in deleted:
        identifier(ident)
    require(len(set(deleted)) == len(deleted))
    deleted = set(deleted)
    subjects = {key[0]: row for key, row in tables["subjects"].items()}
    sessions = {key[0]: row for key, row in tables["sessions"].items()}
    matches = {key[0]: row for key, row in tables["matches"].items()}

    def stamp(value):
        result = timestamp(value)
        require(result <= watermark)
        return result

    def subject_ref(row):
        identifier(row["subject"])
        require(row["subject"] in subjects or row["subject"] in deleted)
        return row["subject"] not in deleted

    def cell_ref(row):
        identifier(row["cell"])
        require(row["cell"] in cells)

    for subject in subjects.values():
        fields(subject, "id group cell arrived_at eligibility observation first_session")
        identifier(subject["group"])
        cell_ref(subject)
        stamp(subject["arrived_at"])
        choice(subject["eligibility"], HUMANS | {"ineligible", "unknown"})
        choice(subject["observation"], {"complete", "missing"})
        first = subject["first_session"]
        if first is not None:
            identifier(first)
            if subject["id"] not in deleted:
                require(first in sessions and sessions[first].get("subject") == subject["id"])
    sessions_by_subject = defaultdict(list)
    for session in sessions.values():
        fields(session, "id subject started_at ended_at")
        live = subject_ref(session)
        began = stamp(session["started_at"])
        if session["ended_at"] is not None:
            require(began <= stamp(session["ended_at"]))
        if live:
            require(began >= stamp(subjects[session["subject"]]["arrived_at"]))
            sessions_by_subject[session["subject"]].append(session)
    for subject_id, sequence in sessions_by_subject.items():
        sequence.sort(key=lambda row: (row["started_at"], row["id"]))
        first = subjects[subject_id]["first_session"]
        require(first is None or first == sequence[0]["id"])
        for previous, current in zip(sequence, sequence[1:]):
            require(previous["ended_at"] is not None and previous["ended_at"] <= current["started_at"])
    for match in matches.values():
        fields(match, "id cell started_at ended_at outcome")
        cell_ref(match)
        began = stamp(match["started_at"])
        choice(match["outcome"], OUTCOMES)
        require((match["ended_at"] is None) == (match["outcome"] in {"active", "unknown"}))
        if match["ended_at"] is not None:
            require(began <= stamp(match["ended_at"]))
    participations = list(tables["participations"].values())
    for row in participations:
        fields(row, "subject match session voluntary")
        live = subject_ref(row)
        require(row["match"] in matches)
        require(row["voluntary"] is None or type(row["voluntary"]) is bool)
        if row["session"] is not None:
            identifier(row["session"])
            if live:
                require(row["session"] in sessions and sessions[row["session"]]["subject"] == row["subject"])
        if live:
            require(matches[row["match"]]["started_at"] >= subjects[row["subject"]]["arrived_at"])
            require(cells[matches[row["match"]]["cell"]]["hosted"] == cells[subjects[row["subject"]]["cell"]]["hosted"])
    started_pairs = set()
    for row in data["joins"]:
        fields(row, "id subject cell joined_at terminal_at outcome reason match")
        live = subject_ref(row)
        cell_ref(row)
        began = stamp(row["joined_at"])
        choice(row["outcome"], {"started", "left", "unresolved"})
        require((row["terminal_at"] is None) == (row["outcome"] == "unresolved"))
        if row["terminal_at"] is not None:
            require(began <= stamp(row["terminal_at"]))
        if live:
            require(row["joined_at"] >= subjects[row["subject"]]["arrived_at"])
            require(cells[row["cell"]]["hosted"] == cells[subjects[row["subject"]]["cell"]]["hosted"])
        if row["outcome"] == "left":
            choice(row["reason"], {"voluntary", "timeout", "disconnect", "infrastructure", "unknown"})
        else:
            require(row["reason"] is None)
        if row["outcome"] == "started":
            identifier(row["match"])
            require(row["match"] in matches)
            match = matches[row["match"]]
            require(match["cell"] == row["cell"] and match["started_at"] == row["terminal_at"])
            pair = (row["subject"], row["match"])
            require(pair not in started_pairs)
            started_pairs.add(pair)
            if live:
                require(pair in tables["participations"])
        else:
            require(row["match"] is None)
    intentions = data["intentions"]
    if intentions is not None:
        require(type(intentions) is list and len(intentions) <= MAX_RECORDS)
        seen = set()
        for row in intentions:
            fields(row, "subject stage")
            subject_ref(row)
            choice(row["stage"], {"invitation_sent", "opt_in_return"})
            key = (row["subject"], row["stage"])
            require(key not in seen)
            seen.add(key)
        total += len(intentions)
    require(total + len(deleted) <= MAX_RECORDS)
    return cells, subjects, sessions, matches, deleted


def ratio(numerator, denominator):
    return {"numerator": numerator, "denominator": denominator,
            "rate": numerator / denominator if denominator else None}


def wait_summary(waits):
    ordered = sorted(waits)
    return {"count": len(ordered),
            "p50": ordered[math.ceil(len(ordered) * 0.50) - 1] if ordered else None,
            "p95": ordered[math.ceil(len(ordered) * 0.95) - 1] if ordered else None}


def calculate(data):
    """Return deterministic per-cell aggregates; no individual identity leaves here."""
    cells, all_subjects, sessions, matches, deleted = validate(data)
    manifest = data["manifest"]
    subjects = {key: row for key, row in all_subjects.items() if key not in deleted}
    human_ids = {key for key, row in subjects.items() if row["eligibility"] in HUMANS}
    by_subject = defaultdict(list)
    by_cell = defaultdict(set)
    for participation in data["participations"]:
        if participation["subject"] in human_ids:
            match = matches[participation["match"]]
            by_subject[participation["subject"]].append((match, participation))
            by_cell[match["cell"]].add(match["id"])
    watermark = timestamp(manifest["observation_complete_through"])
    week_start = date.fromisoformat(manifest["week_start"])
    week_end = week_start + timedelta(days=7)
    result = {"schema_version": "synthetic-cohort-report-v1", "synthetic": True,
              "formula_version": manifest["formula_version"], "experiment": manifest["experiment"],
              "cohort_start": manifest["cohort_start"], "cohort_end": manifest["cohort_end"],
              "observation_complete_through": manifest["observation_complete_through"],
              "week_start": manifest["week_start"], "week_end_exclusive": week_end.isoformat(),
              "queue_timeout_seconds": manifest["queue_timeout_seconds"],
              "second_match_cutoff": manifest["second_match_cutoff"],
              "return_scope": "same_exposure_cell", "quantiles": "nearest_rank_terminal_waits",
              "evidence": "synthetic_formulas_only", "decision": "unavailable",
              "uncertainty": "descriptive_only_correlated_groups_no_independence_claim",
              "deleted_subjects": len(deleted), "cells": []}
    for cell_id, cell in sorted(cells.items()):
        arrivals = {key: row for key, row in subjects.items() if row["cell"] == cell_id}
        new = {key: row for key, row in arrivals.items()
               if row["eligibility"] == "new_eligible_human"
               and manifest["cohort_start"] <= row["arrived_at"] < manifest["cohort_end"]}
        exclusions = dict(Counter(row["eligibility"] for row in arrivals.values()
                                  if row["eligibility"] != "new_eligible_human"))
        exclusions["outside_cohort"] = sum(row["eligibility"] == "new_eligible_human" and key not in new
                                           for key, row in arrivals.items())
        valid = {key: sorted(((match, p) for match, p in rows
                              if match["cell"] == cell_id and match["outcome"] == "normal"),
                             key=lambda item: (item[0]["ended_at"], item[0]["id"]))
                 for key, rows in by_subject.items()}
        activated = {}
        uncertain_activation = set()
        missing_first = 0
        missing_outcome = 0
        for key, subject in new.items():
            first = sessions.get(subject["first_session"])
            if first is None or first["ended_at"] is None:
                missing_first += 1
                continue
            candidates = [match for match, p in valid.get(key, [])
                          if p["session"] == first["id"]
                          and first["started_at"] <= match["started_at"]
                          and match["ended_at"] <= first["ended_at"]]
            potential = [(match, p) for match, p in by_subject.get(key, [])
                         if match["cell"] == cell_id and p["session"] in (None, first["id"])
                         and first["started_at"] <= match["started_at"] <= first["ended_at"]]
            missing_first += int(any(p["session"] is None and
                                     (match["outcome"] == "unknown" or
                                      (match["outcome"] == "normal" and match["ended_at"] <= first["ended_at"]))
                                     for match, p in potential))
            missing_outcome += int(any(match["outcome"] == "unknown" for match, _ in potential))
            if candidates:
                activated[key] = candidates[0]
                known_end = candidates[0]["ended_at"]
                if any((match["outcome"] == "unknown" and match["started_at"] < known_end)
                       or (match["outcome"] == "normal" and p["session"] is None and match["ended_at"] < known_end)
                       for match, p in potential):
                    uncertain_activation.add(key)
        joins = [row for row in data["joins"] if row["subject"] in human_ids and row["cell"] == cell_id]
        entrants = {row["subject"] for row in joins if row["subject"] in new}
        # A join with missing first-session facts still remains in the denominator.
        exposed = (set(arrivals) & human_ids) | {row["subject"] for row in joins}
        exposed |= {key for key, rows in by_subject.items() if any(match["cell"] == cell_id for match, _ in rows)}
        cell_result = {"cell": dict(cell), "sample_humans": len(exposed),
                       "sample_groups": len({subjects[key]["group"] for key in exposed}),
                       "activation_groups": len({new[key]["group"] for key in activated}),
                       "exclusions": exclusions,
                       "activation": {"arrivals": ratio(len(activated), len(new)),
                                      "queue_entrants": ratio(len(set(activated) & entrants), len(entrants)),
                                      "missing_first_session": missing_first,
                                      "missing_match_outcome": missing_outcome,
                                      "missing_activation_time": len(uncertain_activation)}}
        outcome_counts = Counter(matches[key]["outcome"] for key in by_cell[cell_id])
        cell_result["completion"] = dict(ratio(outcome_counts["normal"], sum(outcome_counts.values())),
                                         incomplete=outcome_counts["active"] + outcome_counts["unknown"],
                                         outcomes=dict(sorted(outcome_counts.items())))
        for name, days in (("second_match", 7), ("d1", 1), ("d7", 7)):
            numerator = denominator = immature = missing = 0
            for key, activation in activated.items():
                moment = timestamp(activation["ended_at"])
                target = moment.date() + timedelta(days=days)
                cutoff = (moment + timedelta(days=7) if name == "second_match" else
                          datetime.combine(target + timedelta(days=1), time(), timezone.utc))
                if new[key]["observation"] == "missing" or key in uncertain_activation:
                    missing += 1
                    continue
                if watermark < cutoff:
                    immature += 1
                    continue
                candidates = [(match, p) for match, p in valid.get(key, [])
                              if match["id"] != activation["id"] and match["started_at"] >= activation["ended_at"]]
                if name == "second_match":
                    returned = any(p["voluntary"] is True and timestamp(match["ended_at"]) <= cutoff for match, p in candidates)
                    if not returned and any(p["voluntary"] is None and timestamp(match["ended_at"]) <= cutoff
                                            for match, p in candidates):
                        missing += 1
                        continue
                else:
                    returned = any(timestamp(match["ended_at"]).date() == target for match, _ in candidates)
                if not returned and any(match["cell"] == cell_id and match["outcome"] == "unknown"
                                        and timestamp(match["started_at"]) >= moment
                                        and (timestamp(match["started_at"]) <= cutoff if name == "second_match"
                                             else timestamp(match["started_at"]) < cutoff)
                                        and (name != "second_match" or p["voluntary"] is not False)
                                        for match, p in by_subject.get(key, [])):
                    missing += 1
                    continue
                denominator += 1
                numerator += int(returned)
            cell_result[name] = dict(ratio(numerator, denominator), immature=immature, missing=missing,
                                     excluded=len(arrivals) - len(new), activated=len(activated),
                                     coverage=denominator / len(activated) if activated else None)
        weekly_humans = {key for key, rows in valid.items() if len({timestamp(match["ended_at"]).date()
                         for match, _ in rows if week_start <= timestamp(match["ended_at"]).date() < week_end}) >= 2}
        cell_result["weekly"] = {"humans": len(weekly_humans),
                                 "groups": len({subjects[key]["group"] for key in weekly_humans}),
                                 "observation_mature": watermark >= datetime.combine(week_end, time(), timezone.utc),
                                 "missing_observation": sum(subjects[key]["observation"] == "missing" for key in valid if valid[key])}
        waits = [(timestamp(row["terminal_at"]) - timestamp(row["joined_at"])).total_seconds()
                 for row in joins if row["terminal_at"] is not None]
        starts = [row for row in joins if row["outcome"] == "started"]
        leaves = [row for row in joins if row["outcome"] == "left"]
        within = sum((timestamp(row["terminal_at"]) - timestamp(row["joined_at"])).total_seconds()
                     <= manifest["queue_timeout_seconds"] for row in starts)
        cell_result["queue"] = {"start_within_timeout": ratio(within, len(joins)),
                                "prestart_leave": ratio(len(leaves), len(joins)),
                                "unresolved": sum(row["outcome"] == "unresolved" for row in joins),
                                "terminal_wait_seconds": wait_summary(waits),
                                "leave_reasons": dict(sorted(Counter(row["reason"] for row in leaves).items()))}
        intentions = data["intentions"]
        cell_result["intentions"] = {"availability": "unavailable" if intentions is None else "supplied",
                                     "stages": {} if intentions is None else dict(sorted(Counter(row["stage"] for row in intentions
                                                       if row["subject"] in new).items())),
                                     "conversion": "unavailable", "attribution_window": "unavailable"}
        result["cells"].append(cell_result)
    return result


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def reject_constant(_value):
    raise FixtureError("invalid JSON constant")


def load_fixture(path):
    """Open without following links or blocking on FIFOs; bound bytes before parse."""
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    info = os.fstat(fd)
    if not stat.S_ISREG(info.st_mode) or info.st_size > MAX_INPUT_BYTES:
        os.close(fd)
        raise FixtureError("invalid fixture file")
    with os.fdopen(fd, "rb") as stream:
        raw = stream.read(MAX_INPUT_BYTES + 1)
    require(len(raw) <= MAX_INPUT_BYTES)
    try:
        data = json.loads(raw, object_pairs_hook=unique_object, parse_constant=reject_constant)
    except (ValueError, UnicodeError, RecursionError):
        raise FixtureError("invalid JSON fixture") from None
    validate(data)
    return data


def write_report(path, report):
    """Create a new private aggregate artifact; refuse overwrite or symlink."""
    payload = (json.dumps(report, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n").encode()
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as stream:
        os.fchmod(stream.fileno(), 0o600)
        stream.write(payload)
        stream.flush()
        os.fsync(stream.fileno())
    return hashlib.sha256(payload).hexdigest()


class PrivateArgumentParser(argparse.ArgumentParser):
    def error(self, message):
        self.exit(2, "synthetic_report_refused\n")


def main():
    parser = PrivateArgumentParser(description=__doc__, allow_abbrev=False)
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        digest = write_report(args.output, calculate(load_fixture(args.input)))
    except (FixtureError, OSError):
        print("synthetic_report_refused", file=sys.stderr)
        return 1
    print("synthetic_report_written sha256=" + digest)
    return 0


if __name__ == "__main__":
    sys.exit(main())
