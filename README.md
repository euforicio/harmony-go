<div align="center">

[<img alt="Harmony Go" src="docs/harmony-go-logo.svg" width="420" />](docs/harmony-go-logo.svg)

[![Go Reference](https://pkg.go.dev/badge/github.com/euforicio/harmony-go.svg)](https://pkg.go.dev/github.com/euforicio/harmony-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/euforicio/harmony-go)](https://goreportcard.com/report/github.com/euforicio/harmony-go)
![Build](https://github.com/euforicio/harmony-go/actions/workflows/go.yml/badge.svg)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

---

Harmony Go — a blazing‑fast, 100% Go implementation of the OpenAI Harmony message format. Render conversations to tokens, parse model outputs back to structured messages, and stream‑parse tokens with near‑zero overhead — no CGO, no FFI, no Python.

Built for production Go services that demand low latency and high throughput while staying fully aligned with the upstream Harmony semantics used by the Rust/Python stacks.

## Contents
- What Is Harmony Go
- Why Developers Choose It
- Features
- Installation
- Quick Start (Library)
- CLI Usage
- Performance (Benchmarks & Repro)
- Testing & Parity
- Configuration
- Development
- Roadmap
- License & Acknowledgements

## What Is Harmony Go
Harmony Go provides the core Harmony primitives in pure Go:
- Render a single message or entire conversation to Harmony tokens.
- Render completion/training prompts with correct headers and stop tokens.
- Parse completion tokens back into structured messages.
- Incrementally stream‑parse tokens as they arrive.

It mirrors the reference semantics of the upstream implementation while offering native Go ergonomics and performance.

## Why Developers Choose It
- Extreme performance: up to 70× faster rendering and ≥190× faster parsing vs the official Python bindings in our local runs; streaming matches batch throughput. See full benchmarks below.
- Zero FFI overhead: single static binary; deploy anywhere Go runs.
- Spec‑aligned: channels, tool calls, training substitution, and content types.
- Concurrency‑friendly: designed for high‑throughput servers and pipelines.
- Easy integration: Go module + simple CLI for testing and pipelines.

## Features
- Render: `Render`, `RenderConversation`, `RenderConversationForCompletion`, `RenderConversationForTraining`.
- Parse: `ParseMessagesFromCompletionTokens` for batch; `NewStreamParser` for incremental streaming.
- Token helpers: `StopTokens`, `StopTokensForAssistantActions`, `DecodeUTF8`/`DecodeBytes`.
- Tools & channels: correct formatting tokens, `channel`, `recipient`, and `content_type` handling.
- No external deps: ships with O200k tokenizer integration and Harmony specials.

## Installation
- Library (Go 1.25+): `go get github.com/euforicio/harmony-go`
- CLI: `go install github.com/euforicio/harmony-go/cmd/harmony-go@latest`

## Quick Start (Library)

```go
import (
    harmony "github.com/euforicio/harmony-go"
)

func example() error {
    enc, err := harmony.LoadEncoding(harmony.HarmonyGptOss)
    if err != nil { return err }

    conv := harmony.Conversation{Messages: []harmony.Message{
        {Author: harmony.Author{Role: harmony.RoleUser},
         Content: []harmony.Content{{Type: harmony.ContentText, Text: "What’s the weather in SF?"}}},
    }}

    // Render a completion prompt for the assistant
    toks, err := enc.RenderConversationForCompletion(conv, harmony.RoleAssistant, nil)
    if err != nil { return err }

    // Stream‑parse a hypothetical model response (tokens -> messages)
    parser, _ := harmony.NewStreamParser(enc, &harmony.RoleAssistant)
    for _, t := range toks { _ = parser.Process(t) }
    _ = parser.ProcessEOS()
    msgs := parser.Messages()
    _ = msgs
    return nil
}
```

## CLI Usage

The CLI accepts JSON on stdin and writes JSON to stdout. Commands are discoverable via `harmony-go`.

```bash
# List stop tokens
harmony-go stop

# Render a single message
echo '{"role":"user","content":[{"type":"text","text":"ping"}]}' | \
  harmony-go render-msg

# Render a full conversation
echo '{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}' | \
  harmony-go render-convo

# Render a conversation for completion (assistant next)
echo '{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}' | \
  harmony-go render-completion -role assistant

# Render a conversation for training (substitutes trailing <|end|> with <|return|> for assistant:final)
echo '{"messages":[{"role":"assistant","channel":"final","content":[{"type":"text","text":"ok"}]}]}' | \
  harmony-go render-training

# Parse tokens back to messages
echo '[1,2,3,4]' | harmony-go parse -role assistant

# Decode raw tokens into text (debugging)
echo '[200014]' | harmony-go decode
```

## Performance (Benchmarks & Repro)

Harmony Go is engineered for low allocations and high throughput. In local runs, it consistently outperforms the official Python bindings for render/parse‑heavy workloads. Streaming parse matches batch throughput while shaving a few allocations.

Benchmarks below are from the cross‑parity harness in `benchmarks/python/bench.py`, Go benchmarks in `benchmarks/go`, and a Rust micro‑bench harness in `benchmarks/rust` at 200 iterations. Full details: `docs/python_go_performance.md`.

Rendering

| Benchmark | Python ops/sec | Go ops/sec | Go allocs/op | Go bytes/op |
| --- | ---: | ---: | ---: | ---: |
| Tool‑call render | 2,354 | 159,056 | 65 | 3,086 |
| Large render (auto‑drop) | 87.5 | 580 | 38,644 | 3,282,291 |
| Large render (keep analysis) | 45.5 | 580† | 38,644 | 3,282,291 |

† Go renders analysis turns in both cases; the Python harness toggles `auto_drop_analysis`.

Parsing

| Benchmark | Python ops/sec | Go ops/sec | Go allocs/op | Go bytes/op |
| --- | ---: | ---: | ---: | ---: |
| Tool‑call parse | 2,241 | 426,401 | 62 | 2,096 |
| Tool‑call stream parse | 2,541 | 441,582 | 61 | 1,984 |
| Large completion parse | 2,249 | 21,192 | 1,272 | 37,032 |
| Large completion stream parse | 2,073 | 23,015 | 1,271 | 36,824 |

Observed speedups range from ~6.6× (large render) up to ≥190× (tool‑call parse).

Rust vs Go (micro‑benches)

- Machine: Apple M2 Ultra, macOS, 200 iterations (Sep 21, 2025)
- Metric: ops/sec (higher is better). Go also shows allocs/bytes for context.

| Benchmark | Rust ops/sec | Go ops/sec | Go allocs/op | Go bytes/op |
| --- | ---: | ---: | ---: | ---: |
| Render tool‑call | 2,467 | 225,428 | 14 | 1,014 |
| Render large (keep analysis) | 48.19 | 537.70 | 58,016 | 3,528,189 |
| Parse tool‑call | 2,466 | 288,684 | 55 | 2,280 |
| Stream parse tool‑call | 2,491 | 421,941 | 55 | 2,280 |
| Parse large completion | 2,463 | 29,697 | 851 | 46,200 |
| Stream parse large completion | 2,405 | 29,882 | 851 | 46,200 |

Notes:
- The Rust harness uses the upstream `openai-harmony` crate (git). For the large render case we report the “keep analysis” variant to match the Rust public API (no auto‑drop toggle exported).
- In these runs, the Go implementation significantly outperforms the Rust crate for these specific render/parse paths on this machine. Your numbers may vary.

Reproduce locally

- Python + Go harness: `./.venv/bin/python harmony-go/benchmarks/python/bench.py --iters 200 --which both --benchmem`
- Go only: `go test -run '^$' -bench '^Benchmark' -benchtime=200x -benchmem ./benchmarks/go`
- Rust only: `cargo run --release --manifest-path benchmarks/rust/Cargo.toml -- --iters 200 --which both | tee benchmarks/python/results/rust.json`

Notes on Rust: the upstream Harmony core is written in Rust. Harmony Go mirrors its semantics and avoids cross‑language overhead in Go services. We do not claim to outperform the Rust core; the gains shown here are vs the Python bindings and from removing FFI/bridge costs in Go deployments.

### Latest improvements (Sep 2025)
- Zero‑copy encode entry points reduce intermediate slices.
- Sequential render path appends into a single slice.
- Streaming parser reuses buffers to avoid per‑token string allocs.
- Cached formatting token IDs remove repeated lookups.
- Optional arena‑backed decoder storage (see below).

Typical results on Apple M2 (arm64), Go 1.25:
- Render tool‑call: bytes/op −59% (≈3048 → ≈1264), allocs/op −72% (≈65 → ≈18), ~24% faster.
- Parse large completion: ~45% faster, bytes/op −26%, allocs/op −96%.

### Experimental arenas (Go)
Build with `GOEXPERIMENT=arenas` to place decoder storage in an arena. This primarily reduces long‑lived heap objects and GC pressure in long‑running services. See `docs/ARENAS.md` for guidance.

## Testing & Parity
- Unit tests: `go test ./...`
- Benchmarks: `go test -run '^$' -bench '^Benchmark' -benchmem ./benchmarks/go`
- Lint: `golangci-lint run ./...`
- Coverage: `go test ./... -cover -coverprofile=coverage.out && go tool cover -func=coverage.out`
- Python parity tests (optional): ensure `openai_harmony` is installed, then run pytest in `tests/` to validate CLI parity.

What we check in parity tests:
- Stop token sets and decode parity.
- Conversation rendering (system/developer/tools/channels) vs Python.
- Training substitution (<|end|> → <|return|> for assistant:final).
- Parser and stream parser produce the same message shapes as Python.

## Configuration
- `TIKTOKEN_ENCODINGS_BASE` — directory containing `o200k_base.tiktoken` (overrides remote).
- `TIKTOKEN_GO_CACHE_DIR` — cache directory for the vocab (default `$TMPDIR/tiktoken-go-cache`).
- `TIKTOKEN_OFFLINE` — set `1` to avoid any network download; fails fast if the file is missing.
- `TIKTOKEN_HTTP_TIMEOUT` — HTTP timeout in seconds for vocab download (default 30).

## Development
- Build: `CGO_ENABLED=0 go build ./...`
- Format/Lint: `golangci-lint run ./...`
- Profiles and prebuilt test binaries: see `benchmarks/go/profiles/`.

## Roadmap
- Extended arena/allocator experiments once Go arenas stabilize.
- Additional CLI ergonomics and JSON schemas for inputs/outputs.
- More end‑to‑end examples integrating with popular Go LLM clients.

## License & Acknowledgements
MIT — see [LICENSE](LICENSE).

Upstream projects:
- Original Python package: [openai‑harmony on PyPI](https://pypi.org/project/openai-harmony/)
- Reference implementation: [openai/harmony](https://github.com/openai/harmony)
