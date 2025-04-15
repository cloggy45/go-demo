package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "github.com/user/proto/gen"
)

const (
	defaultHost = "0.0.0.0"
	defaultPort = "50051"
	// Default values for streaming
	defaultFrequency = 1000 // 1 second in milliseconds
	defaultMinValue  = 0
	defaultMaxValue  = 100
	maxBackpressure  = 100 // Maximum number of pending messages before applying backpressure
	
	// Time resolution constants
	ResolutionRaw    = "raw"     // Raw data, no aggregation
	ResolutionSecond = "second"  // Aggregate per second
	ResolutionMinute = "minute"  // Aggregate per minute
	ResolutionHour   = "hour"    // Aggregate per hour
	
	// Log levels
	LogLevelError = "ERROR"
	LogLevelWarn  = "WARN"
	LogLevelInfo  = "INFO"
	LogLevelDebug = "DEBUG"
)

// Logger provides structured logging with different log levels
type Logger struct {
	level     string
	component string
	writer    io.Writer
	mu        sync.Mutex
}

// NewLogger creates a new logger with the specified level and component name
func NewLogger(level, component string, writer io.Writer) *Logger {
	return &Logger{
		level:     strings.ToUpper(level),
		component: component,
		writer:    writer,
	}
}

// SetLevel changes the current log level
func (l *Logger) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = strings.ToUpper(level)
}

// shouldLog checks if the given level should be logged based on current settings
func (l *Logger) shouldLog(level string) bool {
	level = strings.ToUpper(level)
	switch l.level {
	case LogLevelDebug:
		return true
	case LogLevelInfo:
		return level != LogLevelDebug
	case LogLevelWarn:
		return level == LogLevelWarn || level == LogLevelError
	case LogLevelError:
		return level == LogLevelError
	default:
		return true
	}
}

// log handles the actual logging with timestamp, level, and component
func (l *Logger) log(level, format string, args ...interface{}) {
	if !l.shouldLog(level) {
		return
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	message := fmt.Sprintf(format, args...)
	
	fmt.Fprintf(l.writer, "[%s] [%s] [%s] %s\n", timestamp, level, l.component, message)
}

// Debug logs debug level messages
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LogLevelDebug, format, args...)
}

// Info logs info level messages
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LogLevelInfo, format, args...)
}

// Warn logs warning level messages
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LogLevelWarn, format, args...)
}

// Error logs error level messages
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LogLevelError, format, args...)
}

// Fatal logs an error message and then exits the application
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LogLevelError, format, args...)
	os.Exit(1)
}

// Global logger
var logger *Logger

// TimeResolution represents a time resolution for data aggregation
type TimeResolution struct {
	Name     string
	Duration time.Duration
}

// Available time resolutions
var (
	ResolutionRawValue    = TimeResolution{Name: ResolutionRaw, Duration: time.Millisecond}
	ResolutionSecondValue = TimeResolution{Name: ResolutionSecond, Duration: time.Second}
	ResolutionMinuteValue = TimeResolution{Name: ResolutionMinute, Duration: time.Minute}
	ResolutionHourValue   = TimeResolution{Name: ResolutionHour, Duration: time.Hour}
)

// aggregatedValue stores aggregate statistics for a time window
type aggregatedValue struct {
	count int
	sum   float64
	min   float64
	max   float64
	last  float64
}

// DataProducer continuously produces data
type DataProducer struct {
	dataType      pb.DataType
	minValue      int32
	maxValue      int32
	frequency     time.Duration
	rnd           *rand.Rand
	mu            sync.Mutex
	stopCh        chan struct{}
	dataPoints    chan *pb.DataPoint
	sequence      *int64
	resolution    TimeResolution
	aggregates    map[int64]*aggregatedValue // Key is timestamp bucket
	lastAggregate int64                      // Last sent aggregate timestamp
	logger        *Logger
	id            string // Unique identifier for this producer
}

