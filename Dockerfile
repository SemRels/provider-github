# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The plugin-template Authors

FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/plugin ./cmd/plugin

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/plugin /usr/local/bin/plugin
ENTRYPOINT ["/usr/local/bin/plugin"]
