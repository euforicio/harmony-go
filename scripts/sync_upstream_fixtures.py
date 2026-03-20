#!/usr/bin/env python3

from __future__ import annotations

import json
import shutil
import subprocess
from pathlib import Path


UPSTREAM_REPO = "https://github.com/openai/harmony.git"
DEFAULT_UPSTREAM_DIR = Path("/tmp/harmony-upstream")
ROOT = Path(__file__).resolve().parents[1]
OUT_DIR = ROOT / "testdata" / "upstream-head"


def ensure_upstream_repo() -> Path:
    if DEFAULT_UPSTREAM_DIR.exists():
        return DEFAULT_UPSTREAM_DIR
    subprocess.run(
        ["git", "clone", "--depth", "1", UPSTREAM_REPO, str(DEFAULT_UPSTREAM_DIR)],
        check=True,
    )
    return DEFAULT_UPSTREAM_DIR


def git_output(repo: Path, *args: str) -> str:
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True).strip()


def main() -> None:
    upstream = ensure_upstream_repo()
    src_dir = upstream / "test-data"
    if not src_dir.is_dir():
        raise SystemExit(f"missing upstream fixture dir: {src_dir}")

    OUT_DIR.mkdir(parents=True, exist_ok=True)

    for src in sorted(src_dir.glob("*.txt")):
        shutil.copyfile(src, OUT_DIR / src.name)

    metadata = {
        "repo": UPSTREAM_REPO,
        "commit": git_output(upstream, "rev-parse", "HEAD"),
        "date": git_output(upstream, "log", "-1", "--date=iso", "--format=%cd"),
        "subject": git_output(upstream, "log", "-1", "--format=%s"),
    }
    (OUT_DIR / "metadata.json").write_text(
        json.dumps(metadata, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )


if __name__ == "__main__":
    main()
