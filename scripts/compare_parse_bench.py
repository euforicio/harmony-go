#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
BASELINE = ROOT / "benchmarks" / "python" / "results" / "go_bench.txt"
TARGETS = {
    "BenchmarkParseToolCall",
    "BenchmarkStreamParseToolCall",
    "BenchmarkParseLargeCompletion",
    "BenchmarkStreamParseLargeCompletion",
}


def parse_bench_text(text: str) -> dict[str, dict[str, float]]:
    pattern = re.compile(
        r"^(Benchmark\S+)-\d+\s+\d+\s+([\d.]+)\s+ns/op\s+(\d+)\s+B/op\s+(\d+)\s+allocs/op$"
    )
    out: dict[str, dict[str, float]] = {}
    for line in text.splitlines():
        match = pattern.match(line.strip())
        if not match:
            continue
        name, ns_op, bytes_op, allocs_op = match.groups()
        if name not in TARGETS:
            continue
        out[name] = {
            "ns/op": float(ns_op),
            "B/op": float(bytes_op),
            "allocs/op": float(allocs_op),
        }
    return out


def run_current_bench() -> dict[str, dict[str, float]]:
    cmd = [
        "go",
        "test",
        "-run",
        "^$",
        "-bench",
        "^(BenchmarkParseToolCall|BenchmarkStreamParseToolCall|BenchmarkParseLargeCompletion|BenchmarkStreamParseLargeCompletion)$",
        "-benchmem",
        "-benchtime=200x",
        "./benchmarks/go",
    ]
    env = dict(**{"TIKTOKEN_OFFLINE": "1"}, **dict())
    proc = subprocess.run(
        cmd,
        cwd=ROOT,
        capture_output=True,
        text=True,
        env={**os.environ, **env},
        check=True,
    )
    sys.stdout.write(proc.stdout)
    return parse_bench_text(proc.stdout)


def pct_delta(current: float, baseline: float) -> float:
    return ((current - baseline) / baseline) * 100.0


def main() -> int:
    baseline = parse_bench_text(BASELINE.read_text())
    current = run_current_bench()

    missing = sorted(TARGETS - baseline.keys())
    if missing:
        print(f"missing baseline benchmarks: {', '.join(missing)}", file=sys.stderr)
        return 1
    missing = sorted(TARGETS - current.keys())
    if missing:
        print(f"missing current benchmarks: {', '.join(missing)}", file=sys.stderr)
        return 1

    summary: dict[str, dict[str, float]] = {}
    print("\nComparison vs benchmarks/python/results/go_bench.txt")
    for name in sorted(TARGETS):
        summary[name] = {}
        print(name)
        for metric in ("ns/op", "B/op", "allocs/op"):
            delta = pct_delta(current[name][metric], baseline[name][metric])
            summary[name][metric] = delta
            print(f"  {metric}: current={current[name][metric]:.0f} baseline={baseline[name][metric]:.0f} delta={delta:+.1f}%")

    print("\nJSON summary")
    print(json.dumps(summary, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
