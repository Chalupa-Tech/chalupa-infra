# Contributing

## PR review

Every non-draft PR with substantive changes gets an automated review
from `gemini-cli` via `.github/workflows/gemini-review.yml`.

### When review fires automatically

On `pull_request.opened` and `pull_request.synchronize` events, the
review job runs when **all** of the following are true:

- PR is not a draft.
- PR changes at least 20 lines (`additions + deletions`).
- PR touches at least one file outside the exempt list (see below).
- PR does **not** carry the `skip-review` label.

If any of those fail, the `filter_review` job in
`.github/workflows/gemini-dispatch.yml` short-circuits and no review
is posted. The run is still visible in the Actions UI with a `::notice::`
explaining which gate tripped.

### Exempt paths (no auto-review)

These paths don't benefit from LLM review. If a PR touches **only**
these paths, the review is skipped.

- `CHANGELOG.md` (and any nested `CHANGELOG.md`)
- `**/*.lock`, `**/Chart.lock`, `**/package-lock.json`
- `docs/**`
- `**/*.generated.go`, `**/zz_generated_*.go`
- `.gitignore` (and any nested `.gitignore`)

A PR that mixes an exempt-path change with a reviewable change still
fires the review — the gate is "any reviewable file changed", not
"only reviewable files changed".

### Labels

| Label          | Effect                                                           |
|----------------|------------------------------------------------------------------|
| `skip-review`  | Never fires auto-review, regardless of size or path.             |
| `force-review` | Fires auto-review even on drafts, small PRs, or exempt-only PRs. |

`skip-review` wins over `force-review` if both are applied.

### Manual invocation

Even when auto-review is gated off, anyone with OWNER/MEMBER/COLLABORATOR
association can trigger a review on demand by commenting:

```
@gemini-cli /review
```

Comment-triggered reviews bypass **all** gates above — the operator's
explicit request wins. Use this after fixing a draft PR or to force a
review of a docs-only change when you want LLM eyes on the prose.

### Why these gates exist

Gemini review costs real money (~$0.20–0.35 per run). The gates are
calibrated to skip runs that historically contributed no review
signal (generated files, lock bumps, trivial typos) while leaving
a label override for the rare exception. See
`workstreams/ai-reviews/briefs/review-cost-strategy.md` in
`chalupa-brain` for the cost analysis that drove the thresholds.
