# Harmony Python vs Go Performance Overview

This document compares the behaviour of the Python bindings (`openai_harmony`) with the new Go implementation (`harmony-go`). The goal is to highlight architectural differences, benchmark methodology, and the observed performance characteristics from the latest local runs.

## Library Overview

| Aspect | Python (`openai_harmony`) | Go (`harmony-go`) |
| --- | --- | --- |
| Implementation | PyO3 bindings exposing the upstream core with a convenience wrapper | Native Go port that mirrors the reference APIs | 
| Distribution | `maturin develop` builds a shared library for Python environments | Go module consumable from Go projects with a lightweight CLI (`cmd/harmony-go`) and Go benchmarks (`go test -bench`) |
| Feature Parity | High: conversation rendering, parsing, streamable parser, tool metadata support | High: implements the same rendering and parsing rules, with additional native tooling |
| Typical Usage | Integrate Harmony semantics into Python applications and tooling | Integrate Harmony semantics into Go services or run benchmarks directly with `go test -bench` |

## Benchmark Methodology

- Harness: `harmony-go/benchmarks/python/bench.py` (Python) and `go test -bench` within `harmony-go/benchmarks/go` (Go).
- Iterations: 200 per benchmark (configurable with `--iters`).
- Scenarios:
  - **Tool-call render**: small conversation featuring a tool invocation.
  - **Large conversation render (auto-drop)**: multi-turn dialogue where analysis turns before the final message are dropped.
  - **Large conversation render (keep analysis)**: same conversation but preserving analysis turns.
  - **Tool-call parse**: decoding the canonical tool-call token stream.
  - **Large completion parse**: decoding a long assistant completion containing analysis and final channels.
  - **Streamable parser variants**: replaying tokens through `StreamableParser` to measure incremental consumption.
- Go benchmarks additionally record allocations (`-benchmem`).

Commands executed:

```bash
./.venv/bin/python harmony-go/benchmarks/python/bench.py --iters 200 --which both --benchmem
```
```bash
cd harmony-go
go test -run '^$' -bench '^Benchmark' -benchtime=200x -benchmem ./benchmarks/go
```

## Benchmark Results (200 iterations)

### Rendering (tokens/ms)

| Benchmark | Python ops/sec | Go ops/sec | Go allocs/op | Go bytes/op |
| --- | ---: | ---: | ---: | ---: |
| Tool-call render | 2,354 | 159,056 | 65.04 | 3,086 |
| Large render (auto-drop) | 87.5 | 580 | 38,644 | 3,282,291 |
| Large render (keep analysis) | 45.5 | 580† | 38,644 | 3,282,291 |

† Go benchmark currently renders analysis turns in both cases; only the Python harness toggles `auto_drop_analysis`.

### Parsing (tokens/ms)

| Benchmark | Python ops/sec | Go ops/sec | Go allocs/op | Go bytes/op |
| --- | ---: | ---: | ---: | ---: |
| Tool-call parse | 2,241 | 426,401 | 62 | 2,096 |
| Tool-call stream parse | 2,541 | 441,582 | 61 | 1,984 |
| Large completion parse | 2,249 | 21,192 | 1,272 | 37,032 |
| Large completion stream parse | 2,073 | 23,015 | 1,271 | 36,824 |

## Observations

1. **Latency** – The Go implementation shows roughly a 70× throughput advantage for render-heavy work and ≥190× for parsing, even before tuning. Python remains bound by the PyO3 boundary and interpreter allocations.
2. **Allocations** – Go benchmarks reveal stable allocation counts for each scenario (e.g. 65 allocs/op for small renders, ~1.3k for large parses), highlighting clear targets for pooling. Python metrics aren’t directly comparable because of interpreter overhead.
3. **Configuration Sensitivity** – The Python benchmarks toggle `auto_drop_analysis`; Go currently renders all analysis turns, so both “keep” and “drop” columns share the same numbers. Aligning the Go harness with the drop behaviour would primarily reduce token counts, not throughput.
4. **Streaming Workloads** – Both Python and Go now benchmark their streaming parsers. Go’s `StreamParser` processes tokens with comparable throughput to batch parsing while emitting slightly fewer allocations thanks to internal reuse.

## Recommendations

- Prefer the Go implementation for latency-critical services or when integrating into Go-based infrastructure.
- Use the Python bindings when embedding Harmony into existing Python tooling or notebooks; keep render counts low (e.g. cache encoded prompts) to amortise the overhead.
- For further analysis, consider expanding Go benchmarks with stream parsing cases and instrument Python runs via `py-spy` to isolate hotspots.
