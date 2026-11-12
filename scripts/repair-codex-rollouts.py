#!/usr/bin/env python3
"""Repair Agent Sessions' legacy rollout-restored filenames (offline, POSIX).

Preview by default. Close Codex clients and Agent Sessions before --apply.
Only Python's standard library is needed; histories are never rewritten.
"""
import argparse
from contextlib import ExitStack
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import shutil
import sqlite3
import sys
import tempfile

LEGACY = re.compile(r"rollout-restored-([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})\.jsonl", re.I)
STATE = re.compile(r"state_[0-9]+\.sqlite")


def check_path(root, path):
    """Reject symlinks below the explicitly selected, resolved Codex root."""
    parts = path.relative_to(root).parts
    current = root
    for part in parts:
        current = current / part
        if current.is_symlink():
            raise ValueError("symlink refused: " + str(current))
    if path.exists() and not path.is_file():
        raise ValueError("not a regular file: " + str(path))


def identity(path):
    st = path.stat()
    return (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns)


def canonical_name(path, session_id):
    with path.open("rb") as stream:
        while True:
            line = stream.readline(64 * 1024 * 1024 + 1)
            if not line:
                break
            if len(line) > 64 * 1024 * 1024:
                raise ValueError("oversized rollout record: " + str(path))
            record = json.loads(line) if line.strip() else {}
            if not isinstance(record, dict):
                raise ValueError("invalid rollout record: " + str(path))
            if record.get("type") != "session_meta":
                continue
            payload = record.get("payload", {})
            if not isinstance(payload, dict):
                raise ValueError("invalid session metadata: " + str(path))
            if str(payload.get("id", "")).lower() != session_id.lower():
                raise ValueError("session metadata ID differs from filename: " + str(path))
            for value in (payload.get("timestamp"), record.get("timestamp")):
                try:
                    stamp = datetime.fromisoformat(value.replace("Z", "+00:00"))
                    if stamp.tzinfo is None:
                        continue
                    stamp = stamp.astimezone(timezone.utc)
                    return "rollout-{}-{}.jsonl".format(stamp.strftime("%Y-%m-%dT%H-%M-%S"), session_id)
                except (AttributeError, TypeError, ValueError):
                    continue
            raise ValueError("no valid session timestamp: " + str(path))
    raise ValueError("no session_meta record: " + str(path))


def connect(path, readonly=False):
    return sqlite3.connect(path.as_uri() + ("?mode=ro" if readonly else "?mode=rw"), uri=True, timeout=2)


def has_paths(db):
    return {"id", "rollout_path"}.issubset({row[1] for row in db.execute('PRAGMA table_info("threads")')})


def index_data(raw, paths):
    """Change exact native path properties only; preserve other index fields."""
    selected = {LEGACY.fullmatch(Path(old).name)[1].lower() for old in paths}
    def rewrite(value):
        if isinstance(value, dict):
            for key, item in value.items():
                if key in ("rollout_path", "rolloutPath") and isinstance(item, str):
                    legacy = LEGACY.fullmatch(Path(item).name)
                    if legacy and legacy[1].lower() in selected and item not in paths:
                        raise ValueError("JSON index uses a different legacy path: " + item)
            return {key: paths.get(item, item) if key in ("rollout_path", "rolloutPath") and isinstance(item, str)
                    else rewrite(item) for key, item in value.items()}
        if isinstance(value, list):
            return [rewrite(item) for item in value]
        return value
    result = []
    for line in raw.splitlines(keepends=True):
        if not line.strip():
            result.append(line)
            continue
        original = json.loads(line)
        updated = rewrite(original)
        result.append((json.dumps(updated, ensure_ascii=False) + "\n").encode() if updated != original else line)
    return b"".join(result)


def validate_rows(db, repairs):
    for repair in repairs:
        for (stored_path,) in db.execute('SELECT rollout_path FROM threads WHERE lower(id) = lower(?)', (repair["id"],)):
            if stored_path and stored_path not in (str(repair["old"]), str(repair["new"])):
                raise ValueError("index path differs from repair source: " + str(stored_path))
        for (thread_id,) in db.execute('SELECT id FROM threads WHERE rollout_path = ?', (str(repair["old"]),)):
            if str(thread_id).lower() != repair["id"].lower():
                raise ValueError("index associates rollout with a different ID: " + str(repair["old"]))


def plan(root, session=None):
    root = Path(root).expanduser().resolve(strict=True)
    repairs = []
    destinations = set()
    for subdir in ("sessions", "archived_sessions"):
        base = root / subdir
        if base.is_symlink():
            raise ValueError("symlink refused: " + str(base))
        if not base.exists():
            continue
        for old in sorted(base.rglob("rollout-restored-*.jsonl")):
            match = LEGACY.fullmatch(old.name)
            if not match or (session and match[1].lower() != session.lower()):
                continue
            check_path(root, old)
            session_id = match[1]
            new = old.with_name(canonical_name(old, session_id))
            check_path(root, new)
            if new in destinations:
                raise ValueError("multiple restored files target " + str(new))
            destinations.add(new)
            if new.exists() and not os.path.samefile(old, new):
                raise ValueError("destination already exists: " + str(new))
            repairs.append({"id": session_id, "old": old, "new": new, "identity": identity(old)})
    databases = []
    for path in sorted(root.glob("state_*.sqlite")):
        if not STATE.fullmatch(path.name):
            continue
        check_path(root, path)
        for suffix in ("-wal", "-shm", "-journal"):
            check_path(root, Path(str(path) + suffix))
        db = connect(path, readonly=True)
        try:
            if not has_paths(db):
                continue
            validate_rows(db, repairs)
            databases.append(path)
        finally:
            db.close()
    index = root / "session_index.jsonl"
    check_path(root, index)
    paths = {str(r["old"]): str(r["new"]) for r in repairs}
    index_raw = index.read_bytes() if index.exists() else None
    index_updated = index_data(index_raw, paths) if index_raw is not None else None
    return root, repairs, databases, index_raw, index_updated


