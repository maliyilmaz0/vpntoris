# Cross-arch Linux binary builder. Run via:
#   docker buildx build --platform linux/amd64|linux/arm64 ...
ARG TARGETPLATFORM=linux/amd64
FROM --platform=$TARGETPLATFORM golang:1.26-bookworm AS builder

ARG GOARCH=amd64
ARG VERSION=0.0.0
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=$GOARCH

WORKDIR /src
COPY vpntoris-tray/go.mod vpntoris-tray/go.sum ./
RUN go mod download
COPY vpntoris-tray/ ./

RUN mkdir -p /out && \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/vpntorisd . && \
    go build -trimpath -ldflags="-s -w" -o /out/vpntoris-native-helper ./cmd/vpntoris-native-helper && \
    go build -trimpath -ldflags="-s -w" -o /out/vpntoris-service ./cmd/vpntoris-service && \
    go build -trimpath -ldflags="-s -w" -o /out/vpntorisctl ./cli && \
    go build -trimpath -ldflags="-s -w" -o /out/vpntoris-tray ./cmd/vpntoris-tray

# Keep a tiny runnable image so `docker create`/`docker cp` work; prefer
# `docker build --output type=local` when extracting on the host.
FROM busybox:1.36
COPY --from=builder /out/ /out/
CMD ["true"]
