ARG BUILD_FROM

# Build stage
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/ .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mqtt2escpos main.go

# Final stage
FROM $BUILD_FROM

# Set working directory to our add-on persistent data directory
WORKDIR /data

# Copy the built binary
COPY --from=builder /build/mqtt2escpos /usr/local/bin/mqtt2escpos

# Copy run script
COPY run.sh /
RUN chmod a+x /run.sh

CMD [ "/run.sh" ] 
# Health check: healthy when connected to MQTT and subscribed
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8099/health > /dev/null || exit 1
