# gRPC + REST Microservice Demo in Go

This project demonstrates a minimal microservice architecture using Go, gRPC, and Docker:

- A gRPC-based microservice (`random-generator`) that generates random data
- A REST API gateway (`rest-gateway`) that consumes the gRPC service internally
- Docker and Docker Compose for containerization and orchestration

## Architecture

The architecture consists of:

1. **Random Generator Service (gRPC)**
   - Internal service that generates random numbers and strings
   - Exposes a gRPC API for internal communication
   - Runs on port 50051 internally

2. **REST Gateway Service (HTTP)**
   - External-facing service that provides a REST API
   - Communicates with the random-generator service via gRPC
   - Translates REST requests to gRPC calls
   - Runs on port 8080 internally (exposed as 8081)

## Prerequisites

- Docker and Docker Compose
- Go 1.22 or later (for local development)
- `protoc` compiler (only needed for regenerating protocol buffers)

## Project Structure

```
├── proto/                 # Protocol Buffers definitions
│   ├── random.proto       # Service and message definitions
│   └── gen/               # Generated Go code
├── random-generator/      # gRPC Service
│   ├── main.go            # Service implementation
│   └── Dockerfile         # Container definition
├── rest-gateway/          # REST API Service
│   ├── main.go            # REST API implementation
│   └── Dockerfile         # Container definition
└── docker-compose.yml     # Service orchestration
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

## License

This project is licensed under the MIT License - see the LICENSE file for details. 