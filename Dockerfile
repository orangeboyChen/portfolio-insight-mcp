################################################################################
# Stage 1 — build a fully static binary (supports linux/amd64 and linux/arm64)
################################################################################
ARG GO_VERSION=1.26
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary — TARGETOS/TARGETARCH are auto-injected by BuildKit
# when building with `docker buildx --platform=linux/amd64,linux/arm64`
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/server .

################################################################################
# Stage 2 — minimal runtime image (multi-arch: amd64, arm64)
################################################################################
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/server"]
