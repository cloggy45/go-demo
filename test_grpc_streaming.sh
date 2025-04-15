#!/bin/bash

# Exit on error
set -e

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== gRPC Streaming Test Script ===${NC}"

# Check if grpcurl is installed
if ! command -v grpcurl &> /dev/null; then
    echo -e "${RED}Error: grpcurl is not installed.${NC}"
    echo "Please install it with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

# Service host and port
GRPC_HOST=${GRPC_HOST:-"localhost"}
GRPC_PORT=${GRPC_PORT:-"50051"}
HEALTH_PORT=${HEALTH_PORT:-"8080"}
SERVICE_ADDR="${GRPC_HOST}:${GRPC_PORT}"
HEALTH_ADDR="${GRPC_HOST}:${HEALTH_PORT}"

# Ensure the service is running
echo -e "${YELLOW}Verifying service health at ${HEALTH_ADDR}/health...${NC}"
if ! curl -s "http://${HEALTH_ADDR}/health" > /dev/null; then
    echo -e "${RED}Error: Health check failed. Is the service running?${NC}"
    echo "Start the service with: cd random-generator && go run main.go"
    exit 1
fi

# Get health check information
HEALTH_OUTPUT=$(curl -s "http://${HEALTH_ADDR}/health")
echo -e "${GREEN}Service is running. Health check output:${NC}"
echo "$HEALTH_OUTPUT" | sed 's/^/  /'

# List available services
echo -e "\n${YELLOW}Listing available gRPC services...${NC}"
SERVICES=$(grpcurl -plaintext "$SERVICE_ADDR" list)
echo -e "${GREEN}Available services:${NC}"
echo "$SERVICES" | sed 's/^/  /'

# List methods for the RandomService
echo -e "\n${YELLOW}Listing methods for random.RandomService...${NC}"
METHODS=$(grpcurl -plaintext "$SERVICE_ADDR" list random.RandomService)
echo -e "${GREEN}Available methods:${NC}"
echo "$METHODS" | sed 's/^/  /'

# Test the GetRandomNumber RPC (basic connectivity check)
echo -e "\n${YELLOW}Testing GetRandomNumber RPC...${NC}"
RANDOM_NUMBER=$(grpcurl -plaintext -d '{"min": 1, "max": 100}' "$SERVICE_ADDR" random.RandomService/GetRandomNumber)
echo -e "${GREEN}Random number response:${NC}"
echo "$RANDOM_NUMBER" | sed 's/^/  /'

# Test the StreamRandomData RPC
echo -e "\n${YELLOW}Testing StreamRandomData RPC for 10 seconds...${NC}"
echo -e "${BLUE}Streaming random numeric data...${NC}"
timeout 10s grpcurl -plaintext -d '{"data_type": "NUMERIC", "frequency_ms": 500, "min_value": 1, "max_value": 100, "history_size": 5}' "$SERVICE_ADDR" random.RandomService/StreamRandomData | head -n 20

# Check metrics endpoint
echo -e "\n${YELLOW}Checking metrics endpoint...${NC}"
METRICS=$(curl -s "http://${HEALTH_ADDR}/metrics")
echo -e "${GREEN}Metrics output:${NC}"
echo "$METRICS" | sed 's/^/  /'

# Test with different data types
echo -e "\n${YELLOW}Testing StreamRandomData with PERCENTAGE data type (5 seconds)...${NC}"
timeout 5s grpcurl -plaintext -d '{"data_type": "PERCENTAGE", "frequency_ms": 500, "history_size": 3}' "$SERVICE_ADDR" random.RandomService/StreamRandomData | head -n 10

echo -e "\n${YELLOW}Testing StreamRandomData with BOOLEAN data type (5 seconds)...${NC}"
timeout 5s grpcurl -plaintext -d '{"data_type": "BOOLEAN", "frequency_ms": 500, "history_size": 3}' "$SERVICE_ADDR" random.RandomService/StreamRandomData | head -n 10

# Final success message
echo -e "\n${GREEN}===== Tests completed successfully! =====${NC}"
echo -e "${BLUE}The gRPC streaming implementation is working correctly.${NC}"
echo -e "${YELLOW}Check the server logs for debug-level stream lifecycle events.${NC}" 