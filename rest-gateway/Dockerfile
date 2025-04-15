FROM golang:1.22-alpine AS builder

WORKDIR /app

# Create the directory structure
RUN mkdir -p /app/proto /app/rest-gateway

# Copy the proto module
COPY proto/ /app/proto/

# Copy the service code
COPY rest-gateway/ /app/rest-gateway/

# Set the working directory to the service directory
WORKDIR /app/rest-gateway

# Run go mod tidy to update dependencies
RUN go mod edit -replace github.com/user/proto=../proto && go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o rest-gateway-server .

# Use a smaller image for the final container
FROM alpine:latest

WORKDIR /app

# Install CA certificates for HTTPS and wget for healthcheck
RUN apk --no-cache add ca-certificates wget

# Copy the binary from the builder stage
COPY --from=builder /app/rest-gateway/rest-gateway-server .

# Copy the .env file
COPY rest-gateway/.env* ./

# Expose the REST API port
EXPOSE 8080

# Set the entry point
CMD ["./rest-gateway-server"] 