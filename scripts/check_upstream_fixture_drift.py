#!/usr/bin/env python3

from __future__ import annotations

import filecmp
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
LOCAL = ROOT / "testdata" / "upstream-head"
UPSTREAM_REPO = "https://github.com/openai/harmony.git"


def git_output(repo: Path, *args: str) -> str:
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True).strip()


def main() -> int:
    if not LOCAL.is_dir():
        raise SystemExit(f"missing local fixture dir: {LOCAL}")

    with tempfile.TemporaryDirectory(prefix="harmony-upstream-") as tmp:
        repo = Path(tmp) / "repo"
        subprocess.run(
            ["git", "clone", "--depth", "1", UPSTREAM_REPO, str(repo)],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        upstream = repo / "test-data"
        if not upstream.is_dir():
            raise SystemExit(f"missing upstream test-data dir: {upstream}")

        diff = filecmp.dircmp(LOCAL, upstream, ignore=["metadata.json"])
        _, diff_files, funny = filecmp.cmpfiles(
            LOCAL,
            upstream,
            diff.common_files,
            shallow=False,
        )
        changed = False
        if diff.left_only:
            changed = True
            print("local-only files:")
            for name in diff.left_only:
                print(f"  {name}")
        if diff.right_only:
            changed = True
            print("upstream-only files:")
            for name in diff.right_only:
                print(f"  {name}")
        if diff_files:
            changed = True
            print("content diffs:")
            for name in diff_files:
                print(f"  {name}")
        if funny:
            changed = True
            print("comparison errors:")
            for name in funny:
                print(f"  {name}")

        print(f"upstream HEAD: {git_output(repo, 'rev-parse', 'HEAD')}")
        print(f"upstream date: {git_output(repo, 'log', '-1', '--date=iso', '--format=%cd')}")
        print(f"upstream subject: {git_output(repo, 'log', '-1', '--format=%s')}")
        return 1 if changed else 0


if __name__ == "__main__":
    raise SystemExit(main())
