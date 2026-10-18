package config

import (
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config contains the environment-controlled settings used to start the API and workers.
type Config struct {
	AppEnv             string
	APIPort            int
	WorkerPort         int
	WorkerInternalURL  string
	WorkerID           string
	StoragePath        string
	FFmpegPath         string
	WebRTCUDPPort      int
	WebRTCTCPPort      int
	WebRTCNATIPs       []string
	PostgresDSN        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RabbitMQURL        string
	RabbitMQExchange   string
	RabbitMQQueue      string
	RabbitMQRoutingKey string
	RecordLockTTL      time.Duration
	RateLimitEnabled   bool
	RateLimitWindow    time.Duration
	RateLimit          RateLimitConfig
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOBucket        string
	MinIOUseSSL        bool
	MinIOPublicOrigin  string
	JWTSecret          string
	TrustedProxies     []string
	Operations         OperationsConfig
	StageSeven         StageSevenConfig
	StageEight         StageEightConfig
}

// Load loads application settings from the environment and the supported local environment file.
//
// Returns:
//   - result 1: The resulting config.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func Load() (Config, error) {
	_ = godotenv.Load()
	if err := validateTypedEnvironment(); err != nil {
		return Config{}, err
	}
	webRTCUDPPort := envInt("WEBRTC_UDP_PORT", 50000)

	cfg := Config{
		AppEnv:             env("APP_ENV", "local"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		TrustedProxies:     envList("HTTP_TRUSTED_PROXIES"),
		APIPort:            envInt("APP_PORT", 8085),
		WorkerPort:         envInt("WORKER_PORT", 8090),
		WorkerInternalURL:  env("WORKER_INTERNAL_URL", "http://worker:8090"),
		WorkerID:           env("WORKER_ID", "recorder-worker-local"),
		StoragePath:        env("STORAGE_PATH", "/storage"),
		FFmpegPath:         env("FFMPEG_PATH", "ffmpeg"),
		WebRTCUDPPort:      webRTCUDPPort,
		WebRTCTCPPort:      envInt("WEBRTC_TCP_PORT", webRTCUDPPort),
		WebRTCNATIPs:       envList("WEBRTC_NAT_IPS"),
		RedisAddr:          redisAddr(),
		RedisPassword:      env("REDIS_PASSWORD", ""),
		RedisDB:            envInt("REDIS_DB", 0),
		RabbitMQURL:        rabbitMQURL(),
		RabbitMQExchange:   env("RABBIT_MQ_EXCHANGE", "go-recorder.commands"),
		RabbitMQQueue:      env("RABBIT_MQ_QUEUE", "go-recorder.recording.commands"),
		RabbitMQRoutingKey: env("RABBIT_MQ_ROUTING_KEY", "record.commands"),
		RecordLockTTL:      envDuration("RECORD_LOCK_TTL", 6*time.Hour),
		RateLimitEnabled:   envBool("RATE_LIMIT_ENABLED", true),
		RateLimitWindow:    envDuration("RATE_LIMIT_WINDOW", time.Minute),
		RateLimit: RateLimitConfig{
			DefaultRPM:        envInt("RATE_LIMIT_DEFAULT_RPM", 240),
			AuthLoginIPRPM:    envInt("RATE_LIMIT_AUTH_LOGIN_IP_RPM", 10),
			AuthRegisterIPRPM: envInt("RATE_LIMIT_AUTH_REGISTER_IP_RPM", 5),
		},
		MinIOEndpoint:     env("MINIO_ENDPOINT", "minio:9000"),
		MinIOAccessKey:    env("MINIO_ROOT_USER", "go_recorder"),
		MinIOSecretKey:    env("MINIO_ROOT_PASSWORD", "go_recorder_pass"),
		MinIOBucket:       env("MINIO_BUCKET", "recordings"),
		MinIOUseSSL:       envBool("MINIO_USE_SSL", false),
		MinIOPublicOrigin: env("MINIO_PUBLIC_ENDPOINT", "localhost:9000"),
	}
	cfg.PostgresDSN = postgresDSN()
	var err error
	cfg.Operations, err = loadOperations(cfg.IsLocal() || cfg.AppEnv == "test")
	if err != nil {
		return Config{}, err
	}
	if err := validateProduction(cfg); err != nil {
		return Config{}, err
	}
	cfg.StageEight, err = LoadStageEight(cfg.IsLocal() || cfg.AppEnv == "test")
	if err != nil {
		return Config{}, err
	}
	cfg.StageSeven, err = LoadStageSeven(cfg.IsLocal() || cfg.AppEnv == "test")
	if err != nil {
		return Config{}, err
	}
	if cfg.MinIOBucket == "" {
		return Config{}, fmt.Errorf("MINIO_BUCKET is required")
	}

	return cfg, nil
}

// RateLimitConfig contains request-rate rules and the configured limiter failure policy.
type RateLimitConfig struct {
	DefaultRPM        int
	AuthLoginIPRPM    int
	AuthRegisterIPRPM int
}

// IsLocal reports whether the application is running in an explicitly local environment.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (c Config) IsLocal() bool {
	value := strings.ToLower(c.AppEnv)
	return value == "local" || value == "dev" || value == "debug"
}

