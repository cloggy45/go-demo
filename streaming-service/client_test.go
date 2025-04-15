package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/proto/gen"
	"streaming-service/client"
	"streaming-service/config"
)

func main() {
	log.Println("Starting gRPC client test")

	// Load configuration
	cfg := config.LoadConfig()
	log.Printf("Connecting to gRPC server at %s", cfg.GeneratorAddr())

	// Create a client
	streamClient := client.NewStreamClient(
		cfg.GeneratorAddr(),
		client.WithReconnectWait(cfg.ReconnectWait),
		client.WithMaxRetries(cfg.MaxRetries),
	)

	// Connect to the server
	if err := streamClient.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer streamClient.Close()

	// Create a context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		log.Println("Shutting down...")
		cancel()
	}()

	// Test GetRandomNumber
	num, err := streamClient.GetRandomNumber(ctx, 1, 100)
	if err != nil {
		log.Fatalf("GetRandomNumber failed: %v", err)
	}
	log.Printf("Random number: %d", num)

	// Test GetRandomString
	str, err := streamClient.GetRandomString(ctx, 10, true, false)
	if err != nil {
		log.Fatalf("GetRandomString failed: %v", err)
	}
	log.Printf("Random string: %s", str)

	// Count received data points
	var counter int
	startTime := time.Now()

	// Test StreamRandomData
	log.Println("Starting data stream, press Ctrl+C to stop...")
	err = streamClient.StreamRandomData(
		ctx,
		gen.DataType_NUMERIC,
		1000, // 1 second interval
		1,    // min value
		100,  // max value
		5,    // history size
		func(dp *gen.DataPoint) error {
			counter++
			
			// Convert data point to JSON for display
			jsonData, err := json.MarshalIndent(dp, "", "  ")
			if err != nil {
				log.Printf("Error marshaling data point: %v", err)
				return nil
			}
			
			// Only print every 5th data point to avoid flooding the console
			if counter%5 == 0 {
				log.Printf("Data point #%d:\n%s", counter, string(jsonData))
				log.Printf("Rate: %.2f points/sec", float64(counter)/time.Since(startTime).Seconds())
			}
			
			return nil
		},
	)

	if err != nil {
		if err == context.Canceled {
			log.Println("Stream was canceled")
		} else {
			log.Fatalf("StreamRandomData failed: %v", err)
		}
	}

	log.Printf("Test completed. Received %d data points in %v", counter, time.Since(startTime))
} 