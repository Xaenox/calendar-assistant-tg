FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o calendar-assistant ./cmd/bot

# Create a minimal image
FROM alpine:latest

WORKDIR /app

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create tmp directory for temporary files
RUN mkdir -p /app/tmp

# Copy the binary from the builder stage
COPY --from=builder /app/calendar-assistant .

# Set executable permissions
RUN chmod +x /app/calendar-assistant

# Create volume for persistent data
VOLUME ["/app/tmp"]

# Run the application
CMD ["./calendar-assistant"] 