// NewDataProducer creates a new data producer
func NewDataProducer(
	dataType pb.DataType,
	minValue, maxValue int32,
	frequency time.Duration,
	rnd *rand.Rand,
	sequence *int64,
	resolution TimeResolution,
	id string,
) *DataProducer {
	return &DataProducer{
		dataType:   dataType,
		minValue:   minValue,
		maxValue:   maxValue,
		frequency:  frequency,
		rnd:        rnd,
		stopCh:     make(chan struct{}),
		dataPoints: make(chan *pb.DataPoint, maxBackpressure),
		sequence:   sequence,
		resolution: resolution,
		aggregates: make(map[int64]*aggregatedValue),
		logger:     NewLogger(getEnv("LOG_LEVEL", LogLevelInfo), fmt.Sprintf("producer-%s", id), os.Stdout),
		id:         id,
	}
}

// Start begins data production
func (dp *DataProducer) Start() {
	dp.logger.Debug("Starting data producer: id=%s, type=%v, range=[%d,%d], frequency=%v, resolution=%s",
		dp.id, dp.dataType, dp.minValue, dp.maxValue, dp.frequency, dp.resolution.Name)
	go dp.produceData()
}

// Stop halts data production
func (dp *DataProducer) Stop() {
	dp.logger.Debug("Stopping data producer: id=%s", dp.id)
	close(dp.stopCh)
}

// DataChannel returns the channel of produced data points
func (dp *DataProducer) DataChannel() <-chan *pb.DataPoint {
	return dp.dataPoints
}

// produceData continuously generates data points at the specified frequency
func (dp *DataProducer) produceData() {
	ticker := time.NewTicker(dp.frequency)
	defer ticker.Stop()
	
	pointsGenerated := 0
	startTime := time.Now()
	logInterval := time.Second * 10 // Log stats every 10 seconds
	lastLogTime := startTime
	
	dp.logger.Debug("Data production started: id=%s", dp.id)
	
	for {
		select {
		case <-ticker.C:
			// Generate a data point
			dataPoint := dp.generateDataPoint()
			pointsGenerated++
			
			// Log statistics periodically
			now := time.Now()
			if now.Sub(lastLogTime) >= logInterval {
				elapsed := now.Sub(startTime).Seconds()
				rate := float64(pointsGenerated) / elapsed
				dp.logger.Debug("Producer stats: id=%s, points=%d, rate=%.2f/sec", 
					dp.id, pointsGenerated, rate)
				lastLogTime = now
			}
			
			// If using raw resolution, send directly
			if dp.resolution.Name == ResolutionRaw {
				select {
				case dp.dataPoints <- dataPoint:
					// Data point sent
				case <-dp.stopCh:
					dp.logger.Debug("Producer stopped during send: id=%s", dp.id)
					return
				default:
					// Channel is full, apply backpressure
					dp.logger.Warn("Backpressure applied, dropping point: id=%s", dp.id)
				}
				continue
			}
			
			// Otherwise, aggregate the data point
			dp.aggregateDataPoint(dataPoint)
			
			// Check if it's time to send an aggregate
			now = time.Now()
			bucketTime := now.Truncate(dp.resolution.Duration)
			bucketTimestamp := bucketTime.UnixMilli()
			
			// Send aggregates when moving to a new time bucket
			if bucketTimestamp > dp.lastAggregate && dp.lastAggregate > 0 {
				dp.sendAggregates()
			}
			
		case <-dp.stopCh:
			// Producer stopped, send any remaining aggregates
			dp.logger.Debug("Producer received stop signal: id=%s", dp.id)
			dp.sendAggregates()
			dp.logger.Debug("Producer stopped: id=%s, total points=%d", dp.id, pointsGenerated)
			return
		}
	}
}

// aggregateDataPoint adds a data point to the current time bucket
func (dp *DataProducer) aggregateDataPoint(dataPoint *pb.DataPoint) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	
	// Determine the time bucket for this data point
	pointTime := time.UnixMilli(dataPoint.Timestamp)
	bucketTime := pointTime.Truncate(dp.resolution.Duration)
	bucketTimestamp := bucketTime.UnixMilli()
	
	// Get or create the aggregate for this bucket
	agg, exists := dp.aggregates[bucketTimestamp]
	if !exists {
		agg = &aggregatedValue{
			min: float64(dp.maxValue) + 1, // Initialize min to a value higher than possible
			max: float64(dp.minValue) - 1, // Initialize max to a value lower than possible
		}
		dp.aggregates[bucketTimestamp] = agg
	}
	
	// Extract the value and update aggregates
	var value float64
	switch dataPoint.Value.(type) {
	case *pb.DataPoint_NumericValue:
		value = float64(dataPoint.GetNumericValue())
	case *pb.DataPoint_FloatValue:
		value = float64(dataPoint.GetFloatValue())
	case *pb.DataPoint_BooleanValue:
		if dataPoint.GetBooleanValue() {
			value = 1.0
		} else {
			value = 0.0
		}
	}
	
	// Update the aggregate statistics
	agg.count++
	agg.sum += value
	agg.last = value
	if value < agg.min {
		agg.min = value
	}
	if value > agg.max {
		agg.max = value
	}
}

