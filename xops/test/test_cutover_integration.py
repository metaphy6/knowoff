"""Mandatory separate-cluster cutover proof and fixture boundary regressions."""
import json
import os
from pathlib import Path
import tempfile
import unittest


class CutoverFixtureSafetyTests(unittest.TestCase):
    def test_resource_identity_refuses_foreign_or_exposed_containers(self):
        import cutover_integration as proof
        receipt = proof.resource_receipt("a" * 24, os.getuid())
        state = {"Id": "b" * 64, "Name": "/" + receipt["prefix"] + "-source",
                 "Config": {"Labels": receipt["labels"], "Image": proof.snapshot.IMAGES["postgres"]},
                 "HostConfig": {"PortBindings": {}, "Binds": None},
                 "Mounts": [{"Type": "tmpfs"}],
                 "NetworkSettings": {"Networks": {receipt["prefix"]: {"NetworkID": "c" * 64}}}}
        proof.validate_container(receipt, state, "source", "b" * 64, "c" * 64)
        for change in [{"Id": "d" * 64}, {"Name": "/ordinary-postgres"},
                       {"Config": state["Config"] | {"Labels": {}}},
                       {"HostConfig": {"PortBindings": {"5432/tcp": [{}]}, "Binds": None}},
                       {"Mounts": [{"Type": "bind"}]},
                       {"NetworkSettings": {"Networks": {"ordinary": {"NetworkID": "c" * 64}}}}]:
            with self.subTest(change=list(change)), self.assertRaises(proof.snapshot.SnapshotError):
                proof.validate_container(receipt, state | change, "source", "b" * 64, "c" * 64)

    def test_owned_partial_creation_can_be_cleaned_before_network_attachment(self):
        import cutover_integration as proof
        receipt = proof.resource_receipt("a" * 24, os.getuid())
        state = {"Id": "b" * 64, "Name": "/" + receipt["prefix"] + "-source",
                 "Config": {"Labels": receipt["labels"], "Image": proof.snapshot.IMAGES["postgres"]},
                 "HostConfig": {"PortBindings": {}, "Binds": None}, "Mounts": [],
                 "NetworkSettings": {"Networks": {}}}
        proof.validate_container(receipt, state, "source", "b" * 64, "c" * 64, attached=False)
        with self.assertRaises(proof.snapshot.SnapshotError):
            proof.validate_container(receipt, state, "source", "b" * 64, "c" * 64)
        with self.assertRaises(proof.snapshot.SnapshotError):
            proof.validate_container(receipt, state | {"Id": "d" * 64}, "source", "b" * 64, "c" * 64, attached=False)

    def test_builder_cleanup_refuses_foreign_or_writable_mounts(self):
        import cutover_integration as proof
        receipt = proof.resource_receipt("a" * 24, os.getuid())
        state = {"Id": "b" * 64, "Name": "/" + receipt["prefix"] + "-builder",
                 "Config": {"Labels": receipt["labels"], "Image": "fixture-toolchain"},
                 "HostConfig": {"NetworkMode": "none", "PortBindings": {}},
                 "Mounts": [{"Source": str(proof.ROOT / "server"), "Destination": "/source", "RW": False}]}
        proof.validate_builder(receipt, state, "b" * 64, "fixture-toolchain")
        for change in [{"Id": "c" * 64}, {"Config": state["Config"] | {"Labels": {}}},
                       {"HostConfig": {"NetworkMode": "bridge"}},
                       {"Mounts": [state["Mounts"][0] | {"RW": True}]},
                       {"Mounts": [state["Mounts"][0] | {"Source": "/unrelated"}]}]:
            with self.subTest(change=list(change)), self.assertRaises(proof.snapshot.SnapshotError):
                proof.validate_builder(receipt, state | change, "b" * 64, "fixture-toolchain")

    def test_failure_never_writes_a_completion_receipt(self):
        from unittest.mock import patch
        import cutover_integration as proof
        class BrokenFixture:
            ids = {}
            closed = False
            def __init__(self, directory):
                pass
            def create(self):
                raise proof.snapshot.SnapshotError("private synthetic failure")
            def close(self):
                type(self).closed = True
        with tempfile.TemporaryDirectory(dir="/tmp/agent-runs") as work, patch.object(proof, "Fixture", BrokenFixture):
            with self.assertRaises(proof.snapshot.SnapshotError):
                proof.rehearse(Path(work))
            self.assertTrue(BrokenFixture.closed)
            self.assertFalse((Path(work) / "results.json").exists())
            failure = Path(work) / "failure.json"
            self.assertEqual(failure.stat().st_mode & 0o777, 0o600)
            self.assertNotIn("private synthetic failure", failure.read_text())

    def test_bad_nonce_is_refused_before_any_command(self):
        import cutover_integration as proof
        for nonce in ["", "../elsewhere", "a" * 23, "$(touch x)", "A" * 24]:
            with self.subTest(nonce=nonce), self.assertRaises(proof.snapshot.SnapshotError):
                proof.resource_receipt(nonce, os.getuid())

    def test_command_input_is_private_and_passed_on_stdin(self):
        import cutover_integration as proof
        class Recorder:
            def run(self, command, **kwargs):
                self.command, self.data = command, kwargs.get("data")
                return b'{"phase":"sealed"}\n'
        fixture = object.__new__(proof.Fixture)
        fixture.runner = Recorder()
        fixture.ids = {"helper": "b" * 64}
        fixture.roles = {"Roles": {"Database": "fixture", "Owner": "own", "Runtime": "rw", "Capture": "rd", "Migrator": "mg"}, "Control": "ctrl", "WritersEnabled": False}
        fixture.prefix = "fixture-prefix"
        fixture.validate = lambda: None
        request = {"proof": {"lease": "synthetic-private-lease"}}
        fixture.control("source", "seal", request)
        self.assertNotIn("synthetic-private-lease", " ".join(fixture.runner.command))
        self.assertEqual(json.loads(fixture.runner.data)["seal"], request)
        self.assertNotIn("shell", fixture.runner.command)


class CutoverIntegrationTests(unittest.TestCase):
    def test_real_separate_cluster_handoff_and_single_writer_replay(self):
        import cutover_integration as proof
        # Retain bounded private diagnostic artifacts if the proof fails.
        directory = Path(tempfile.mkdtemp(prefix="cutover-proof-", dir="/tmp/agent-runs"))
        result = proof.rehearse(directory)
        self.assertEqual(result["failures"], 0)
        self.assertEqual(result["skips"], 0)
        self.assertEqual(result["restores"], 2)
        self.assertEqual(result["held_event_applied"], 1)
        self.assertEqual(result["unacknowledged_outbox_retained"], 1)
        self.assertTrue(result["different_system_identifiers"])
        self.assertTrue(result["prepared_transaction_refused"])
        self.assertTrue(result["committed_handoff_survives_expiry"])
        self.assertGreater(result["domain_tables"], 60)
        self.assertTrue((directory / "results.json").is_file())
        print(json.dumps({"cutover_rehearsal": result}, sort_keys=True), flush=True)


if __name__ == "__main__":
    unittest.main()
