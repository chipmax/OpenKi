# Headless image for openki: built with the "headless" tag so the tray and
# WebView2 packages are compiled out entirely. That is the difference between
# "runs on a server" and "was built to run on a server": no GTK, no appindicator,
# no CGO, so this build also works as a plain static binary on a NAS.
#
# The dashboard is loopback-only by design, so publish the API port and reach
# the panel through an SSH tunnel or a reverse proxy on the same host.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags headless -trimpath \
    -ldflags "-X github.com/chipmax/openki/internal/gateway.Version=docker" \
    -o /out/openki ./cmd/openki

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H -u 1000 openki
COPY --from=builder /out/openki /usr/local/bin/openki
# Headless is the only mode this binary has; setting it explicitly keeps the
# compose file below meaningful even for a GUI-capable build.
ENV OPENKI_HEADLESS=1 \
    OPENKI_HOME=/data
RUN mkdir -p /data && chown 1000:1000 /data
VOLUME /data
USER 1000
EXPOSE 8787
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s \
  CMD wget -qO- http://127.0.0.1:8787/healthz || exit 1
ENTRYPOINT ["openki"]
