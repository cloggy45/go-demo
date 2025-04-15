package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/user/proto/gen"
)

// StreamClient is the gRPC client for the RandomService streaming functionality
type StreamClient struct {
	client        pb.RandomServiceClient
	conn          *grpc.ClientConn
	addr          string
	mu            sync.Mutex
	reconnectWait time.Duration
	maxRetries    int
	lastError     error
	isConnected   bool
}

// StreamClientOption is a function that configures a StreamClient
type StreamClientOption func(*StreamClient)

// WithReconnectWait sets the wait time between reconnection attempts
func WithReconnectWait(d time.Duration) StreamClientOption {
	return func(c *StreamClient) {
		c.reconnectWait = d
	}
}

// WithMaxRetries sets the maximum number of reconnection attempts
func WithMaxRetries(n int) StreamClientOption {
	return func(c *StreamClient) {
		c.maxRetries = n
	}
}

// NewStreamClient creates a new gRPC client for the RandomService
func NewStreamClient(addr string, opts ...StreamClientOption) *StreamClient {
	client := &StreamClient{
		addr:          addr,
		reconnectWait: 5 * time.Second,
		maxRetries:    10,
	}

	// Apply options
	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Connect establishes a connection to the gRPC server
func (c *StreamClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isConnected {
		return nil
	}

	// Set up a connection to the gRPC server
	var err error
	c.conn, err = grpc.Dial(c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		c.lastError = fmt.Errorf("failed to connect to server: %v", err)
		return c.lastError
	}

	// Create a client using the connection
	c.client = pb.NewRandomServiceClient(c.conn)
	c.isConnected = true
	log.Printf("Connected to gRPC server at %s", c.addr)

	return nil
}

// Close closes the connection to the gRPC server
func (c *StreamClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.isConnected = false
		log.Printf("Disconnected from gRPC server at %s", c.addr)
	}
}

// IsConnected returns whether the client is connected to the server
func (c *StreamClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnected
}

// GetLastError returns the last error encountered
func (c *StreamClient) GetLastError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// StreamRandomData streams random data from the gRPC server
func (c *StreamClient) StreamRandomData(
	ctx context.Context,
	dataType pb.DataType,
	frequency int32,
	minValue, maxValue int32,
	historySize int32,
	dataHandler func(*pb.DataPoint) error,
) error {
	c.mu.Lock()
	if !c.isConnected {
		c.mu.Unlock()
		if err := c.Connect(); err != nil {
			return err
		}
		c.mu.Lock()
	}
	client := c.client
	c.mu.Unlock()

	// Create the stream request
	req := &pb.StreamRequest{
		DataType:    dataType,
		FrequencyMs: frequency,
		MinValue:    minValue,
		MaxValue:    maxValue,
		HistorySize: historySize,
	}

	var retries int
	var lastErr error

	for retries < c.maxRetries {
		if retries > 0 {
			log.Printf("Retry %d/%d after %v", retries, c.maxRetries, c.reconnectWait)
			time.Sleep(c.reconnectWait)
		}

		// Start the stream
		log.Printf("Starting data stream: type=%v, frequency=%dms, range=[%d,%d], history=%d",
			dataType, frequency, minValue, maxValue, historySize)

		stream, err := client.StreamRandomData(ctx, req)
		if err != nil {
			lastErr = fmt.Errorf("failed to start stream: %v", err)
			log.Printf("Error: %v", lastErr)
			retries++
			
			// Try to reconnect
			c.mu.Lock()
			c.isConnected = false
			c.mu.Unlock()
			c.Connect()
			continue
		}

		// Process the stream
		for {
			// Check if context was canceled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Continue processing
			}

			// Receive the next data point
			dataPoint, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					// End of stream
					return nil
				}

				// Handle different error types
				if status.Code(err) == codes.Canceled {
					// Stream was canceled
					return fmt.Errorf("stream canceled: %v", err)
				}

				// Other error, try to reconnect
				lastErr = fmt.Errorf("error receiving data point: %v", err)
				log.Printf("Stream error: %v", lastErr)
				
				// Mark as disconnected and break out to retry
				c.mu.Lock()
				c.isConnected = false
				c.lastError = lastErr
				c.mu.Unlock()
				break
			}

			// Process the data point
			if err := dataHandler(dataPoint); err != nil {
				return fmt.Errorf("error handling data point: %v", err)
			}
		}

		retries++
	}

	return fmt.Errorf("max retries exceeded: %v", lastErr)
}

// GetRandomNumber gets a random number from the server
func (c *StreamClient) GetRandomNumber(ctx context.Context, min, max int32) (int32, error) {
	c.mu.Lock()
	if !c.isConnected {
		c.mu.Unlock()
		if err := c.Connect(); err != nil {
			return 0, err
		}
		c.mu.Lock()
	}
	client := c.client
	c.mu.Unlock()

	resp, err := client.GetRandomNumber(ctx, &pb.NumberRequest{
		Min: min,
		Max: max,
	})
	if err != nil {
		c.mu.Lock()
		c.lastError = err
		c.mu.Unlock()
		return 0, err
	}

	return resp.Value, nil
}

// GetRandomString gets a random string from the server
func (c *StreamClient) GetRandomString(ctx context.Context, length int32, includeDigits, includeSpecial bool) (string, error) {
	c.mu.Lock()
	if !c.isConnected {
		c.mu.Unlock()
		if err := c.Connect(); err != nil {
			return "", err
		}
		c.mu.Lock()
	}
	client := c.client
	c.mu.Unlock()

	resp, err := client.GetRandomString(ctx, &pb.StringRequest{
		Length:         length,
		IncludeDigits:  includeDigits,
		IncludeSpecial: includeSpecial,
	})
	if err != nil {
		c.mu.Lock()
		c.lastError = err
		c.mu.Unlock()
		return "", err
	}

	return resp.Value, nil
} 