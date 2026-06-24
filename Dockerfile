# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The provider-github Authors

# ── build stage ────────────────────────────────────────────────────────────────
# Use BUILDPLATFORM so cross-compilation happens on the native runner (fast).
# TARGETOS/TARGETARCH are injected by buildx for each platform slice.
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

ARG VERSION=dev
ARG BUILD_DATE
ARG VCS_REF

LABEL org.opencontainers.image.title="semrel-plugin-provider-github" \
      org.opencontainers.image.description="semrel plugin: provider-github" \
      org.opencontainers.image.url="https://semrel.io" \
      org.opencontainers.image.source="https://github.com/SemRels/provider-github" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.vendor="SemRels"

COPY --from=build /out/plugin /usr/local/bin/plugin
USER nonroot
ENTRYPOINT ["/usr/local/bin/plugin"]
