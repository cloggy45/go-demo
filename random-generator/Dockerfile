FROM golang:1.22-alpine AS builder

WORKDIR /app

# Create the directory structure
RUN mkdir -p /app/proto /app/random-generator

# Copy the proto module
COPY proto/ /app/proto/

# Copy the service code
COPY random-generator/ /app/random-generator/

# Set the working directory to the service directory
WORKDIR /app/random-generator

# Run go mod tidy to update dependencies
RUN go mod edit -replace github.com/user/proto=../proto && go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o random-generator-server .

# Use a smaller image for the final container
FROM alpine:latest

WORKDIR /app

# Install CA certificates for HTTPS and netcat for healthcheck
RUN apk --no-cache add ca-certificates netcat-openbsd

# Copy the binary from the builder stage
COPY --from=builder /app/random-generator/random-generator-server .

# Copy the .env file
COPY random-generator/.env* ./

# Expose the gRPC port
EXPOSE 50051

# Set the entry point
CMD ["./random-generator-server"] 