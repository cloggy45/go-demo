FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git for go get commands
RUN apk add --no-cache git

# Create the directory structure
RUN mkdir -p /app/proto /app/streaming-service

# Copy the proto module
COPY proto/ /app/proto/

# Copy the service code
COPY streaming-service/ /app/streaming-service/

# Set the working directory to the service directory
WORKDIR /app/streaming-service

# Run go mod tidy to update dependencies
RUN go mod edit -replace github.com/user/proto=../proto && go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o streaming-service-server .

# Use a smaller image for the final container
FROM alpine:latest

WORKDIR /app

# Install CA certificates for HTTPS and wget/nc for healthcheck
RUN apk --no-cache add ca-certificates wget netcat-openbsd

# Copy the binary from the builder stage
COPY --from=builder /app/streaming-service/streaming-service-server .

# Copy the .env file
COPY streaming-service/.env* ./

# Expose the SSE server port
EXPOSE 8085

# Set the entry point
CMD ["./streaming-service-server"]
