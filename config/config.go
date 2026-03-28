package config

import (
	"os"
	"time"
)

type Config struct {
	DatabaseURL    string
	RedisURL       string
	KMSKeyARN      string
	KMSEndpoint    string
	AWSRegion      string
	HMACKey        string
	CVVTTL         time.Duration
	PortTokenizer  string
	PortProxy      string
	LogLevel       string
	LogFormat      string
}

func Load() (*Config, error) {
	cvvTTL, err := time.ParseDuration(getEnv("CVV_TTL", "1h"))
	if err != nil {
		return nil, err
	}

	return &Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://vault:vault@localhost:5432/vault?sslmode=disable"),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379/0"),
		KMSKeyARN:      getEnv("KMS_KEY_ARN", ""),
		KMSEndpoint:    getEnv("KMS_ENDPOINT", "http://localhost:4566"),
		AWSRegion:      getEnv("AWS_REGION", "us-east-1"),
		HMACKey:        getEnv("HMAC_KEY", ""),
		CVVTTL:         cvvTTL,
		PortTokenizer:  getEnv("PORT_TOKENIZER", "8080"),
		PortProxy:      getEnv("PORT_PROXY", "8081"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		LogFormat:      getEnv("LOG_FORMAT", "json"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
