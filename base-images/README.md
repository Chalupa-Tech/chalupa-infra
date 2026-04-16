# Base Images

Shared base container images for Chalupa-Tech services, built by CI in this repo and pushed to the
Gitea registry at `gitea.tailbecff0.ts.net/chalupa-tech/`.

## Images

### `chalupa-base-go`

**Registry:** `gitea.tailbecff0.ts.net/chalupa-tech/chalupa-base-go`

A minimal `FROM scratch` base image containing only the CA certificate bundle and a nonroot user
(UID/GID 65532). ~200KB. Intended as the final stage for all Go service images.

**Contents:**
- `/etc/ssl/certs/ca-certificates.crt` — CA bundle from Alpine's `ca-certificates` package
- Runs as UID 65532 (nonroot, no shell)

**Known limitations:**
- No timezone data — services requiring timezone-aware time must install their own (`tzdata` package
  or copy from a builder stage)
- No shell, no package manager — debugging requires ephemeral containers or distroless debug variants

### `chalupa-base-job`

**Registry:** `gitea.tailbecff0.ts.net/chalupa-tech/chalupa-base-job`

Extends `chalupa-base-go` with CLI tools needed by Kubernetes CronJobs and init containers that
interact with OpenBao (secrets) and NATS (messaging).

**Contents (in addition to chalupa-base-go):**
- `/usr/local/bin/bao` — OpenBao CLI v2.2.0 (static binary, Vault-compatible)
- `/usr/local/bin/nats` — natscli v0.2.3 (static binary)
- `/usr/local/bin/jq` — jq v1.7.1 (official static binary from jqlang/jq)

**Known limitations:**
- Same timezone limitation as `chalupa-base-go`
- No shell — `bao`, `nats`, and `jq` must be invoked directly as the container entrypoint or via
  `exec` form; not usable in shell scripts without a shell layer

## Tagging Strategy

Tags are driven by git tags on this repo (`chalupa-infra`):

| Tag | When applied |
|-----|-------------|
| `latest` | Every successful build on `main` |
| `<sha_short>` | Every successful build (7-char git SHA) |
| `v<version>` | When a `v*` git tag is pushed (e.g. `v1.0.0`) |

Semver is recommended for production Dockerfiles. Downstream services should pin to a `v<version>`
tag and let Renovate propose upgrades.

## Cutting a New Release

1. Update `CHANGELOG.md` — add an entry under `## [Unreleased]` describing the change, then rename
   it to `## [x.y.z]` following semver rules.
2. Merge to `main`.
3. Push a semver git tag:
   ```bash
   git tag -a v1.0.1 -m "Release v1.0.1"
   git push origin v1.0.1
   ```
4. The `build-base-images.yml` workflow triggers automatically on the tag push and publishes
   `chalupa-base-go:v1.0.1` and `chalupa-base-job:v1.0.1` to Gitea.
5. Renovate detects the new tag and opens PRs in downstream repos to bump the pinned version.

## How Downstream Services Reference the Image

Pin to a semver tag in your final `FROM` stage:

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o /app .

FROM gitea.tailbecff0.ts.net/chalupa-tech/chalupa-base-go:v1.0.0
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
```

For CronJob workloads that need `bao`/`nats`/`jq`:

```dockerfile
FROM gitea.tailbecff0.ts.net/chalupa-tech/chalupa-base-job:v1.0.0
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
```

Renovate will auto-merge patch and minor bumps to these `FROM` lines. Major bumps require manual
dashboard approval.
