FROM alpine:3.21 AS tools
ARG TARGETARCH
RUN apk add --no-cache ca-certificates jq curl unzip

ARG BAO_VERSION=2.2.0
RUN curl -fsSL \
    "https://github.com/openbao/openbao/releases/download/v${BAO_VERSION}/bao_${BAO_VERSION}_linux_${TARGETARCH}.tar.gz" \
    | tar xz -C /usr/local/bin bao

ARG NATS_VERSION=0.2.3
RUN curl -fsSL -o /tmp/nats.zip \
    "https://github.com/nats-io/natscli/releases/download/v${NATS_VERSION}/nats-${NATS_VERSION}-linux-${TARGETARCH}.zip" \
    && unzip -j /tmp/nats.zip "*/nats" -d /usr/local/bin \
    && rm /tmp/nats.zip \
    && chmod +x /usr/local/bin/nats

FROM gitea.tailbecff0.ts.net/chalupa-tech/chalupa-base-go:latest
COPY --from=tools /usr/local/bin/bao /usr/local/bin/bao
COPY --from=tools /usr/local/bin/nats /usr/local/bin/nats
COPY --from=tools /usr/bin/jq /usr/bin/jq
USER 65532:65532
