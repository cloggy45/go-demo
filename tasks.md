# Objective

Create a real-time data streaming pipeline that connects a gRPC server to web clients using Server-Sent Events (SSE). The system will stream time-series data from the gRPC service, transform it into a format optimized for D3.js visualizations and table displays, and deliver it to web clients through a REST API using SSE. This architecture enables real-time updates for interactive dashboards with minimal latency, while ensuring reliability, scalability, and security throughout the data pipeline. We will leverage the existing project structure, reuse components where possible, create a new dedicated streaming service, and orchestrate all services with Docker.

# gRPC to REST API Streaming Implementation (SSE) for D3.js Visualization

## 1. Extend Existing Protocol Buffer Definitions for Streaming

- [x] Update existing protocol buffer (.proto) file with server streaming RPC method
  - [x] Reuse existing message types where possible (e.g., NumberResponse, StringResponse)
  - [x] Define a new service or extend RandomService with streaming capabilities
  - [x] Add a server-streaming RPC method (returns "stream" of responses)
  - [x] Include timestamp and sequence fields in response messages for ordering
  - [x] Add appropriate field options and comments for documentation
  - [x] Design data structure optimized for time-series visualization in D3.js

- [x] Update the gRPC server implementation with streaming capability
  - [x] Reuse existing Go server setup from random-generator service
  - [x] Implement the new streaming service interface defined in the extended .proto file
  - [x] Create a mechanism to generate continuous data for streaming
  - [x] Implement flow control to handle backpressure
  - [x] Add configurable data update frequency options (e.g., 1s, 5s, real-time)

- [x] Ensure the server can continuously stream data to clients
  - [x] Implement a data producer that can continuously generate events
  - [x] Set up buffering for messages when clients are slow
  - [x] Configure appropriate timeouts and keepalive settings
  - [x] Implement graceful shutdown to handle in-flight streams
  - [x] Create data aggregation capability for different time resolutions

- [x] Add basic error handling and logging
  - [x] Implement error propagation to clients
  - [x] Add debug-level logging for stream lifecycle events (connect, disconnect, errors)
  - [x] Handle client disconnections cleanly
  - [x] Implement health check endpoint that confirms streaming service status

- [x] **TEST: Verify gRPC Streaming Implementation**
  - [x] Use a gRPC client tool (like grpcurl or BloomRPC) to call the streaming endpoint
  - [x] Confirm continuous data is being streamed back
  - [x] Check server logs for proper debug-level stream lifecycle events
  - [x] Verify the health check endpoint returns the proper status

## 2. Create a gRPC Client in the New Streaming Service

- [x] Generate Go client code from the updated protocol buffer definitions
  - [x] Use the existing protoc setup and compilation script
  - [x] Generate client stubs from extended .proto definitions
  - [x] Integrate with the existing project structure

- [x] Implement client logic to connect to the gRPC server
  - [x] Leverage connection patterns from the existing rest-gateway service
  - [x] Create client configuration (timeouts, retries)
  - [x] Establish connection with proper connection pooling
  - [x] Implement graceful connection shutdown
  - [x] Add data request parameters for visualization (time range, resolution)

- [x] Add logic to process the streaming data received from the server
  - [x] Create message handlers for each type of streaming response
  - [x] Implement a simple event-based system to process incoming messages
  - [x] Set up in-memory queue/buffer for received messages
  - [x] Add sequence tracking to handle out-of-order messages
  - [s] Log received messages at debug level to confirm data flow

- [x] **TEST: Verify gRPC Client Connection and Data Reception**
  - [x] Start the gRPC server and the client
  - [x] Check client logs to confirm successful connection to the server
  - [x] Verify streaming data is being received and processed
  - [x] Manually interrupt the server to test reconnection logic
  - [x] Examine logs to confirm proper reconnection behavior

## 3. Create New Streaming Service with SSE Endpoint

- [x] Set up a dedicated streaming service
  - [x] Create a new Go service similar to the existing rest-gateway
  - [x] Use Gin framework like the existing REST API implementation
  - [x] Configure server for handling long-lived connections
  - [x] Set up routing for the SSE endpoints
  - [x] Configure CORS for browser clients if needed
  - [x] Add query parameters for data customization (timeframe, granularity)

