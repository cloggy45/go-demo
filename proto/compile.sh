#!/bin/bash

# Exit on error
set -e

# Directory containing .proto files
PROTO_DIR=$(dirname "$0")
# Directory for generated Go code
GEN_DIR="$PROTO_DIR/gen"

# Create output directory if it doesn't exist
mkdir -p $GEN_DIR

# Generate Go code
protoc --proto_path=$PROTO_DIR \
  --go_out=$GEN_DIR --go_opt=paths=source_relative \
  --go-grpc_out=$GEN_DIR --go-grpc_opt=paths=source_relative \
  $PROTO_DIR/random.proto

echo "Proto compilation completed. Generated files in $GEN_DIR" 