// sendAggregates sends all completed aggregates as data points
func (dp *DataProducer) sendAggregates() {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	
	now := time.Now()
	currentBucket := now.Truncate(dp.resolution.Duration).UnixMilli()
	
	// Send all aggregates except the current one (which might still be collecting data)
	for bucketTimestamp, agg := range dp.aggregates {
		if bucketTimestamp >= currentBucket {
			continue
		}
		
		// Create a metadata map with aggregate statistics
		metadata := map[string]string{
			"resolution": dp.resolution.Name,
			"count":      fmt.Sprintf("%d", agg.count),
			"min":        fmt.Sprintf("%f", agg.min),
			"max":        fmt.Sprintf("%f", agg.max),
			"avg":        fmt.Sprintf("%f", agg.sum/float64(agg.count)),
		}
		
		// Create a data point with the aggregate value (we use avg as the main value)
		dataPoint := &pb.DataPoint{
			Timestamp: bucketTimestamp,
			Sequence:  atomic.AddInt64(dp.sequence, 1),
			Metadata:  metadata,
			Series:    dp.resolution.Name, // Use resolution as series name
		}
		
		// Set the appropriate value type
		switch dp.dataType {
		case pb.DataType_NUMERIC:
			avgValue := int32(agg.sum / float64(agg.count))
			dataPoint.Value = &pb.DataPoint_NumericValue{NumericValue: avgValue}
		case pb.DataType_PERCENTAGE:
			avgValue := float32(agg.sum / float64(agg.count))
			dataPoint.Value = &pb.DataPoint_FloatValue{FloatValue: avgValue}
		case pb.DataType_BOOLEAN:
			// For boolean, we convert to true if average > 0.5
			boolValue := (agg.sum / float64(agg.count)) > 0.5
			dataPoint.Value = &pb.DataPoint_BooleanValue{BooleanValue: boolValue}
		}
		
		// Send the aggregate data point
		select {
		case dp.dataPoints <- dataPoint:
			// Data point sent
		case <-dp.stopCh:
			return
		}
		
		// Delete the sent aggregate
		delete(dp.aggregates, bucketTimestamp)
		dp.lastAggregate = bucketTimestamp
	}
}

// generateDataPoint creates a new data point with the current timestamp
func (dp *DataProducer) generateDataPoint() *pb.DataPoint {
	// Lock to safely access the random number generator
	dp.mu.Lock()
	defer dp.mu.Unlock()
	
	// Create a new data point with current timestamp and sequence number
	dataPoint := &pb.DataPoint{
		Timestamp: time.Now().UnixMilli(),
		Sequence:  atomic.AddInt64(dp.sequence, 1),
		Metadata:  make(map[string]string),
	}
	
	// Generate value based on data type
	switch dp.dataType {
	case pb.DataType_NUMERIC:
		value := dp.minValue + int32(dp.rnd.Intn(int(dp.maxValue-dp.minValue+1)))
		dataPoint.Value = &pb.DataPoint_NumericValue{NumericValue: value}
		
	case pb.DataType_PERCENTAGE:
		value := float32(dp.rnd.Intn(101)) // 0-100
		dataPoint.Value = &pb.DataPoint_FloatValue{FloatValue: value}
		
	case pb.DataType_BOOLEAN:
		value := dp.rnd.Intn(2) == 1 // 50% chance true/false
		dataPoint.Value = &pb.DataPoint_BooleanValue{BooleanValue: value}
		
	default:
		// Default to numeric if type is unknown
		value := dp.minValue + int32(dp.rnd.Intn(int(dp.maxValue-dp.minValue+1)))
		dataPoint.Value = &pb.DataPoint_NumericValue{NumericValue: value}
	}
	
	return dataPoint
}

