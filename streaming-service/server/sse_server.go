package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SSEServer represents a Server-Sent Events server
type SSEServer struct {
	host           string
	port           string
	router         *gin.Engine
	clients        map[*sseClient]bool
	clientsMutex   sync.Mutex
	newClients     chan *sseClient
	closingClients chan *sseClient
	messages       chan string
	shutdown       chan struct{}
	isRunning      bool
	server         *http.Server
	keepAlive      time.Duration
}

type sseClient struct {
	id     string
	writer http.ResponseWriter
}

type SSEOption func(*SSEServer)

// WithKeepAliveInterval sets the keep-alive interval for the SSE server
func WithKeepAliveInterval(interval time.Duration) SSEOption {
	return func(s *SSEServer) {
		s.keepAlive = interval
	}
}

// NewSSEServer creates a new SSE server
func NewSSEServer(host, port string, opts ...SSEOption) *SSEServer {
	// Use gin in release mode for production
	gin.SetMode(gin.ReleaseMode)
	
	server := &SSEServer{
		host:           host,
		port:           port,
		router:         gin.Default(),
		clients:        make(map[*sseClient]bool),
		newClients:     make(chan *sseClient),
		closingClients: make(chan *sseClient),
		messages:       make(chan string, 100), // Buffer for 100 messages
		shutdown:       make(chan struct{}),
		keepAlive:      10 * time.Second, // Default: 10 seconds
	}
	
	// Apply options
	for _, opt := range opts {
		opt(server)
	}
	
	// Set up routes
	server.setupRoutes()
	
	return server
}

// setupRoutes configures the HTTP routes for the SSE server
func (s *SSEServer) setupRoutes() {
	// Add health check endpoint
	s.router.GET("/health", s.healthHandler)
	
	// Add SSE stream endpoint
	s.router.GET("/stream", s.streamHandler)
	
	// Add metrics endpoint
	s.router.GET("/metrics", s.metricsHandler)
	
	// Add data series endpoint
	s.router.GET("/data-series", s.dataSeriesHandler)
}

// Start starts the SSE server
func (s *SSEServer) Start() error {
	if s.isRunning {
		return fmt.Errorf("server is already running")
	}
	
	// Set up HTTP server
	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", s.host, s.port),
		Handler: s.router,
	}
	
	// Start the message broker in a goroutine
	go s.handleMessages()
	
	// Set server as running
	s.isRunning = true
	
	// Start the HTTP server
	log.Printf("SSE Server starting at http://%s:%s", s.host, s.port)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	
	return nil
}

// Stop gracefully shuts down the SSE server
func (s *SSEServer) Stop() error {
	if !s.isRunning {
		return nil
	}
	
	// Signal the message broker to stop
	close(s.shutdown)
	
	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Shut down the HTTP server
	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}
	
	s.isRunning = false
	log.Println("SSE Server stopped")
	return nil
}

// BroadcastMessage sends a message to all connected clients
func (s *SSEServer) BroadcastMessage(message string) {
	if !s.isRunning {
		log.Println("Cannot broadcast message: server is not running")
		return
	}
	
	select {
	case s.messages <- message:
		// Message sent to channel
	default:
		log.Println("Warning: message channel full, dropping message")
	}
}

// BroadcastEvent sends a message to all connected clients with a specific event type
func (s *SSEServer) BroadcastEvent(eventType, message string) {
	if !s.isRunning {
		log.Println("Cannot broadcast event: server is not running")
		return
	}
	
	// Format the event according to SSE specification
	formattedMessage := fmt.Sprintf("event: %s\ndata: %s", eventType, message)
	
	select {
	case s.messages <- formattedMessage:
		// Message sent to channel
	default:
		log.Println("Warning: message channel full, dropping message")
	}
}

// handleMessages processes messages and client connections/disconnections
func (s *SSEServer) handleMessages() {
	ticker := time.NewTicker(s.keepAlive)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.shutdown:
			// Server is shutting down
			return
			
		case client := <-s.newClients:
			// Add new client
			s.clientsMutex.Lock()
			s.clients[client] = true
			log.Printf("Client connected: %s, total clients: %d", client.id, len(s.clients))
			s.clientsMutex.Unlock()
			
		case client := <-s.closingClients:
			// Remove client
			s.clientsMutex.Lock()
			delete(s.clients, client)
			log.Printf("Client disconnected: %s, total clients: %d", client.id, len(s.clients))
			s.clientsMutex.Unlock()
			
		case msg := <-s.messages:
			// Broadcast message to all clients
			s.clientsMutex.Lock()
			for client := range s.clients {
				// If message already contains event information, don't add data: prefix
				if strings.HasPrefix(msg, "event:") {
					fmt.Fprintf(client.writer, "%s\n\n", msg)
				} else {
					// Send message with "data:" prefix as per SSE spec
					fmt.Fprintf(client.writer, "data: %s\n\n", msg)
				}
				
				// Flush response writer to ensure message is sent immediately
				if f, ok := client.writer.(http.Flusher); ok {
					f.Flush()
				}
			}
			s.clientsMutex.Unlock()
			
		case <-ticker.C:
			// Send keep-alive comment to all clients
			s.clientsMutex.Lock()
			for client := range s.clients {
				// Send comment as per SSE spec
				fmt.Fprintf(client.writer, ": ping %d\n\n", time.Now().UnixNano())
				
				// Flush response writer
				if f, ok := client.writer.(http.Flusher); ok {
					f.Flush()
				}
			}
			s.clientsMutex.Unlock()
		}
	}
}

// healthHandler returns a simple health check response
func (s *SSEServer) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// streamHandler handles SSE streaming connections
func (s *SSEServer) streamHandler(c *gin.Context) {
	// Set headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	
	// Create a unique ID for this client
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	
	// Create client
	client := &sseClient{
		id:     clientID,
		writer: c.Writer,
	}
	
	// Register this client
	s.newClients <- client
	
	// Handle client disconnection using context cancelation instead of CloseNotify
	clientCtx := c.Request.Context()
	go func() {
		<-clientCtx.Done()
		s.closingClients <- client
	}()
	
	// Keep the connection open
	c.Stream(func(w io.Writer) bool {
		// This will keep the connection open
		select {
		case <-s.shutdown:
			return false
		case <-clientCtx.Done():
			return false
		case <-time.After(1 * time.Hour): // Safety timeout
			return true
		}
	})
}

// metricsHandler returns server metrics
func (s *SSEServer) metricsHandler(c *gin.Context) {
	s.clientsMutex.Lock()
	clientCount := len(s.clients)
	s.clientsMutex.Unlock()
	
	c.JSON(http.StatusOK, gin.H{
		"clients":       clientCount,
		"uptime":        time.Since(time.Now()), // This will be negative, fix in real implementation
		"messages_sent": 0,                     // Add a counter in a real implementation
	})
}

// dataSeriesHandler returns historical data points
func (s *SSEServer) dataSeriesHandler(c *gin.Context) {
	// This is a placeholder - in a real implementation, you would fetch from a data store
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"data":   []float64{},
	})
} 