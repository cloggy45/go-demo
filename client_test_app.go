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

	// Test data streaming with a counter
	var dataPoints int
	log.Printf("Starting data stream...")
	
	// This is a basic implementation to test connectivity
	// The full SSE implementation will be in the next steps
	err = streamClient.StreamRandomData(
		ctx,
		pb.DataType_NUMERIC,
		cfg.DefaultFrequency,
		cfg.DefaultMinValue,
		cfg.DefaultMaxValue,
		5, // Include 5 historical points
		func(dp *pb.DataPoint) error {
			dataPoints++
			if dataPoints%10 == 0 {
				log.Printf("Received %d data points", dataPoints)
			}
			return nil
		},
	)

	if err != nil && err != context.Canceled {
		log.Fatalf("Stream error: %v", err)
	}

	log.Printf("Stream ended, received %d data points", dataPoints)
	log.Printf("Streaming service shut down")
} 