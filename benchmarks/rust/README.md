Rust Harmony Micro-Bench

This crate mirrors the Go benchmarks to measure the upstream `openai-harmony` (Rust) implementation on the same inputs.

Run

- Build: `cargo build --release --manifest-path benchmarks/rust/Cargo.toml`
- Bench (JSON output):
  - Render + Parse: `cargo run --release --manifest-path benchmarks/rust/Cargo.toml -- --iters 200 --which both | tee benchmarks/python/results/rust.json`
  - Render only: `cargo run --release --manifest-path benchmarks/rust/Cargo.toml -- --iters 200 --which render`
  - Parse only: `cargo run --release --manifest-path benchmarks/rust/Cargo.toml -- --iters 200 --which parse`

Notes

- Uses the upstream crate via git: `openai-harmony = { git = "https://github.com/openai/harmony" }`.
- We report ops/sec; memory metrics are not collected here (the Go benches include alloc/bytes).
- Large render is the "keep analysis" variant because the auto-drop toggle isn’t exposed publicly in the Rust API.
