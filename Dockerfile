# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The provider-github Authors

# ── build stage ────────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/plugin ./cmd/plugin

# ── distroless release image ───────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

# NOTE: intentionally no repo-specific `org.opencontainers.image.*` LABEL block
# here. sync-template.yml only substitutes the "provider-github Authors" line
# in this file when propagating it to plugin repos (see `sed` step) — it does
# NOT rewrite LABEL title/description/source values. Adding those here would
# get copied verbatim (and wrongly) into every plugin's Dockerfile. Add labels
# in each plugin's own Dockerfile after copying this template, not here.

COPY --from=build /out/plugin /usr/local/bin/plugin
USER nonroot
ENTRYPOINT ["/usr/local/bin/plugin"]