- [x] Implement a health check endpoint
  - [x] Create a `/health` endpoint returning service status
  - [x] Include connection status to gRPC server in health check response
  - [x] Add metrics on active SSE connections
  - [x] Log health check requests at debug level

- [x] Create an SSE endpoint with appropriate headers
  - [x] Set required headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, etc.
  - [x] Implement keep-alive mechanism (sending periodic comments)
  - [x] Set up proper response chunking
  - [x] Create handler for client connection requests
  - [x] Log each new SSE connection at debug level with client information

- [x] Implement connection handling to maintain open client connections
  - [x] Create a connection manager to track all SSE clients
  - [x] Implement proper connection closure detection
  - [x] Handle client disconnections gracefully
  - [x] Log connection and disconnection events at debug level
  - [x] Implement timeout handling for idle connections

- [x] Add simple client tracking mechanism
  - [x] Create a registry of connected clients with unique identifiers
  - [x] Implement message broadcasting to all connected clients
  - [x] Log client count metrics at regular intervals at debug level


## 4. Connect gRPC Client and SSE Server in the Streaming Service

- [x] Create a bridge component within the new streaming service
  - [x] Design a simple message bus to connect components
  - [x] Implement the bridge to consume gRPC and produce SSE events
  - [x] Log message flow through the bridge at debug level
  - [x] Add configuration for reconnection and buffer size

- [x] Implement logic to forward data from gRPC stream to SSE connections
  - [x] Create a message transformer to convert gRPC messages to SSE events
  - [x] Implement event formatting with proper `event:`, `data:`, and `id:` fields
  - [x] Log each forwarded message at debug level
  - [x] Add batching logic for high-frequency updates if needed

- [x] Add data transformation for D3.js and table visualization
  - [x] Create JSON serialization for visualization-friendly formats
  - [x] Structure data in D3.js-friendly format (arrays, objects with consistent keys)
  - [x] Add basic metadata for axes and data ranges
  - [x] Log transformed data structure at debug level periodically

- [x] Implement basic error handling
  - [x] Handle gRPC connection errors with appropriate retry
  - [x] Log errors at appropriate levels
  - [x] Create error events for SSE clients when needed
  - [x] Ensure clean shutdown of all components

- [x] **TEST: Verify End-to-End Data Flow**
  - [x] Set up all components (gRPC server, bridge, SSE endpoint)
  - [x] Open an HTML page using EventSource to confirm messages are received and displayed in the console
  - [x] Check logs at each step to verify proper message transformation

## 5. Basic Security and Docker Setup

- [ ] Implement minimal security measures
  - [ ] Add simple API key authentication if needed
  - [ ] Implement basic rate limiting per client

- [x] Create Dockerfile for the new streaming service
  - [x] Base on the existing Dockerfile from rest-gateway
  - [x] Configure appropriate build stages for compilation
  - [x] Set up the runtime environment
  - [x] Use environment variables for ALL configuration settings

- [x] Configure environment variables for the streaming service
  - [x] Define variables for service connection details (host, port)
  - [x] Configure stream parameters (buffer size, update frequency)
  - [x] Set logging levels and formats via environment variables
  - [x] Define fallback default values for all environment variables
  - [x] Document all environment variables in the README

- [x] Update docker-compose.yml to include the new streaming service
  - [x] Add the new service definition
  - [x] Configure environment variables for service discovery
  - [x] Set up proper dependencies
  - [x] Define appropriate port mappings
  - [x] Connect to the existing microservice network
  - [x] Use environment variable substitution in docker-compose where appropriate

- [ ] **TEST: Verify Docker Deployment**
  - [ ] Build and run the entire stack with docker-compose
  - [ ] Verify all services start successfully
  - [ ] Test modifying behavior by changing environment variables
  - [ ] Connect to the SSE endpoint from outside the Docker network
  - [ ] Confirm data flows through the entire system
  - [ ] Check logs to ensure proper communication between containers