def apply(root, repairs, databases, index_raw, index_updated):
    if not repairs:
        return None
    try:
        import fcntl
    except ImportError:
        raise ValueError("--apply requires POSIX file locking (macOS or Linux)")
    with ExitStack() as stack:
        for repair in repairs:
            check_path(root, repair["old"])
            lock = root / "thread-writer-locks" / (repair["id"] + ".lock")
            check_path(root, lock)
            if lock.exists():
                stream = stack.enter_context(lock.open("rb"))
                try:
                    fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError:
                    raise ValueError("Codex still holds this session; close its clients first: " + repair["id"])
            if identity(repair["old"]) != repair["identity"]:
                raise ValueError("rollout changed since preview: " + str(repair["old"]))
        # Preserve histories, including a consistent snapshot of SQLite WAL data.
        backup = Path(tempfile.mkdtemp(prefix="rollout-repair-backup-", dir=root))
        os.chmod(backup, 0o700)
        for repair in repairs:
            target = backup / repair["old"].relative_to(root)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(repair["old"], target)
        index = root / "session_index.jsonl"
        if index_raw is not None:
            (backup / index.name).write_bytes(index_raw)
        for path in databases:
            check_path(root, path)
            for suffix in ("-wal", "-shm", "-journal"):
                check_path(root, Path(str(path) + suffix))
            src = connect(path, readonly=True)
            dst = sqlite3.connect(backup / path.name)
            try:
                src.backup(dst)
            finally:
                dst.close()
                src.close()
        (backup / "manifest.json").write_text(json.dumps({"repairs": [
            {key: str(r[key]) for key in ("id", "old", "new")} for r in repairs]}, indent=2) + "\n")
        # Keep legacy paths until every reference has been updated. If any step
        # fails, both names remain valid and a subsequent run can finish safely.
        try:
            for repair in repairs:
                check_path(root, repair["old"])
                check_path(root, repair["new"])
                if identity(repair["old"]) != repair["identity"]:
                    raise ValueError("rollout changed during backup: " + str(repair["old"]))
                if repair["new"].exists():
                    if not os.path.samefile(repair["old"], repair["new"]):
                        raise ValueError("destination appeared: " + str(repair["new"]))
                else:
                    os.link(repair["old"], repair["new"])
            for path in databases:
                check_path(root, path)
                for suffix in ("-wal", "-shm", "-journal"):
                    check_path(root, Path(str(path) + suffix))
                db = connect(path)
                try:
                    db.execute("BEGIN IMMEDIATE")
                    validate_rows(db, repairs)
                    for repair in repairs:
                        db.execute('UPDATE threads SET rollout_path = ? WHERE rollout_path = ?',
                                   (str(repair["new"]), str(repair["old"])))
                    db.commit()
                finally:
                    db.close()
            if index_raw != index_updated:
                check_path(root, index)
                if index.read_bytes() != index_raw:
                    raise ValueError("session index changed during repair")
                fd, temporary = tempfile.mkstemp(prefix=".rollout-index-", dir=root)
                try:
                    with os.fdopen(fd, "wb") as stream:
                        stream.write(index_updated)
                        stream.flush()
                        os.fsync(stream.fileno())
                    os.replace(temporary, index)
                finally:
                    if os.path.exists(temporary):
                        os.unlink(temporary)
            for repair in repairs:
                if not os.path.samefile(repair["old"], repair["new"]):
                    raise ValueError("rollout changed before cleanup: " + str(repair["old"]))
                repair["old"].unlink()
        except Exception as error:
            raise RuntimeError("repair incomplete; existing paths retained where possible; backups at {}: {}".format(backup, error)) from error
        return backup


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex-home", default=os.environ.get("CODEX_HOME", "~/.codex"))
    parser.add_argument("--session", help="repair only this session UUID")
    parser.add_argument("--apply", action="store_true", help="apply after closing Codex clients and Agent Sessions; otherwise preview only")
    args = parser.parse_args(argv)
    if args.session and not LEGACY.fullmatch("rollout-restored-" + args.session + ".jsonl"):
        parser.error("--session must be a UUID")
    try:
        root, repairs, databases, raw, updated = plan(args.codex_home, args.session)
        for repair in repairs:
            print("{}\n  -> {}".format(repair["old"], repair["new"]))
        print("{} rollout(s); {} SQLite index(es). {}".format(len(repairs), len(databases), "Applying offline repair." if args.apply else "Preview only; no repair applied."))
        if args.apply:
            backup = apply(root, repairs, databases, raw, updated)
            print("Repair complete." + (" Backups: " + str(backup) if backup else " Nothing to repair."))
        return 0
    except (OSError, ValueError, RuntimeError, sqlite3.Error) as error:
        print("Repair failed: " + str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