// randomServer is the implementation of the RandomService
type randomServer struct {
	pb.UnimplementedRandomServiceServer
	rnd       *rand.Rand
	mu        sync.Mutex // Protects the random number generator
	sequence  int64      // Atomic counter for sequence numbers
	streamWg  sync.WaitGroup // Used for graceful shutdown
	producers map[string]*DataProducer // Active data producers
	producerMu sync.Mutex // Protects the producers map
	logger    *Logger // Server logger
}

// newRandomServer creates a new random server instance
func newRandomServer(rng *rand.Rand) *randomServer {
	return &randomServer{
		rnd:       rng,
		sequence:  0,
		producers: make(map[string]*DataProducer),
		logger:    NewLogger(getEnv("LOG_LEVEL", LogLevelInfo), "random-server", os.Stdout),
	}
}

// GetRandomNumber generates a random number within the requested range
func (s *randomServer) GetRandomNumber(ctx context.Context, req *pb.NumberRequest) (*pb.NumberResponse, error) {
	min := req.GetMin()
	max := req.GetMax()

	if min > max {
		s.logger.Warn("Invalid range, swapping min and max: min=%d, max=%d", min, max)
		min, max = max, min // Swap if min > max
	}

	s.mu.Lock()
	randomNum := min + int32(s.rnd.Intn(int(max-min+1)))
	s.mu.Unlock()
	
	s.logger.Debug("GetRandomNumber request: min=%d, max=%d, result=%d", min, max, randomNum)
	
	return &pb.NumberResponse{
		Value: randomNum,
	}, nil
}

// GetRandomString generates a random string with the requested length and character set
func (s *randomServer) GetRandomString(ctx context.Context, req *pb.StringRequest) (*pb.StringResponse, error) {
	length := req.GetLength()
	includeDigits := req.GetIncludeDigits()
	includeSpecial := req.GetIncludeSpecial()

	if length <= 0 || length > 1000 {
		err := status.Errorf(codes.InvalidArgument, "length must be between 1 and 1000, got %d", length)
		s.logger.Error("Invalid string length: %d", length)
		return nil, err
	}

	// Define character sets
	letters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits := "0123456789"
	special := "!@#$%^&*()-_=+[]{}|;:,.<>?/"

	// Build the character set based on the request
	chars := letters
	if includeDigits {
		chars += digits
	}
	if includeSpecial {
		chars += special
	}

	// Generate the random string
	s.mu.Lock()
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[s.rnd.Intn(len(chars))]
	}
	s.mu.Unlock()

	randomStr := string(result)
	s.logger.Debug("GetRandomString request: length=%d, includeDigits=%v, includeSpecial=%v, result=%s",
		length, includeDigits, includeSpecial, randomStr)

	return &pb.StringResponse{
		Value: randomStr,
	}, nil
}

