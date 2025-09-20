# Harmony Go — Arenas (Experimental)

Harmony Go can store the tokenizer’s decoder table in a Go arena when compiled
with the experimental arenas feature. This primarily reduces long‑lived heap
objects and GC pressure in services that keep the decoder alive for the
process lifetime.

Status: experimental. The default build uses the heap implementation.

## Build flavors

- Default (heap, recommended):

  go test ./...

- Real arenas (experimental):

  GOEXPERIMENT=arenas go test ./...

This switches the implementation selected via build tags:

- `tokenizer/decoder_store_heap.go` — `//go:build !goexperiment.arenas`
- `tokenizer/decoder_store_arena.go` — `//go:build goexperiment.arenas`

## Behavior and performance

- Per‑op speed and bytes/op are similar to the heap path since decoding still
  appends bytes into the destination slice (copy). This is required to avoid
  leaking arena‑backed slices to the GC‑managed heap.
- Arenas help by keeping the decoder storage out of the GC heap, reducing GC
  work and resident memory in long‑running services.

## Safety constraints

- Do not store arena‑backed slices in heap data structures. The arena file
  only copies from the arena blob into the caller’s destination.
- The arena is owned by the decoder and freed on shutdown; never retain
  references into the arena.

## A/B quickly

Build test binaries once and run a subset of benches offline:

```bash
GOCACHE=.gocache GOTMPDIR=.gotmp GOMODCACHE=.gomodcache GOPATH=.gopath \
  go test -c -o profiles/heap.test ./benchmarks/go
GOEXPERIMENT=arenas GOCACHE=.gocache GOTMPDIR=.gotmp GOMODCACHE=.gomodcache GOPATH=.gopath \
  go test -c -o profiles/arena.test ./benchmarks/go

TIKTOKEN_OFFLINE=1 ./profiles/heap.test -test.run=^$ -test.bench='BenchmarkRenderToolCall' -test.benchmem -test.benchtime=200ms
GOEXPERIMENT=arenas TIKTOKEN_OFFLINE=1 ./profiles/arena.test -test.run=^$ -test.bench='BenchmarkRenderToolCall' -test.benchmem -test.benchtime=200ms
```

## When to adopt by default

Switching the library’s default to arenas makes sense once the Go team
stabilizes and releases arenas, and tooling/ecosystem support improves. Until
then, the heap path remains the default and recommended configuration.

