"""Synthetic repair tests; never open the developer's Codex home."""
import contextlib
import importlib.util
import io
import json
import os
import queue
import subprocess
import threading
from pathlib import Path
import sqlite3
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("repair_codex_rollouts", Path(__file__).with_name("repair-codex-rollouts.py"))
repair = importlib.util.module_from_spec(spec)
spec.loader.exec_module(repair)
ID = "01a02317-c499-77e0-aea1-983624e8aadb"


@unittest.skipUnless(os.name == "posix", "repair apply requires POSIX file locking")
class RepairTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.old = self.root / "sessions/2026/11/06" / ("rollout-restored-" + ID + ".jsonl")
        self.old.parent.mkdir(parents=True)
        self.body = (json.dumps({"type": "session_meta", "timestamp": "2026-11-06T23:30:38Z", "payload": {
            "id": ID, "timestamp": "2026-08-21T13:52:35+07:00", "cwd": "/Users/gink/Workspaces/tools/netscape"}}) +
            '\n{"type":"world_state","payload":{"unchanged":true}}\n').encode()
        self.old.write_bytes(self.body)
        self.new = self.old.with_name("rollout-2026-08-21T06-52-35-" + ID + ".jsonl")
        self.databases = [self.root / "state_4.sqlite", self.root / "state_5.sqlite"]
        for path in self.databases:
            db = sqlite3.connect(path)
            db.execute("CREATE TABLE threads (id TEXT PRIMARY KEY, rollout_path TEXT, marker TEXT)")
            db.execute("INSERT INTO threads VALUES (?, ?, ?)", (ID, str(self.old), "preserve"))
            db.commit()
            db.close()
        self.index = self.root / "session_index.jsonl"
        self.index_raw = (json.dumps({"id": ID, "rollout_path": str(self.old), "thread_name": str(self.old)}) + "\n" +
                          json.dumps({"id": "other", "metadata": {"rolloutPath": str(self.old)}}) + "\n").encode()
        self.index.write_bytes(self.index_raw)

    def paths(self):
        result = []
        for path in self.databases:
            db = sqlite3.connect(path)
            result.append(db.execute("SELECT rollout_path, marker FROM threads WHERE id=?", (ID,)).fetchone())
            db.close()
        return result

    def apply(self):
        return repair.apply(*repair.plan(self.root))

    def test_preview_does_not_repair_or_backup(self):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            self.assertEqual(repair.main(["--codex-home", str(self.root), "--session", ID]), 0)
        self.assertIn("Preview only", out.getvalue())
        self.assertTrue(self.old.exists())
        self.assertFalse(self.new.exists())
        self.assertEqual(self.old.read_bytes(), self.body)
        self.assertEqual(self.index.read_bytes(), self.index_raw)
        self.assertEqual(self.paths(), [(str(self.old), "preserve")] * 2)
        self.assertEqual(list(self.root.glob("rollout-repair-backup-*")), [])

    def test_apply_updates_paths_backups_and_preserves_history(self):
        backup = self.apply()
        self.assertFalse(self.old.exists())
        self.assertEqual(self.new.read_bytes(), self.body)
        self.assertEqual(self.paths(), [(str(self.new), "preserve")] * 2)
        lines = [json.loads(line) for line in self.index.read_text().splitlines()]
        self.assertEqual(lines[0]["rollout_path"], str(self.new))
        self.assertEqual(lines[0]["thread_name"], str(self.old), "a title is not a native path reference")
        self.assertEqual(lines[1]["metadata"]["rolloutPath"], str(self.new))
        self.assertEqual((backup / self.old.relative_to(self.root)).read_bytes(), self.body)
        self.assertEqual((backup / self.index.name).read_bytes(), self.index_raw)
        for path in self.databases:
            db = sqlite3.connect(backup / path.name)
            self.assertEqual(db.execute("SELECT rollout_path FROM threads").fetchone()[0], str(self.old))
            db.close()
        self.assertEqual(json.loads((backup / "manifest.json").read_text())["repairs"][0]["id"], ID)
        self.assertEqual(repair.plan(self.root)[1], [])
        self.assertIsNone(self.apply())
        self.assertEqual(len(list(self.root.glob("rollout-repair-backup-*"))), 1)

    def test_backup_includes_sqlite_wal_data(self):
        db = sqlite3.connect(self.databases[1])
        self.addCleanup(db.close)
        db.execute("PRAGMA journal_mode=WAL")
        db.execute("UPDATE threads SET marker='in WAL'")
        db.commit()
        backup = self.apply()
        restored = sqlite3.connect(backup / self.databases[1].name)
        try:
            self.assertEqual(restored.execute("SELECT marker FROM threads").fetchone()[0], "in WAL")
        finally:
            restored.close()

    def test_conflicting_target_is_never_overwritten(self):
        self.new.write_bytes(b"different session data")
        with self.assertRaisesRegex(ValueError, "destination already exists"):
            self.apply()
        self.assertEqual(self.old.read_bytes(), self.body)
        self.assertEqual(self.new.read_bytes(), b"different session data")
        self.assertEqual(self.paths()[0][0], str(self.old))

    def test_mismatched_metadata_id_refused(self):
        self.old.write_bytes(self.body.replace(ID.encode(), b"11111111-1111-1111-1111-111111111111"))
        with self.assertRaisesRegex(ValueError, "ID differs"):
            self.apply()
        self.assertFalse(self.new.exists())

    def test_symlink_source_or_database_refused(self):
        original = self.old.read_bytes()
        target = self.root / "outside.jsonl"
        target.write_bytes(original)
        self.old.unlink()
        self.old.symlink_to(target)
        with self.assertRaisesRegex(ValueError, "symlink refused"):
            self.apply()
        self.old.unlink()
        self.old.write_bytes(original)
        db = self.databases[0]
        elsewhere = self.root / "outside.sqlite"
        db.rename(elsewhere)
        db.symlink_to(elsewhere)
        with self.assertRaisesRegex(ValueError, "symlink refused"):
            self.apply()
        self.assertEqual(target.read_bytes(), original)

    def test_active_writer_lock_refused(self):
        import fcntl
        lock = self.root / "thread-writer-locks" / (ID + ".lock")
        lock.parent.mkdir()
        with lock.open("wb") as owner:
            fcntl.flock(owner, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaisesRegex(ValueError, "still holds this session"):
                self.apply()
        self.assertFalse(self.new.exists())
        self.assertEqual(list(self.root.glob("rollout-repair-backup-*")), [])

    def test_changed_source_since_preview_refused(self):
        planned = repair.plan(self.root)
        self.old.write_bytes(self.body + b"\n")
        with self.assertRaisesRegex(ValueError, "changed since preview"):
            repair.apply(*planned)
        self.assertFalse(self.new.exists())

    def test_wrong_index_id_refused(self):
        db = sqlite3.connect(self.databases[0])
        db.execute("UPDATE threads SET id='wrong'")
        db.commit()
        db.close()
        with self.assertRaisesRegex(ValueError, "different ID"):
            self.apply()
        self.assertFalse(self.new.exists())

    def test_aliased_database_path_refused_before_mutation(self):
        db = sqlite3.connect(self.databases[0])
        db.execute("UPDATE threads SET rollout_path=?", ("/another/codex/home/" + self.old.name,))
        db.commit()
        db.close()
        with self.assertRaisesRegex(ValueError, "path differs"):
            self.apply()
        self.assertTrue(self.old.exists())
        self.assertFalse(self.new.exists())

    def test_aliased_json_index_path_refused_before_mutation(self):
        self.index.write_text(json.dumps({"id": ID, "rollout_path": "/another/home/" + self.old.name}) + "\n")
        with self.assertRaisesRegex(ValueError, "different legacy path"):
            self.apply()
        self.assertTrue(self.old.exists())
        self.assertFalse(self.new.exists())

    def test_partial_database_failure_retains_both_paths_and_rerun_finishes(self):
        original_connect = repair.connect
        def fail_second_write(path, readonly=False):
            if path == self.databases[1] and not readonly:
                raise sqlite3.OperationalError("injected second-index failure")
            return original_connect(path, readonly)
        with patch.object(repair, "connect", side_effect=fail_second_write):
            with self.assertRaisesRegex(RuntimeError, "repair incomplete"):
                self.apply()
        self.assertTrue(os.path.samefile(self.old, self.new))
        self.assertEqual(self.paths(), [(str(self.new), "preserve"), (str(self.old), "preserve")])
        self.assertEqual(self.index.read_bytes(), self.index_raw)
        self.apply()
        self.assertFalse(self.old.exists())
        self.assertEqual(self.new.read_bytes(), self.body)
        self.assertEqual(self.paths(), [(str(self.new), "preserve")] * 2)

    def test_missing_timestamp_refused(self):
        self.old.write_text(json.dumps({"type": "session_meta", "payload": {"id": ID}}) + "\n")
        with self.assertRaisesRegex(ValueError, "no valid session timestamp"):
            self.apply()

    def test_target_session_filter_leaves_other_records_alone(self):
        self.assertEqual(repair.plan(self.root, "11111111-1111-1111-1111-111111111111")[1], [])
        self.assertTrue(self.old.exists())


@unittest.skipUnless(os.environ.get("CODEX_ROLLOUT_TEST_BIN"), "set CODEX_ROLLOUT_TEST_BIN for a real, no-inference resume smoke test")
class NativeResumeTests(unittest.TestCase):
    def test_repaired_paginated_rollout_resumes_by_id(self):
        with tempfile.TemporaryDirectory(prefix="codex-restore-native-") as temporary:
            root = Path(temporary)
            folder = root / "sessions/2026/11/06"
            folder.mkdir(parents=True)
            old = folder / ("rollout-restored-" + ID + ".jsonl")
            meta = {"id": ID, "timestamp": "2026-08-21T13:52:35Z", "cwd": str(root),
                    "originator": "restore_regression", "cli_version": "0.167.1", "source": "cli",
                    "model_provider": "openai", "history_mode": "paginated",
                    "base_instructions": {"text": "Synthetic offline resume check."}}
            body = (json.dumps({"ordinal": 0, "type": "session_meta", "timestamp": meta["timestamp"], "payload": meta}) + "\n").encode()
            old.write_bytes(body)

            def resume(path=None):
                env = dict(os.environ, CODEX_HOME=str(root), CODEX_SQLITE_HOME=str(root))
                env.pop("AGENT_SESSIONS_CODEX", None)
                with (root / "stderr.log").open("w") as log:
                    process = subprocess.Popen([os.environ["CODEX_ROLLOUT_TEST_BIN"], "app-server"],
                                               cwd=root, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                               stderr=log, text=True)
                    responses = queue.Queue()
                    def read():
                        for line in process.stdout:
                            responses.put(json.loads(line))
                        responses.put(None)
                    reader = threading.Thread(target=read, daemon=True)
                    reader.start()
                    def request(number, method, params):
                        process.stdin.write(json.dumps({"id": number, "method": method, "params": params}) + "\n")
                        process.stdin.flush()
                        while True:
                            result = responses.get(timeout=15)
                            if result is None:
                                raise RuntimeError("Codex app-server exited before responding")
                            if result.get("id") == number:
                                return result
                    try:
                        initialized = request(1, "initialize", {"clientInfo": {"name": "restore_regression", "version": "1"},
                                                                 "capabilities": {"experimentalApi": True}})
                        self.assertIn("result", initialized)
                        params = {"threadId": ID, "cwd": str(root)}
                        if path:
                            params["path"] = str(path)
                        return request(2, "thread/resume", params)
                    finally:
                        process.stdin.close()
                        try:
                            process.wait(timeout=3)
                        except subprocess.TimeoutExpired:
                            process.kill()
                            process.wait()
                        reader.join(timeout=3)
                        process.stdout.close()

            failed = resume(old)
            self.assertIn("canonical rollout filename", failed["error"]["message"])
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(repair.main(["--codex-home", str(root), "--apply"]), 0)
            new = folder / ("rollout-2026-08-21T13-52-35-" + ID + ".jsonl")
            self.assertEqual(new.read_bytes(), body)
            self.assertIn("result", resume(), "repaired session must resume by ID, not only an explicit path")


if __name__ == "__main__":
    unittest.main()
