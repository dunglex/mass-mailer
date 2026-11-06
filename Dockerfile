# Builder stage
FROM golang:1.26 AS builder

WORKDIR /src

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/mass-mailer ./cmd/mass-mailer

# Runtime stage
FROM gcr.io/distroless/static-debian12 AS runtime

# Copy the binary from builder
COPY --from=builder /out/mass-mailer /mass-mailer

# Expose the port
EXPOSE 8080

# Set environment variables
ENV DATA_DIR=/data

ENTRYPOINT ["/mass-mailer"]
