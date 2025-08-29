package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv       string
	HTTPAddr     string
	DatabaseURL  string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	TLSCertFile  string
	TLSKeyFile   string
	DebugAddr    string

	KafkaBrokers     string
	KafkaTopicOrders string
	KafkaTopicDLQ    string
	OutboxInterval   time.Duration
	OutboxBatch      int

	RateLimitRPS   float64
	RateLimitBurst int

	OIDCIssuer        string
	OIDCAudience      string
	OIDCRequiredScope string
	AuthEnabled       bool
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func mustDur(val string, def time.Duration) time.Duration {
	if val == "" {
		return def
	}

	i, err := time.ParseDuration(val)
	if err != nil {
		log.Panicf("invalid duration %q: %v", val, err)
		return def
	}

	return i
}

func mustInt(val string, def int) int {
	if val == "" {
		return def
	}

	i, err := strconv.Atoi(val)
	if err != nil {
		log.Panicf("invalid integer %q: %v", val, err)
		return def
	}

	return i
}

func Load() *Config {
	return &Config{
		AppEnv:       getEnv("APP_ENV", "local"),
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://app:app@localhost:5432/orders?sslmode=disable"),
		ReadTimeout:  mustDur(os.Getenv("READ_TIMEOUT"), 5*time.Second),
		WriteTimeout: mustDur(os.Getenv("WRITE_TIMEOUT"), 10*time.Second),
		IdleTimeout:  mustDur(os.Getenv("IDLE_TIMEOUT"), 60*time.Second),
		TLSCertFile:  getEnv("TLS_CERT", ""),
		TLSKeyFile:   getEnv("TLS_KEY", ""),
		DebugAddr:    getEnv("DEBUG_ADDR", ":9090"),

		KafkaBrokers:     getEnv("KAFKA_BROKERS", "localhost:19092"),
		KafkaTopicOrders: getEnv("KAFKA_TOPIC_ORDERS", "orders"),
		KafkaTopicDLQ:    getEnv("KAFKA_TOPIC_DLQ", "orders.dlq"),
		OutboxInterval:   mustDur(getEnv("OUTBOX_RELAY_INTERVAL", "2s"), 2*time.Second),
		OutboxBatch:      mustInt(getEnv("OUTBOX_RELAY_BATCH", "200"), 200),

		RateLimitRPS:   float64(mustInt(getEnv("RATE_LIMIT_RPS", "10"), 10)),
		RateLimitBurst: mustInt(getEnv("RATE_LIMIT_BURST", "20"), 20),

		OIDCIssuer:        getEnv("OIDC_ISSUER", ""),
		OIDCAudience:      getEnv("OIDC_AUDIENCE", ""),
		OIDCRequiredScope: getEnv("OIDC_REQUIRED_SCOPE", ""),
		AuthEnabled:       getEnv("OIDC_ISSUER", "") != "",
	}
}