// StreamRandomData implements the server streaming RPC method
// It streams random data points at the requested frequency
func (s *randomServer) StreamRandomData(req *pb.StreamRequest, stream pb.RandomService_StreamRandomDataServer) error {
	// Extract peer information for logging
	peerInfo, ok := peer.FromContext(stream.Context())
	peerAddr := "unknown"
	if ok {
		peerAddr = peerInfo.Addr.String()
	}

	// Increment wait group counter for graceful shutdown
	s.streamWg.Add(1)
	defer s.streamWg.Done()

	// Get parameters from request with defaults
	dataType := req.GetDataType()
	frequency := req.GetFrequencyMs()
	minValue := req.GetMinValue()
	maxValue := req.GetMaxValue()
	historySize := req.GetHistorySize()
	
	// Extract resolution from metadata if present
	ctx := stream.Context()
	resolution := ResolutionRawValue // Default to raw data
	if md, ok := ctx.Value("resolution").(string); ok {
		switch md {
		case ResolutionSecond:
			resolution = ResolutionSecondValue
		case ResolutionMinute:
			resolution = ResolutionMinuteValue
		case ResolutionHour:
			resolution = ResolutionHourValue
		}
	}

	// Apply defaults if values are not provided or invalid
	if frequency <= 0 {
		s.logger.Debug("Using default frequency for client %s: %dms", peerAddr, defaultFrequency)
		frequency = defaultFrequency
	}
	
	if minValue >= maxValue {
		s.logger.Warn("Invalid range from client %s, using defaults: min=%d, max=%d", 
			peerAddr, minValue, maxValue)
		minValue = defaultMinValue
		maxValue = defaultMaxValue
	}

	s.logger.Info("Stream started: client=%s, type=%v, frequency=%dms, range=[%d,%d], history=%d, resolution=%s",
		peerAddr, dataType, frequency, minValue, maxValue, historySize, resolution.Name)

	// Create a unique producer ID for this stream
	producerID := fmt.Sprintf("stream-%d", atomic.AddInt64(&s.sequence, 1))
	
	// Set up a data producer for this stream
	producer := NewDataProducer(
		dataType,
		minValue,
		maxValue,
		time.Duration(frequency)*time.Millisecond,
		s.rnd,
		&s.sequence,
		resolution,
		producerID,
	)
	
	// Register the producer
	s.producerMu.Lock()
	s.producers[producerID] = producer
	activeStreams := len(s.producers)
	s.producerMu.Unlock()
	
	s.logger.Debug("Producer registered: id=%s, active_streams=%d", producerID, activeStreams)
	
	// Stream metrics
	pointsSent := 0
	startTime := time.Now()
	
	// Ensure producer is removed when done
	defer func() {
		producer.Stop()
		s.producerMu.Lock()
		delete(s.producers, producerID)
		remainingStreams := len(s.producers)
		s.producerMu.Unlock()
		
		elapsed := time.Since(startTime).Seconds()
		s.logger.Info("Stream ended: client=%s, id=%s, points_sent=%d, duration=%.2fs, points_per_sec=%.2f, remaining_streams=%d",
			peerAddr, producerID, pointsSent, elapsed, float64(pointsSent)/elapsed, remainingStreams)
	}()

	// Catch and log panics
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic in stream handler: id=%s, error=%v", producerID, r)
		}
	}()

	// Send initial historical data if requested
	if historySize > 0 {
		s.logger.Debug("Sending historical data: id=%s, count=%d", producerID, historySize)
		if err := s.sendHistoricalData(stream, dataType, minValue, maxValue, historySize); err != nil {
			s.logger.Error("Error sending historical data: id=%s, error=%v", producerID, err)
			
			// Classify and return appropriate error type
			if status.Code(err) == codes.Canceled {
				s.logger.Info("Client canceled stream during historical data: id=%s", producerID)
				return status.Error(codes.Canceled, "stream canceled by client")
			}
			return status.Errorf(codes.Internal, "failed to send historical data: %v", err)
		}
		pointsSent += int(historySize)
	}
	
	// Create a context with cancellation for cleanup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	
	// Monitor client connection health
	go func() {
		<-ctx.Done()
		s.logger.Debug("Stream context done: id=%s, reason=%v", producerID, ctx.Err())
	}()
	
	// Start the data producer
	producer.Start()
	
	// Process data from the producer
	dataChannel := producer.DataChannel()
	for {
		select {
		case dataPoint, ok := <-dataChannel:
			if !ok {
				// Channel closed, producer stopped
				s.logger.Debug("Producer channel closed: id=%s", producerID)
				return nil
			}
			
			// Attempt to send the data point
			if err := stream.Send(dataPoint); err != nil {
				// Classify error type
				statusCode := status.Code(err)
				if statusCode == codes.Canceled || statusCode == codes.Unavailable {
					s.logger.Info("Client disconnected: id=%s, code=%s, error=%v", 
						producerID, statusCode.String(), err)
				} else {
					s.logger.Error("Error sending data point: id=%s, code=%s, error=%v", 
						producerID, statusCode.String(), err)
				}
				return err
			}
			
			pointsSent++
			
			// Log progress periodically
			if pointsSent%1000 == 0 {
				elapsed := time.Since(startTime).Seconds()
				s.logger.Debug("Stream progress: id=%s, points_sent=%d, rate=%.2f/sec", 
					producerID, pointsSent, float64(pointsSent)/elapsed)
			}
			
		case <-stream.Context().Done():
			// Client disconnected or context was canceled
			err := stream.Context().Err()
			s.logger.Info("Stream context done: id=%s, error=%v", producerID, err)
			
			if err == context.Canceled {
				return status.Error(codes.Canceled, "stream canceled by client")
			} else if err == context.DeadlineExceeded {
				return status.Error(codes.DeadlineExceeded, "stream deadline exceeded")
			}
			return err
		}
	}
}

