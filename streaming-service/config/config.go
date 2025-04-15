package config

import (
	"os"
	"strconv"
	"time"
)

// Config represents the configuration for the streaming service
type Config struct {
	// Server configuration
	ServerHost string
	ServerPort string

	// gRPC client configuration
	GeneratorHost string
	GeneratorPort string

	// Connection settings
	ReconnectWait time.Duration
	MaxRetries    int

	// Stream settings
	DefaultFrequency int32
	DefaultMinValue  int32
	DefaultMaxValue  int32
	BufferSize       int

	// SSE settings
	SSEKeepAliveInterval time.Duration
}

// LoadConfig loads the configuration from environment variables
func LoadConfig() *Config {
	return &Config{
		// Server configuration
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "8085"),

		// gRPC client configuration
		GeneratorHost: getEnv("GENERATOR_HOST", "localhost"),
		GeneratorPort: getEnv("GENERATOR_PORT", "50051"),

		// Connection settings
		ReconnectWait: getDurationEnv("RECONNECT_WAIT", 5*time.Second),
		MaxRetries:    getIntEnv("MAX_RETRIES", 10),

		// Stream settings
		DefaultFrequency: int32(getIntEnv("DEFAULT_FREQUENCY_MS", 1000)),
		DefaultMinValue:  int32(getIntEnv("DEFAULT_MIN_VALUE", 0)),
		DefaultMaxValue:  int32(getIntEnv("DEFAULT_MAX_VALUE", 100)),
		BufferSize:       getIntEnv("BUFFER_SIZE", 100),

		// SSE settings
		SSEKeepAliveInterval: getDurationEnv("SSE_KEEPALIVE_INTERVAL", 15*time.Second),
	}
}

// GeneratorAddr returns the full address of the generator service
func (c *Config) GeneratorAddr() string {
	return c.GeneratorHost + ":" + c.GeneratorPort
}

// ServerAddr returns the full address of the streaming server
func (c *Config) ServerAddr() string {
	return c.ServerHost + ":" + c.ServerPort
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getIntEnv returns the value of an environment variable as an integer or a default value
func getIntEnv(key string, defaultValue int) int {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

// getDurationEnv returns the value of an environment variable as a duration or a default value
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultValue
	}
	val, err := time.ParseDuration(valStr)
	if err != nil {
		return defaultValue
	}
	return val
} 