package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/user/proto/gen"
)

const (
	defaultHost = "0.0.0.0"
	defaultPort = "50051"
)

// randomServer is the implementation of the RandomService
type randomServer struct {
	pb.UnimplementedRandomServiceServer
	rnd *rand.Rand
}

// GetRandomNumber generates a random number within the requested range
func (s *randomServer) GetRandomNumber(ctx context.Context, req *pb.NumberRequest) (*pb.NumberResponse, error) {
	min := req.GetMin()
	max := req.GetMax()

	if min > max {
		min, max = max, min // Swap if min > max
	}

	randomNum := min + int32(s.rnd.Intn(int(max-min+1)))
	
	log.Printf("GetRandomNumber request: min=%d, max=%d, result=%d", min, max, randomNum)
	
	return &pb.NumberResponse{
		Value: randomNum,
	}, nil
}

// GetRandomString generates a random string with the requested length and character set
func (s *randomServer) GetRandomString(ctx context.Context, req *pb.StringRequest) (*pb.StringResponse, error) {
	length := req.GetLength()
	includeDigits := req.GetIncludeDigits()
	includeSpecial := req.GetIncludeSpecial()

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
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[s.rnd.Intn(len(chars))]
	}

	randomStr := string(result)
	log.Printf("GetRandomString request: length=%d, includeDigits=%v, includeSpecial=%v, result=%s",
		length, includeDigits, includeSpecial, randomStr)

	return &pb.StringResponse{
		Value: randomStr,
	}, nil
}

func main() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using default or environment values")
	}

	// Set up a seed for the random generator
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))

	// Get host and port from environment variables
	host := getEnv("GRPC_HOST", defaultHost)
	port := getEnv("GRPC_PORT", defaultPort)
	addr := fmt.Sprintf("%s:%s", host, port)

	// Create a listener on the specified port
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create a new gRPC server
	grpcServer := grpc.NewServer()

	// Register our service
	pb.RegisterRandomServiceServer(grpcServer, &randomServer{rnd: rng})

	// Register reflection service on gRPC server
	reflection.Register(grpcServer)

	log.Printf("Starting gRPC server on %s", addr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
} 