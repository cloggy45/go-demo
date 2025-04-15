# gRPC + REST Microservice Demo in Go

This project demonstrates a minimal microservice architecture using Go, gRPC, and Docker:

- A gRPC-based microservice (`random-generator`) that generates random data
- A REST API gateway (`rest-gateway`) that consumes the gRPC service internally
- A streaming service (`streaming-service`) that provides SSE streaming of data from the gRPC service
- Docker and Docker Compose for containerization and orchestration

## Architecture

The architecture consists of:

1. **Random Generator Service (gRPC)**
   - Internal service that generates random numbers and strings
   - Exposes a gRPC API for internal communication
   - Runs on port 50051 internally
   - Acts as the data source for both the REST gateway and streaming service

2. **REST Gateway Service (HTTP)**
   - External-facing service that provides a REST API
   - Communicates with the random-generator service via gRPC
   - Translates REST requests to gRPC calls
   - Runs on port 8080 internally (exposed as 8081)

3. **Streaming Service (SSE)**
   - External-facing service that provides Server-Sent Events (SSE) streaming
   - Communicates with the random-generator service via gRPC
   - Consumes the continuous data stream from the random-generator service
   - Translates gRPC data streams to SSE events for real-time client consumption
   - Maintains persistent connections with web clients using SSE protocol
   - Runs on port 8085 internally (exposed as 8085)

### Data Flow

The data flow in the system works as follows:

1. The **Random Generator Service** continuously generates random data points
2. The **Streaming Service** connects to the Random Generator Service via gRPC
3. The Streaming Service establishes a stream that receives data points in real time
4. When web clients connect to the Streaming Service's `/stream` endpoint via SSE:
   - Each client establishes a persistent HTTP connection
   - The Streaming Service forwards data from the gRPC stream to connected clients as SSE events
   - Clients (like the test_sse_client.html) receive and visualize this data in real time

This architecture enables efficient real-time data streaming from the backend to web clients with minimal latency.

## Prerequisites

- Docker and Docker Compose
- Go 1.23 or later (for local development)
- `protoc` compiler (only needed for regenerating protocol buffers)

## Project Structure

```
├── proto/                   # Protocol Buffers definitions
│   ├── random.proto         # Service and message definitions
│   └── gen/                 # Generated Go code
├── random-generator/        # gRPC Service
│   ├── main.go              # Service implementation
│   └── Dockerfile           # Container definition
├── rest-gateway/            # REST API Service
│   ├── main.go              # REST API implementation
│   └── Dockerfile           # Container definition
├── streaming-service/       # SSE Streaming Service
│   ├── main.go              # Service implementation
│   ├── server/              # SSE server implementation
│   ├── client/              # gRPC client implementation
│   ├── config/              # Configuration handling
│   └── Dockerfile           # Container definition
├── test_sse_client.html     # Test client for SSE streaming
└── docker-compose.yml       # Service orchestration
```

## Building and Running

### Using Docker Compose (Recommended)

Build and start all services:

```bash
docker-compose up -d
```

Stop all services:

```bash
docker-compose down
```

### Local Development

For the Random Generator service:

```bash
cd random-generator
go run main.go
```

For the REST Gateway service:

```bash
cd rest-gateway
go run main.go
```

For the Streaming Service:

```bash
cd streaming-service
go run main.go
```

## API Usage

### REST API Endpoints

Base URL: `http://localhost:8081`

#### Health Check

```bash
curl http://localhost:8081/health
```

Response:
```json
{
  "status": "ok"
}
```

#### Generate Random Number

```bash
curl "http://localhost:8081/random/number?min=1&max=100"
```

Parameters:
- `min`: Minimum value (default: 1)
- `max`: Maximum value (default: 100)

Response:
```json
{
  "number": 42,
  "min": 1,
  "max": 100
}
```

#### Generate Random String

```bash
curl "http://localhost:8081/random/string?length=10&digits=true&special=false"
```

Parameters:
- `length`: Length of the string (default: 10)
- `digits`: Include digits (default: true)
- `special`: Include special characters (default: false)

Response:
```json
{
  "string": "aB3cDe4fGh",
  "length": 10,
  "include_digits": true,
  "include_special": false
}
```

### Streaming Service (SSE)

Base URL: `http://localhost:8085`

#### Health Check

```bash
curl http://localhost:8085/health
```

Response:
```json
{
  "status": "ok",
  "time": "2025-04-15T13:25:47Z"
}
```

#### Metrics

```bash
curl http://localhost:8085/metrics
```

Response:
```json
{
  "clients": 1,
  "messages_sent": 0,
  "uptime": 375
}
```

#### SSE Stream

To consume the SSE stream programmatically:

```javascript
const eventSource = new EventSource('http://localhost:8085/stream');

eventSource.addEventListener('data', function(e) {
  const data = JSON.parse(e.data);
  console.log('Received data:', data);
});

eventSource.addEventListener('error', function(e) {
  console.error('Error:', e);
});
```

### Test SSE Client

The project includes a test client for the SSE streaming service. To use it:

1. Start the services using Docker Compose
2. Open the `test_sse_client.html` file in a web browser
3. Click the "Connect" button (the default URL is already set to `http://localhost:8085/stream`)

Features of the test client:

- Real-time data visualization with a live chart
- Event log with timestamps
- Statistics showing total events, complete events, and incomplete events
- Display of the latest received value
- Ability to disconnect and reconnect to the stream

The chart will show the random numeric values over time, updating in real-time as data is received from the streaming service.

## License

This project is licensed under the MIT License - see the LICENSE file for details. 