# syntax = docker/dockerfile:1.0-experimental
# Build the manager binary
FROM golang:1.17.0 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.sum ./

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o oscen ./cmd/oscen

# Create a final image
FROM alpine:3.13.5

LABEL org.opencontainers.image.source https://github.com/oscenbot/oscen

WORKDIR /
RUN addgroup --gid 1000 -S oscen && adduser -S oscen -G oscen --uid 1000

COPY --from=builder /workspace/oscen .

USER oscen:oscen
ENTRYPOINT ["/oscen"]