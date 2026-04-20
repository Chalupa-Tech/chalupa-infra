#!/usr/bin/env python3
"""Parse a Gemini CLI telemetry.log artifact and push derived metrics to
VictoriaMetrics via /api/v1/import/prometheus.

Input: path to a .gemini/telemetry.log file (pretty-printed JSON objects
concatenated, NOT NDJSON — see
reference_gemini_cli_telemetry_log_schema memory).

Output: HTTP POST(s) to ${VM_WRITE_URL%/api/v1/write}/api/v1/import/prometheus
with BasicAuth gh-actions:${TELEMETRY_TOKEN}.

Metrics emitted (names align with phase-39c's gen-ai-pricing-rules.yaml
recording rules so the Gemini Review Spend dashboard lights up without
further changes):

- gen_ai_client_token_usage_total{gen_ai_system, gen_ai_request_model,
    gen_ai_operation_name, gen_ai_token_type, github_repository,
    github_run_id}
    Counter. One sample per (token_type, model) seen across
    gemini_cli.api_response events in the log. Value = sum of the
    corresponding *_token_count field.

- gen_ai_client_operation_duration_seconds_bucket / _sum / _count{
    gen_ai_system, gen_ai_request_model, gen_ai_operation_name,
    github_repository, github_run_id, le}
    Histogram. Bucketed per phase-39c's schema (le=0.1/0.5/1/2/5/10/
    30/60/180/420/600/+Inf). One bucket series per (model, le).

Environment:
- VM_WRITE_URL        Default https://vm-write.chalupatech.com/api/v1/write.
                      Trailing /api/v1/write is stripped and replaced
                      with /api/v1/import/prometheus.
- TELEMETRY_TOKEN     BasicAuth password (username hardcoded to
                      gh-actions, matching .github/alloy/default.alloy).
- GITHUB_REPOSITORY   e.g. Chalupa-Tech/chalupa-infra. Required.
- GITHUB_RUN_ID       e.g. 24690185334. Required.

Exit codes:
- 0 on success, or if the log exists but contains no api_response
  events (telemetry was written but no API was called — harmless).
- 2 on malformed log, missing env, or HTTP error.

Failure to ingest MUST NOT fail the calling workflow step — the caller
wraps this with continue-on-error. The script still exits non-zero so
operators see red in the step view.
"""

from __future__ import annotations

import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

TOKEN_FIELDS = [
    ("input_token_count", "input"),
    ("output_token_count", "output"),
    ("cached_content_token_count", "cached"),
    ("thoughts_token_count", "thought"),
    ("tool_token_count", "tool"),
]

DURATION_BUCKETS_S = [0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 180.0, 420.0, 600.0]

GEN_AI_SYSTEM = "gemini"
GEN_AI_OPERATION_NAME = "generate_content"


def parse_log(text: str) -> list[dict]:
    dec = json.JSONDecoder()
    out: list[dict] = []
    i, n = 0, len(text)
    while i < n:
        while i < n and text[i].isspace():
            i += 1
        if i >= n:
            break
        obj, i = dec.raw_decode(text, i)
        out.append(obj)
    return out


def api_response_events(records: list[dict]) -> list[dict]:
    return [
        r for r in records
        if r.get("attributes", {}).get("event.name") == "gemini_cli.api_response"
    ]


def escape_label(v: str) -> str:
    return v.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def format_labels(labels: dict[str, str]) -> str:
    return ",".join(f'{k}="{escape_label(str(v))}"' for k, v in labels.items())


