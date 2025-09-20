# Contributing to Harmony Go

Thanks for your interest in improving Harmony Go! This guide explains how to set up your environment, run tests, and submit high‑quality pull requests.

## Prerequisites
- Go 1.25+ (see CI matrix in `.github/workflows/go.yml`).
- `golangci-lint` for local linting (optional but recommended).
- Python (optional) if you want to run cross‑parity benchmarks in `docs/python_go_performance.md`.

## Getting Started
- Fork the repo and clone your fork.
- Ensure builds pass locally:
  - `CGO_ENABLED=0 go build ./...`
  - `go test ./...`
  - `golangci-lint run ./...` (if installed)

## Development Workflow
1. Create a feature branch off `main`.
2. Make focused, small commits with clear messages.
3. Add tests where it adds confidence (parsers, renderers, and JSON codecs benefit most).
4. Run `go test -race ./...` for concurrency‑sensitive changes.
5. Update docs when behavior or APIs change (`README.md`, `docs/*.md`).

## Benchmarks (optional)
To compare performance locally:
- Go only: `go test -run '^$' -bench '^Benchmark' -benchmem ./benchmarks/go`
- See `docs/python_go_performance.md` for methodology and notes.

## Style & Conventions
- Follow standard Go formatting (`go fmt`) and idioms.
- Avoid introducing non‑Go dependencies; the library aims to be CGO‑free.
- Keep APIs minimal and spec‑aligned with the Harmony format.

## Pull Requests
- Describe the problem, the approach, and any trade‑offs.
- Include before/after performance numbers when optimizing.
- Link related issues and add screenshots for CLI UX changes if applicable.

### Requesting Full CI On PRs
- Default PR CI is fast: it tests only changed packages with `-short`.
- To run full CI (race + coverage) on a PR, add the `full-ci` label to the PR.
- Coverage from full CI is uploaded as an artifact on that run.
- The `full-ci` label is auto‑created by CI on PR events using `.github/labels.yml`; no CLI or manual step is needed.
  - The repo also auto‑adds `full-ci` for high‑impact paths: `go.mod`, `go.sum`, and anything under `tokenizer/**`. You’ll see a bot comment when the label is added or removed.

## Security / Vulnerabilities
If you believe you’ve found a security issue, please open a minimal, private report to the maintainer rather than a public issue. Include steps to reproduce and impacted versions.

## License
By contributing, you agree that your contributions are licensed under the repository’s MIT license (see `LICENSE`).
