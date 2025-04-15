package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/user/proto/gen"
)

const (
	defaultRestHost      = "0.0.0.0"
	defaultRestPort      = "8080"
	defaultGeneratorHost = "random-generator"
	defaultGeneratorPort = "50051"
)

type RandomServiceClient struct {
	client pb.RandomServiceClient
	conn   *grpc.ClientConn
}

func NewRandomServiceClient(target string) (*RandomServiceClient, error) {
	// Set up a connection to the gRPC server
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to generator service: %v", err)
	}

	// Create a client using the connection
	client := pb.NewRandomServiceClient(conn)
	return &RandomServiceClient{
		client: client,
		conn:   conn,
	}, nil
}

func (r *RandomServiceClient) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}

func main() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using default or environment values")
	}

	// Get REST API config from environment variables
	restHost := getEnv("REST_HOST", defaultRestHost)
	restPort := getEnv("REST_PORT", defaultRestPort)
	restAddr := fmt.Sprintf("%s:%s", restHost, restPort)

	// Get generator service config from environment variables
	generatorHost := getEnv("GENERATOR_HOST", defaultGeneratorHost)
	generatorPort := getEnv("GENERATOR_PORT", defaultGeneratorPort)
	generatorAddr := fmt.Sprintf("%s:%s", generatorHost, generatorPort)

	// Initialize the gRPC client
	log.Printf("Connecting to generator service at %s", generatorAddr)
	randClient, err := NewRandomServiceClient(generatorAddr)
	if err != nil {
		log.Fatalf("Failed to connect to generator service: %v", err)
	}
	defer randClient.Close()

	// Set up the Gin router
	router := gin.Default()

	// Add a simple health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// Random number endpoint
	router.GET("/random/number", func(c *gin.Context) {
		// Parse query parameters
		minParam := c.DefaultQuery("min", "1")
		maxParam := c.DefaultQuery("max", "100")

		min, err := strconv.ParseInt(minParam, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'min' parameter"})
			return
		}

		max, err := strconv.ParseInt(maxParam, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'max' parameter"})
			return
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Call the gRPC service
		resp, err := randClient.client.GetRandomNumber(ctx, &pb.NumberRequest{
			Min: int32(min),
			Max: int32(max),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get random number: %v", err)})
			return
		}

		// Return the response as JSON
		c.JSON(http.StatusOK, gin.H{
			"number": resp.Value,
			"min":    min,
			"max":    max,
		})
	})

	// Random string endpoint
	router.GET("/random/string", func(c *gin.Context) {
		// Parse query parameters
		lengthParam := c.DefaultQuery("length", "10")
		includeDigits := c.DefaultQuery("digits", "true") == "true"
		includeSpecial := c.DefaultQuery("special", "false") == "true"

		length, err := strconv.ParseInt(lengthParam, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid 'length' parameter"})
			return
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Call the gRPC service
		resp, err := randClient.client.GetRandomString(ctx, &pb.StringRequest{
			Length:        int32(length),
			IncludeDigits: includeDigits,
			IncludeSpecial: includeSpecial,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get random string: %v", err)})
			return
		}

		// Return the response as JSON
		c.JSON(http.StatusOK, gin.H{
			"string":         resp.Value,
			"length":         length,
			"include_digits": includeDigits,
			"include_special": includeSpecial,
		})
	})

	// Start the REST server
	log.Printf("Starting REST server on %s", restAddr)
	if err := router.Run(restAddr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
} 