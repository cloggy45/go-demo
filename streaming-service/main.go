package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	pb "github.com/user/proto/gen"
	"streaming-service/client"
	"streaming-service/config"
	"streaming-service/server"
)

func main() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using default or environment values")
	}

	// Load configuration
	cfg := config.LoadConfig()
	
	log.Printf("Starting streaming service")
	log.Printf("Connecting to gRPC server at %s", cfg.GeneratorAddr())

	// Create a gRPC client
	streamClient := client.NewStreamClient(
		cfg.GeneratorAddr(),
		client.WithReconnectWait(cfg.ReconnectWait),
		client.WithMaxRetries(cfg.MaxRetries),
	)

	// Connect to the server
	if err := streamClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer streamClient.Close()

	// Create a context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signals
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Check connection by calling a simple RPC
	startTime := time.Now()
	num, err := streamClient.GetRandomNumber(ctx, 1, 100)
	if err != nil {
		log.Fatalf("Failed to call GetRandomNumber: %v", err)
	}
	log.Printf("GetRandomNumber response: %d (took %v)", num, time.Since(startTime))

	// Create SSE server
	sseServer := server.NewSSEServer(
		cfg.ServerHost,
		cfg.ServerPort,
		server.WithKeepAliveInterval(cfg.SSEKeepAliveInterval),
	)

	// Create data stream bridge
	bridge := server.NewDataStreamBridge(
		streamClient,
		sseServer,
		server.WithBufferSize(cfg.BufferSize),
		server.WithDataType(pb.DataType_NUMERIC),
		server.WithFrequency(cfg.DefaultFrequency),
		server.WithValueRange(cfg.DefaultMinValue, cfg.DefaultMaxValue),
		server.WithHistorySize(5), // Start with 5 historical points
	)

	// Start the bridge
	if err := bridge.Start(); err != nil {
		log.Fatalf("Failed to start data stream bridge: %v", err)
	}
	
	// Start the SSE server in a goroutine
	go func() {
		log.Printf("Starting SSE server at http://%s:%s", cfg.ServerHost, cfg.ServerPort)
		if err := sseServer.Start(); err != nil {
			log.Fatalf("Failed to start SSE server: %v", err)
		}
	}()

	// Wait for shutdown signal
	sig := <-signals
	log.Printf("Received signal %v, shutting down...", sig)
	
	// Stop all components
	log.Printf("Stopping data stream bridge...")
	bridge.Stop()
	
	log.Printf("Stopping SSE server...")
	sseServer.Stop()
	
	log.Printf("Canceling context...")
	cancel()
	
	log.Printf("Streaming service shut down")
} 