def build_samples(
    events: list[dict], github_repository: str, github_run_id: str
) -> list[str]:
    if not events:
        return []

    base_labels = {
        "gen_ai_system": GEN_AI_SYSTEM,
        "gen_ai_operation_name": GEN_AI_OPERATION_NAME,
        "github_repository": github_repository,
        "github_run_id": github_run_id,
    }

    token_totals: dict[tuple[str, str], int] = {}
    duration_by_model: dict[str, list[float]] = {}

    for ev in events:
        a = ev["attributes"]
        model = a.get("model") or "unknown"
        for field, token_type in TOKEN_FIELDS:
            v = a.get(field)
            if isinstance(v, (int, float)) and v > 0:
                key = (model, token_type)
                token_totals[key] = token_totals.get(key, 0) + int(v)
        dur_ms = a.get("duration_ms")
        if isinstance(dur_ms, (int, float)):
            duration_by_model.setdefault(model, []).append(float(dur_ms) / 1000.0)

    now_ms = int(time.time() * 1000)
    lines: list[str] = []

    for (model, token_type), total in sorted(token_totals.items()):
        labels = {
            **base_labels,
            "gen_ai_request_model": model,
            "gen_ai_token_type": token_type,
        }
        lines.append(
            f"gen_ai_client_token_usage_total{{{format_labels(labels)}}} {total} {now_ms}"
        )

    for model, durations in sorted(duration_by_model.items()):
        labels = {**base_labels, "gen_ai_request_model": model}
        count = len(durations)
        total = sum(durations)
        for boundary in DURATION_BUCKETS_S:
            n = sum(1 for d in durations if d <= boundary)
            bucket_labels = {**labels, "le": str(boundary)}
            lines.append(
                f"gen_ai_client_operation_duration_seconds_bucket{{{format_labels(bucket_labels)}}} {n} {now_ms}"
            )
        inf_labels = {**labels, "le": "+Inf"}
        lines.append(
            f"gen_ai_client_operation_duration_seconds_bucket{{{format_labels(inf_labels)}}} {count} {now_ms}"
        )
        lines.append(
            f"gen_ai_client_operation_duration_seconds_sum{{{format_labels(labels)}}} {total} {now_ms}"
        )
        lines.append(
            f"gen_ai_client_operation_duration_seconds_count{{{format_labels(labels)}}} {count} {now_ms}"
        )

    return lines


def post_samples(lines: list[str], import_url: str, auth: tuple[str, str]) -> None:
    body = "\n".join(lines).encode("utf-8")
    user, pw = auth
    cred = base64.b64encode(f"{user}:{pw}".encode()).decode()
    req = urllib.request.Request(
        import_url,
        data=body,
        headers={
            "Content-Type": "text/plain; charset=utf-8",
            "Authorization": f"Basic {cred}",
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        if resp.status >= 300:
            raise RuntimeError(f"VM returned HTTP {resp.status}")


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(f"usage: {argv[0]} <telemetry.log path>", file=sys.stderr)
        return 2

    log_path = Path(argv[1])
    if not log_path.exists():
        print(f"telemetry log not found: {log_path}", file=sys.stderr)
        return 2

    raw = log_path.read_text()
    if not raw.strip():
        print("telemetry log is empty — nothing to ingest", file=sys.stderr)
        return 0

    try:
        records = parse_log(raw)
    except json.JSONDecodeError as e:
        print(f"malformed telemetry log: {e}", file=sys.stderr)
        return 2

    events = api_response_events(records)
    if not events:
        print("no gemini_cli.api_response events — nothing to ingest", file=sys.stderr)
        return 0

    github_repository = os.environ.get("GITHUB_REPOSITORY", "").strip()
    github_run_id = os.environ.get("GITHUB_RUN_ID", "").strip()
    if not github_repository or not github_run_id:
        print("GITHUB_REPOSITORY and GITHUB_RUN_ID must be set", file=sys.stderr)
        return 2

    token = os.environ.get("TELEMETRY_TOKEN", "").strip()
    if not token:
        print("TELEMETRY_TOKEN unset — skipping ingest", file=sys.stderr)
        return 0

    vm_write_url = os.environ.get(
        "VM_WRITE_URL", "https://vm-write.chalupatech.com/api/v1/write"
    ).rstrip("/")
    if vm_write_url.endswith("/api/v1/write"):
        base = vm_write_url[: -len("/api/v1/write")]
    else:
        base = vm_write_url
    import_url = f"{base}/api/v1/import/prometheus"

    samples = build_samples(events, github_repository, github_run_id)
    if not samples:
        print("events contained no positive token counts — nothing to push", file=sys.stderr)
        return 0

    print(
        f"ingesting {len(samples)} samples from {len(events)} api_response events "
        f"→ {import_url}",
        file=sys.stderr,
    )

    try:
        post_samples(samples, import_url, ("gh-actions", token))
    except (urllib.error.URLError, RuntimeError) as e:
        print(f"VM push failed: {e}", file=sys.stderr)
        return 2

    print("ingest complete", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