// sendHistoricalData sends a batch of historical data points
func (s *randomServer) sendHistoricalData(
	stream pb.RandomService_StreamRandomDataServer,
	dataType pb.DataType,
	minValue, maxValue, count int32,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	now := time.Now()
	
	// Don't send too much historical data
	if count > 1000 {
		s.logger.Warn("Limiting historical data from %d to 1000 points", count)
		count = 1000
	}
	
	s.logger.Debug("Generating %d historical data points", count)
	
	for i := int32(0); i < count; i++ {
		// Generate data point with timestamp in the past
		pastTime := now.Add(-time.Duration(count-i) * time.Second)
		
		// Create a data point
		dataPoint := &pb.DataPoint{
			Timestamp: pastTime.UnixMilli(),
			Sequence:  atomic.AddInt64(&s.sequence, 1),
			Metadata:  make(map[string]string),
		}
		
		// Generate value based on data type
		switch dataType {
		case pb.DataType_NUMERIC:
			value := minValue + int32(s.rnd.Intn(int(maxValue-minValue+1)))
			dataPoint.Value = &pb.DataPoint_NumericValue{NumericValue: value}
			
		case pb.DataType_PERCENTAGE:
			value := float32(s.rnd.Intn(101)) // 0-100
			dataPoint.Value = &pb.DataPoint_FloatValue{FloatValue: value}
			
		case pb.DataType_BOOLEAN:
			value := s.rnd.Intn(2) == 1 // 50% chance true/false
			dataPoint.Value = &pb.DataPoint_BooleanValue{BooleanValue: value}
			
		default:
			// Default to numeric if type is unknown
			value := minValue + int32(s.rnd.Intn(int(maxValue-minValue+1)))
			dataPoint.Value = &pb.DataPoint_NumericValue{NumericValue: value}
		}
		
		// Send the historical data point
		if err := stream.Send(dataPoint); err != nil {
			s.logger.Warn("Error sending historical data point %d/%d: %v", i+1, count, err)
			return handleStreamError(err)
		}
		
		// Check for context cancellation between sends
		if stream.Context().Err() != nil {
			s.logger.Info("Stream context canceled during historical data send: %v", stream.Context().Err())
			return handleStreamError(stream.Context().Err())
		}
	}
	
	s.logger.Debug("Successfully sent %d historical data points", count)
	return nil
}

// handleStreamError converts common errors to appropriate gRPC status errors
func handleStreamError(err error) error {
	if err == nil {
		return nil
	}
	
	if err == context.Canceled {
		return status.Error(codes.Canceled, "stream canceled by client")
	}
	
	if err == context.DeadlineExceeded {
		return status.Error(codes.DeadlineExceeded, "stream deadline exceeded")
	}
	
	// If it's already a gRPC status error, return it directly
	if _, ok := status.FromError(err); ok {
		return err
	}
	
	// Default to internal error
	return status.Errorf(codes.Internal, "stream error: %v", err)
}

