package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/user/proto/gen"
	"streaming-service/client"
)

// DataPoint represents a single data point with timestamp and value
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// DataSeries represents a series of data points
type DataSeries struct {
	Points   []DataPoint          `json:"points"`
	Metadata map[string]string    `json:"metadata,omitempty"`
	mu       sync.Mutex
	maxSize  int
}

// DataStreamBridge connects a gRPC data stream to an SSE server
type DataStreamBridge struct {
	client     *client.StreamClient
	server     *SSEServer
	ctx        context.Context
	cancel     context.CancelFunc
	dataSeries *DataSeries
	dataType   pb.DataType
	frequency  int32
	minValue   int32
	maxValue   int32
	bufferSize int
}

// DataBridgeOption defines a functional option for configuring the DataStreamBridge
type DataBridgeOption func(*DataStreamBridge)

// WithDataType sets the data type for the data stream
func WithDataType(dataType pb.DataType) DataBridgeOption {
	return func(b *DataStreamBridge) {
		b.dataType = dataType
	}
}

// WithFrequency sets the frequency (in milliseconds) for the data stream
func WithFrequency(frequency int32) DataBridgeOption {
	return func(b *DataStreamBridge) {
		b.frequency = frequency
	}
}

// WithValueRange sets the min and max values for the data stream
func WithValueRange(min, max int32) DataBridgeOption {
	return func(b *DataStreamBridge) {
		b.minValue = min
		b.maxValue = max
	}
}

// WithBufferSize sets the buffer size for the data bridge
func WithBufferSize(size int) DataBridgeOption {
	return func(b *DataStreamBridge) {
		b.bufferSize = size
	}
}

// WithHistorySize sets the maximum number of historical data points to keep
func WithHistorySize(size int) DataBridgeOption {
	return func(b *DataStreamBridge) {
		b.dataSeries.maxSize = size
	}
}

// NewDataStreamBridge creates a new bridge between a gRPC client and SSE server
func NewDataStreamBridge(client *client.StreamClient, server *SSEServer, opts ...DataBridgeOption) *DataStreamBridge {
	ctx, cancel := context.WithCancel(context.Background())
	
	bridge := &DataStreamBridge{
		client:     client,
		server:     server,
		ctx:        ctx,
		cancel:     cancel,
		dataSeries: &DataSeries{
			Points:   make([]DataPoint, 0),
			Metadata: make(map[string]string),
			maxSize:  100, // Default history size
		},
		dataType:   pb.DataType_NUMERIC,
		frequency:  1000, // Default: 1 second
		minValue:   0,
		maxValue:   100,
		bufferSize: 10, // Default buffer size
	}
	
	// Apply options
	for _, opt := range opts {
		opt(bridge)
	}
	
	return bridge
}

// Start begins streaming data from the gRPC client to the SSE server
func (b *DataStreamBridge) Start() error {
	log.Printf("Starting data stream bridge")
	log.Printf("Configured with: type=%v, frequency=%d ms, range=[%d, %d]", 
		b.dataType, b.frequency, b.minValue, b.maxValue)
	
	// Start the data stream in a goroutine
	go func() {
		err := b.client.StreamRandomData(
			b.ctx,
			b.dataType,
			b.frequency,
			b.minValue,
			b.maxValue,
			int32(b.dataSeries.maxSize), // Request historical points
			b.handleDataPoint,          // Callback for each data point
		)
		
		if err != nil && err != context.Canceled {
			log.Printf("Error in data stream: %v", err)
		}
	}()
	
	return nil
}

// Stop terminates the data stream
func (b *DataStreamBridge) Stop() {
	log.Printf("Stopping data stream bridge")
	b.cancel()
}

// handleDataPoint processes each data point received from the gRPC stream
func (b *DataStreamBridge) handleDataPoint(dp *pb.DataPoint) error {
	// Convert protobuf timestamp to time.Time
	ts := time.Unix(0, dp.Timestamp)
	
	// Extract value based on the data type
	var val float64
	switch v := dp.Value.(type) {
	case *pb.DataPoint_NumericValue:
		val = float64(v.NumericValue)
	default:
		// Handle other types if needed
		return fmt.Errorf("unsupported data point type: %T", dp.Value)
	}
	
	// Create a data point
	dataPoint := DataPoint{
		Timestamp: ts,
		Value:     val,
	}
	
	// Add to data series
	b.dataSeries.addPoint(dataPoint)
	
	// Get a proper full timestamp in milliseconds
	// This ensures we get a full-length milliseconds timestamp
	currentTime := time.Now().UnixMilli()
	
	// Prepare JSON message with a proper timestamp
	message := map[string]interface{}{
		"timestamp": currentTime,  // Use current time for consistent timestamps
		"value":     val,
	}
	
	// Add metadata if present
	if dp.Metadata != nil && len(dp.Metadata) > 0 {
		message["metadata"] = dp.Metadata
	}
	
	// Convert to JSON
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshaling message: %v", err)
	}
	
	// Log the message for debugging
	log.Printf("Sending SSE message: %s", string(jsonData))
	
	// Broadcast to all SSE clients with the 'data' event type
	b.server.BroadcastEvent("data", string(jsonData))
	
	return nil
}

// GetDataSeries returns the current data series
func (b *DataStreamBridge) GetDataSeries() *DataSeries {
	return b.dataSeries
}

// addPoint adds a data point to the series, maintaining the maximum size
func (ds *DataSeries) addPoint(point DataPoint) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	
	// Add the new point
	ds.Points = append(ds.Points, point)
	
	// Trim if we exceed the maximum size
	if len(ds.Points) > ds.maxSize && ds.maxSize > 0 {
		ds.Points = ds.Points[len(ds.Points)-ds.maxSize:]
	}
}

// ToJSON converts the data series to a JSON string
func (ds *DataSeries) ToJSON() (string, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	
	// Create a copy of the points slice to avoid race conditions
	pointsCopy := make([]DataPoint, len(ds.Points))
	copy(pointsCopy, ds.Points)
	
	// Create a response structure
	response := struct {
		Points   []DataPoint       `json:"points"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}{
		Points:   pointsCopy,
		Metadata: ds.Metadata,
	}
	
	// Convert to JSON
	jsonData, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("error marshaling data series: %v", err)
	}
	
	return string(jsonData), nil
} 