# AI-assisted contributions

AI-assisted code, documentation, review, and debugging are welcome in Silo.
The contributor submitting the work owns the result and is responsible for its
accuracy, safety, scope, and maintainability.

This policy applies to pull requests and issues prepared by a person, an agent,
or a combination of both.

## Required disclosure

> [!IMPORTANT]
> Every pull request and issue must disclose whether AI was involved. Report the
> exact tool and model identifiers shown by the tool, describe the level of
> involvement, and summarize the independent or adversarial review. If no AI
> was used, say so explicitly.

Include this completed block in the issue or pull request body:

```md
### AI Disclosure

- Tool(s): exact tool name(s), or "none"
- Model(s): exact model identifier(s) reported by each tool, or "n/a"
- Involvement: Fully AI-generated, human verified | AI-assisted | Human-written, AI-reviewed | No AI used
- Adversarial review: findings from an independent/adversarial review and how they were resolved, or "n/a" only when no AI or implementation change was involved
```

The disclosure is about provenance and reviewability. AI use is not a reason to
reject a contribution, and "no AI" is a complete answer when accurate.
Exact model identifiers help maintainers understand the tool capabilities used,
reproduce the workflow where necessary, and decide whether line-by-line review
or a separate implementation is the most efficient path.

## Contributor responsibility

Before submitting AI-assisted work:

1. Read every line of the final diff.
2. Understand the behavior well enough to explain the reasoning and tradeoffs.
3. Confirm that APIs, configuration, schemas, commands, and repository paths
   actually exist.
4. Add and run focused tests for the behavior changed.
5. Run the repository checks relevant to the complete diff.
6. Exercise user-facing behavior manually when practical.
7. Look beyond the edited files for silent behavior changes, dead code,
   security regressions, and interactions across system layers.
8. Run an independent or adversarial review and resolve its findings.

For non-trivial changes, state the review scope and method. A bare "no findings"
statement is not a sufficient review summary.

"The AI suggested it" is not an explanation. The submitting contributor owns
the implementation regardless of how it was produced.

## Evidence standard

Use real commands and real observations. Focused tests during development may
look like:

```sh
go test ./internal/<package>/...
cd web && pnpm exec vitest run path/to/changed.test.tsx
```

Use `make test-go`, `make test-web`, and the relevant verification targets for
the full pre-submission gate described in [CONTRIBUTING.md](../CONTRIBUTING.md).
Paste actual results into the pull request and identify any command that was not
run or did not pass.

Tests may have blind spots, including tests generated with AI. Passing tests do
not replace code review, manual verification, or analysis of system-level
effects.

For bug reports:

- reproduce the problem on a real deployment before filing
- provide exact steps and actual observed behavior
- paste raw log output; redact credentials, tokens, personal data, and private
  media details, mark the redactions, and do not replace the remainder with an
  AI paraphrase
- separate observations from suspected root cause
- place AI-generated analysis under technical notes after the reproduction

## Integrity and enforcement

> [!WARNING]
> Fabricated evidence results in an immediate block, including on a first
> offense. This includes invented APIs, unexecuted reproduction steps,
> synthesized logs, imagined vulnerabilities, unobserved bugs, and false test
> results.

Undisclosed AI use discovered after submission results in the contribution
being closed. Repeated non-disclosure results in the contributor being blocked.
The policy violation is the missing disclosure, not the use of AI.

## Review outcomes

Maintainers review the idea, implementation, evidence, and fit with the current
codebase. They may request changes, narrow the scope, decline the contribution,
or accept the idea and implement it differently. A separate implementation is a
normal project decision and does not invalidate a well-researched proposal.
