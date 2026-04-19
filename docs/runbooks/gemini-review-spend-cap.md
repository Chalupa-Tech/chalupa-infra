# Runbook — Gemini review spend caps

Responding to `GeminiSpendSoftCap80`, `GeminiSpendSoftCap100`, or
`GeminiSpendHardCap` fires from the `gen-ai-review-alerts` VMRule.

## Severity ladder

| Alert | Threshold | Severity | Intended action |
|---|---|---|---|
| `GeminiSpendSoftCap80` | >$16/mo | info | Monitor. No action. |
| `GeminiSpendSoftCap100` | >$20/mo | warning | Comment on active PRs; consider pausing large experimental reviews. |
| `GeminiSpendHardCap` | >$50/mo | critical | Halt all Gemini activity until root cause is understood. |

All fires post to the existing severity-routed Discord channels via
the cluster's alertmanager receivers.

## Triage — on any fire

1. Open the **Gemini Review Spend** Grafana dashboard
   (observability folder) and filter by `github_repository` from the
   alert labels.
2. Identify whether the spend is driven by:
   - A single runaway PR (very large diff / many `@gemini-cli /review`
     re-runs) — kill the run via `gh run cancel` and address the PR
     directly.
   - Elevated baseline — model may have changed (check
     `vars.GEMINI_MODEL`) or routine PR volume has grown.
3. Check `gen_ai_pricing_table_age_days` — if stale (>180d), the
   cost may be understated *and* drifted; refresh the pricing table
   before acting on the fire.

## `GeminiSpendHardCap` — halt procedure

Until the alertmanager → GitHub Actions `repository_dispatch` bridge
lands (phase-44), the hard-cap halt is a **manual one-liner**:

```bash
gh api --method PATCH orgs/Chalupa-Tech/actions/variables/GEMINI_DISPATCH_ENABLED \
  --field name=GEMINI_DISPATCH_ENABLED \
  --field value=false \
  --field visibility=all
```

`gemini-dispatch.yml` gates on `vars.GEMINI_DISPATCH_ENABLED != 'false'`,
so flipping this stops new reviews, triages, and invokes immediately.
In-flight runs continue to completion.

Once the root cause is understood and resolved, re-enable:

```bash
gh api --method PATCH orgs/Chalupa-Tech/actions/variables/GEMINI_DISPATCH_ENABLED \
  --field name=GEMINI_DISPATCH_ENABLED \
  --field value=true \
  --field visibility=all
```

Or delete the variable entirely (undefined → enabled).

## Follow-up after any fire

- Record the root cause in the fire's Discord thread.
- If rates drifted, bump the pricing table
  (`chalupa-brain/docs/gen-ai-pricing-table.yaml`) AND the
  corresponding `gen_ai_pricing_table_as_of_timestamp` rule in
  `k8s/platform/observability/templates/gen-ai-pricing-rules.yaml`.
- If the soft caps are firing frequently on routine PRs, consider:
  - Switching `vars.GEMINI_MODEL` to `gemini-2.5-flash-lite`.
  - Tightening the per-PR `maxSessionTurns` in `gemini-review.yml`
    (currently 25).
  - Auditing whether the workstream-context injection
    (`fetch_ws_context` step) is pulling excessive brief material —
    >80KB triggers a soft-cap warning already.
