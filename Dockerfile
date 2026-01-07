# Multi-stage Dockerfile for CI/CD builds
# This differs from goreleaser.dockerfile which expects pre-built binaries

# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make nodejs npm

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Create dummy UI dist directory (UI is optional for backend)
RUN mkdir -p ui/dist && echo "Docker build" > ui/dist/index.html

# Generate API code (required for compilation)
RUN go generate ./...

# Build the binary
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-extldflags=-static -s -w -X github.com/tgdrive/teldrive/internal/version.Version=${VERSION} -X github.com/tgdrive/teldrive/internal/version.CommitSHA=${COMMIT}" \
    -o teldrive \
    main.go

# Final stage
FROM scratch

COPY --from=builder /build/teldrive /teldrive

EXPOSE 8080

ENTRYPOINT ["/teldrive", "run"]
