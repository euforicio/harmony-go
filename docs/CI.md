# Continuous Integration (CI)

This repository uses GitHub Actions to lint, test, and (optionally) benchmark.

Speed-focused defaults for Pull Requests (PRs):

- Concurrency cancellation avoids running outdated workflows when you push new commits to the same PR.
- Docs-only changes are ignored with `paths-ignore` to skip CI entirely for those PRs.
- Lint and tests run in parallel to reduce overall wall time.
- Tests run in a fast mode (`-short`) and only for changed Go packages when possible.
- Tokenizer data (`.tiktoken-cache`) and Go build cache (`.gocache`) are persisted across runs.

When full coverage is needed on a PR:

- Apply the `full-ci` label to the PR to run the full test suite with `-race` and coverage. Coverage will be uploaded as an artifact for that run.
- Labels are auto‑provisioned on PR events by the "Auto Label" workflow, which syncs `.github/labels.yml` before labeling. No CLI or manual step is required.

Auto-labeling and default rules:

- PRs are auto-labeled based on changed paths using `.github/labeler.yml`.
- By default, changes to `go.mod`, `go.sum`, or anything under `tokenizer/**` add the `full-ci` label automatically.
- Label flips post a small bot comment so reviewers know whether a PR will run fast or full CI.

Pushes to `main`:

- Run full tests with `-race` and coverage and upload the coverage profile artifact.
- Run the Arenas build and a minimal arenas test.

Benchmarks:

- Benchmarks are manual via the workflow_dispatch event. They build test binaries and run a subset of representative benchmarks; results are uploaded as artifacts.

Notes:

- If `go.mod` or `go.sum` changes in a PR, CI runs tests across all packages (dependency graph can shift).
- Some tests exercise network timeout paths. If those become a bottleneck, we can guard them with `testing.Short()` and rely on the `full-ci` label for complete coverage.