// postgresDSN builds the PostgreSQL connection string from operator-controlled configuration.
//
// Returns:
//   - result 1: PostgreSQL connection string built from trusted settings.
func postgresDSN() string {
	host := env("POSTGRES_HOST", "postgres")
	port := env("POSTGRES_PORT", "5432")
	db := env("POSTGRES_DB", "go_recorder")
	user := env("POSTGRES_USER", "go_recorder")
	password := env("POSTGRES_PASSWORD", "go_recorder_pass")
	sslmode := env("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC", host, port, user, password, db, sslmode)
}

// redisAddr builds the Redis host and port used by the application client.
//
// Returns:
//   - result 1: Configured Redis host and port.
func redisAddr() string {
	host := env("REDIS_HOST", "redis")
	port := env("REDIS_PORT", "6379")

	return fmt.Sprintf("%s:%s", host, port)
}

// rabbitMQURL builds the AMQP connection URL from the configured broker settings.
//
// Returns:
//   - result 1: Configured AMQP connection URL.
func rabbitMQURL() string {
	if dsn := env("RABBIT_MQ_DSN", ""); dsn != "" {
		return dsn
	}
	user := env("RABBIT_MQ_USER", "go_recorder")
	password := env("RABBIT_MQ_PASSWORD", "go_recorder_pass")
	host := env("RABBIT_MQ_HOST", "rabbitmq")
	port := env("RABBIT_MQ_PORT", "5672")
	vhost := env("RABBIT_MQ_VHOST", "/")
	dsn := neturl.URL{
		Scheme:  "amqp",
		User:    neturl.UserPassword(user, password),
		Host:    net.JoinHostPort(host, port),
		Path:    "/" + vhost,
		RawPath: "/" + neturl.PathEscape(vhost),
	}

	return dsn.String()
}

// env reads an environment variable and uses the fallback when it is empty.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//   - fallback: Value used when the optional setting is absent or invalid.
//
// Returns:
//   - result 1: Configured environment value or the supplied fallback.
func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

// envInt reads an integer environment setting, falling back when it cannot be parsed.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//   - fallback: Value used when the optional setting is absent or invalid.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

// envBool reads a boolean environment setting, falling back when it cannot be parsed.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//   - fallback: Value used when the optional setting is absent or invalid.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// envDuration reads a duration environment setting, falling back when it cannot be parsed.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//   - fallback: Value used when the optional setting is absent or invalid.
//
// Returns:
//   - result 1: The duration or bounded waiting interval computed by the operation.
func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return time.Duration(seconds) * time.Second
}

// envList splits a comma-separated environment setting into nonempty trimmed entries.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: The collected string values.
func envList(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}
