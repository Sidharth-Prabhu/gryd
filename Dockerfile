# Multi-platform build stage
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

WORKDIR /src

# Allow Go to automatically fetch required toolchain version
ENV GOTOOLCHAIN=auto

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Target platform arguments passed by Docker buildx
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X github.com/loom-cli/gryd/internal/cli.Version=${VERSION}" -o /out/gryd ./cmd/gryd

# Final runtime image
FROM alpine:3.21

# Install ca-certificates, bash, git, curl, and procps (standard dev/orchestration tools)
RUN apk add --no-cache ca-certificates tzdata bash git curl procps

WORKDIR /workspace

# Copy the compiled binary
COPY --from=builder /out/gryd /usr/local/bin/gryd

# Make binary executable and set working directory
RUN chmod +x /usr/local/bin/gryd

ENTRYPOINT ["gryd"]
CMD ["--help"]
