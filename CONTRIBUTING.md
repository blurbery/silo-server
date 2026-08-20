# Contributing to Silo

Thank you for contributing to Silo. Contributions from people using any
development workflow—including AI-assisted workflows—are welcome. Every
contributor remains responsible for understanding, testing, and explaining the
work they submit.

Most of Silo's codebase was developed with AI assistance. The same ownership,
evidence, and disclosure standards apply to maintainers and external
contributors.

## Before you start

> [!IMPORTANT]
> Coordinate non-trivial work before implementation. Open an issue or start a
> project discussion for features, API or behavior changes, schema migrations,
> large refactors, and other changes that affect product scope. Documentation,
> typo fixes, and narrow bug fixes may go directly to a pull request.

Silo is pre-1.0 and evolving. Early coordination helps avoid duplicate work,
conflicts with changes already in progress, and proposals outside the current
scope. Review [Project non-goals](docs/non-goals.md) and the relevant
architecture documentation before proposing a new capability.

Durable architecture and contracts belong under `docs/architecture/`. Temporary
implementation plans and working notes belong in the issue or pull request, not
as permanent repository documents.

## Reporting a problem

Use the [GitHub issue forms](https://github.com/Silo-Server/silo-server/issues/new/choose)
for bugs, installation problems, and performance issues. Start with what you
observed, not a root-cause theory.

Include:

- what you were trying to do
- the exact steps you performed
- expected and actual behavior
- the specific action that is slow or broken, such as save, scan, browse,
  import, or playback
- whether the problem is consistent or intermittent
- the relevant library, media type, filter, setting, or value
- Silo version, build, branch, or commit
- deployment details
- screenshots, recordings, or raw log excerpts when relevant

Put suspected files, SQL output, stack traces, and root-cause theories under
**Technical notes**, after the workflow and reproduction are clear.
Redact credentials, tokens, personal data, and private media details from raw
evidence, and mark each redaction. Do not paraphrase or synthesize the remainder.

## Prepare a focused change

1. Read the existing implementation and tests in the area you will change.
2. Keep one concern per pull request; do not mix unrelated cleanup or refactors.
3. Follow established patterns and add comments only where behavior is not
   obvious from the code.
4. Add focused tests that fail before the fix and pass after it.
5. Exercise user-facing behavior in a running application when the change can
   be tested manually.
6. Review the complete diff for unintended behavior, generated-file drift,
   local paths, credentials, and unrelated edits.
7. Obtain an independent or adversarial review of non-trivial changes and
   resolve the findings before submission.

Tests are evidence, not proof. Consider system-level effects beyond the files
you touched, and be prepared to explain the implementation, alternatives, and
tradeoffs during review.

## Development setup

Follow [DEVELOPMENT.md](DEVELOPMENT.md) for prerequisites, local services,
builds, migrations, tests, and repository structure.

If a change spans Silo and `silo-plugin-sdk`, use an untracked local `go.work`
workspace for iteration. `go.work` and `go.work.sum` are intentionally ignored.
CI runs from a clean checkout, and release builds explicitly set `GOWORK=off`.
Any SDK package or symbol used by repository code must therefore exist in a
pushed, tagged `github.com/Silo-Server/silo-plugin-sdk` release before merge.

## Validate your change

Run focused tests while iterating:

```sh
go test ./internal/<package>/...
cd web && pnpm exec vitest run path/to/changed.test.tsx
```

Before opening a pull request, run every relevant repository gate. A typical
local validation is:

```sh
# Go build, formatting, vet, and tests
make embed-stub
go build ./...
gofmt -l .
go vet ./...
make test-go

# Web install, lint, formatting, build, and tests
cd web
pnpm install --frozen-lockfile
pnpm run lint
pnpm run format:check
pnpm run build
cd ..
make test-web

# Generated contracts, fixtures, and docs hygiene
make verify-settings-bindings-all
make verify-playback-fixtures
make verify-local-paths
```

`gofmt -l .` must produce no output. Run additional focused tests for every
manually resolved or high-risk area.

> [!NOTE]
> `make lint` runs `golangci-lint` across the full Go tree and can report
> inherited findings. Pull-request CI gates changed Go lines with
> `golangci-lint run --new-from-merge-base="origin/<base>" ./...`; new or changed
> lines must be clean. The current [CI workflow](.github/workflows/ci.yml) is the
> authoritative list of required checks.

Paste actual command results into the pull request. Do not report a check as
passing if it was skipped, failed, or was not run in the stated environment.

## AI-assisted contributions

> [!WARNING]
> AI use must be disclosed in every issue and pull request. Fabricated APIs,
> observations, vulnerabilities, reproduction steps, logs, or test results are
> not acceptable. Bug reports must come from a real reproduction, and logs must
> be raw rather than AI-paraphrased.

Read and follow the canonical
[AI-assisted contribution policy](docs/ai-contributions.md). It defines the
required disclosure block, contributor responsibilities, evidence standard,
and enforcement policy. "No AI" is a valid disclosure; non-disclosure is not.

## Open the pull request

Use a concise [Conventional Commit](https://www.conventionalcommits.org/)
title. A reviewable pull request should include:

- a linked issue or scope item for non-trivial work; use `N/A — narrow fix` when
  prior coordination was not required
- the user or maintainer problem being solved
- why the chosen approach is appropriate
- actual validation commands and results
- migration, compatibility, security, or operational risks
- screenshots or recordings for visible UI changes
- the completed AI disclosure
- a summary of independent or adversarial review findings and resolutions

Keep the commit history intentional and the final diff limited to the stated
problem.

## Review expectations

Maintainers may ask for a smaller change, request a different implementation,
decline work that no longer fits the project, or accept the idea and implement
it separately. Opening a pull request does not guarantee merge. Clear scope,
reproducible evidence, and a focused diff make review faster.

If scope is uncertain, ask before investing in an implementation.

## Instructions for coding agents

Coding agents must read [AGENTS.md](AGENTS.md) before changing the repository.
`CLAUDE.md` points to the same project instructions. This guide and the
[AI-assisted contribution policy](docs/ai-contributions.md) apply equally to
agent-authored and human-authored work.