func main() {
	// Record the start time for uptime reporting
	startTime := time.Now()
	
	// Initialize the global logger
	logLevel := getEnv("LOG_LEVEL", LogLevelInfo)
	logger = NewLogger(logLevel, "main", os.Stdout)
	
	// Log startup information
	logger.Info("Starting random-generator service")
	logger.Info("Log level set to %s", logLevel)
	
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		logger.Info("No .env file found, using default or environment values")
	}

	// Set up a seed for the random generator
	seed := time.Now().UnixNano()
	logger.Debug("Initializing random generator with seed: %d", seed)
	rng := rand.New(rand.NewSource(seed))

	// Get host and port from environment variables
	host := getEnv("GRPC_HOST", defaultHost)
	port := getEnv("GRPC_PORT", defaultPort)
	addr := fmt.Sprintf("%s:%s", host, port)
	logger.Info("Server will listen on %s", addr)

	// Create a listener on the specified port
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("Failed to listen: %v", err)
	}

	// Configure keepalive options for better streaming performance
	kaParams := keepalive.ServerParameters{
		MaxConnectionIdle:     2 * time.Minute,  // If a client is idle for 2 minutes, send a GOAWAY
		MaxConnectionAge:      5 * time.Minute,  // If any connection is alive for more than 5 minutes, send a GOAWAY
		MaxConnectionAgeGrace: 30 * time.Second, // Allow 30 seconds for pending RPCs to complete before forcibly closing connections
		Time:                  20 * time.Second, // Ping the client if it is idle for 20 seconds to ensure the connection is still active
		Timeout:               10 * time.Second, // Wait 10 seconds for the ping ACK before assuming the connection is dead
	}

	kaPolicy := keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second, // If a client pings more than once every 10 seconds, terminate the connection
		PermitWithoutStream: true,            // Allow pings even when there are no active streams
	}
	
	logger.Debug("Configured keepalive parameters: idle=%v, age=%v, grace=%v, ping=%v, timeout=%v",
		kaParams.MaxConnectionIdle, kaParams.MaxConnectionAge, kaParams.MaxConnectionAgeGrace,
		kaParams.Time, kaParams.Timeout)

	// Create a new gRPC server with keepalive options
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(kaParams),
		grpc.KeepaliveEnforcementPolicy(kaPolicy),
	)

	// Create the random server
	server := newRandomServer(rng)

	// Register our service
	pb.RegisterRandomServiceServer(grpcServer, server)

	// Register reflection service on gRPC server
	reflection.Register(grpcServer)
	logger.Debug("Registered gRPC reflection service")
    
	// Create a context that will be canceled on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handler for graceful shutdown
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		
		sig := <-signals
		logger.Info("Received shutdown signal: %v", sig)
		cancel() // Cancel the context, which will trigger shutdown
	}()

	// Set up graceful shutdown
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down gRPC server...")
		
		// Stop all active producers
		server.producerMu.Lock()
		producerCount := len(server.producers)
		for id, producer := range server.producers {
			logger.Debug("Stopping producer: %s", id)
			producer.Stop()
		}
		server.producerMu.Unlock()
		logger.Info("Stopped %d active producers", producerCount)
		
		// Graceful stop - wait for existing streams to finish
		stopped := make(chan struct{})
		go func() {
			server.streamWg.Wait()
			close(stopped)
		}()
		
		// Give streams some time to complete, but don't wait forever
		logger.Debug("Waiting for streams to complete...")
		select {
		case <-stopped:
			logger.Info("All streams completed gracefully")
		case <-time.After(10 * time.Second):
			logger.Warn("Timeout waiting for streams to complete, forcing shutdown")
		}
		
		grpcServer.GracefulStop()
		logger.Info("gRPC server shutdown complete")
	}()

	// Add health check endpoint for monitoring
	go func() {
		healthMux := http.NewServeMux()
		healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			
			// Count active streams
			server.producerMu.Lock()
			activeStreams := len(server.producers)
			server.producerMu.Unlock()
			
			// Log health check request
			logger.Debug("Health check request from %s (active streams: %d)", 
				r.RemoteAddr, activeStreams)
			
			// Return detailed health status
			w.Write([]byte(fmt.Sprintf(`{
				"status": "ok",
				"service": "random-generator",
				"timestamp": "%s",
				"activeStreams": %d,
				"uptime": "%s"
			}`, time.Now().Format(time.RFC3339), activeStreams, time.Since(startTime).String())))
		})
		
		// Add a stream count endpoint
		healthMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			
			// Count active streams
			server.producerMu.Lock()
			activeStreams := len(server.producers)
			server.producerMu.Unlock()
			
			// Return Prometheus-style metrics
			fmt.Fprintf(w, "# HELP random_generator_active_streams Number of active streaming connections\n")
			fmt.Fprintf(w, "# TYPE random_generator_active_streams gauge\n")
			fmt.Fprintf(w, "random_generator_active_streams %d\n", activeStreams)
		})
		
		healthAddr := fmt.Sprintf("%s:8080", host)
		logger.Info("Starting health check server on %s", healthAddr)
		if err := http.ListenAndServe(healthAddr, healthMux); err != nil {
			logger.Error("Health check server error: %v", err)
		}
	}()

	logger.Info("gRPC server started on %s", addr)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("Failed to serve: %v", err)
	}
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
} 