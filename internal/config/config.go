// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// Config содержит параметры, управляемые через среду, которые используются для запуска API и
// воркеров.
type Config struct {
	AppEnv             string
	APIPort            int
	WorkerPort         int
	WorkerID           string
	StoragePath        string
	FFmpegPath         string
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
}

// Load загружает параметры приложения из среды и поддерживаемого локального файла конфигурации.
//
// @returns:
//   - result1: Результирующий файл config.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func Load() (Config, error) {
	_ = godotenv.Load()
	if err := validateTypedEnvironment(); err != nil {
		return Config{}, err
	}
	cfg := Config{
		AppEnv:             env("APP_ENV", "local"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		TrustedProxies:     envList("HTTP_TRUSTED_PROXIES"),
		APIPort:            envInt("APP_PORT", 8085),
		WorkerPort:         envInt("WORKER_PORT", 8090),
		WorkerID:           env("WORKER_ID", "record-worker-local"),
		StoragePath:        env("STORAGE_PATH", "/storage"),
		FFmpegPath:         env("FFMPEG_PATH", "ffmpeg"),
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
	cfg.StageSeven, err = LoadStageSeven(cfg.IsLocal() || cfg.AppEnv == "test")
	if err != nil {
		return Config{}, err
	}
	if cfg.MinIOBucket == "" {
		return Config{}, fmt.Errorf("MINIO_BUCKET is required")
	}

	return cfg, nil
}

// RateLimitConfig содержит правила ограничения частоты запросов и настроенную политику
// обработки сбоев ограничителя.
type RateLimitConfig struct {
	DefaultRPM        int
	AuthLoginIPRPM    int
	AuthRegisterIPRPM int
}

// IsLocal сообщает, запущено ли приложение в явно локальной среде.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (c Config) IsLocal() bool {
	value := strings.ToLower(c.AppEnv)
	return value == "local" || value == "dev" || value == "debug"
}

// postgresDSN формирует строку подключения PostgreSQL на основе конфигурации, управляемой
// оператором.
//
// @returns:
//   - result1: Строка подключения PostgreSQL сформирована на основе доверенных настроек.
func postgresDSN() string {
	host := env("POSTGRES_HOST", "postgres")
	port := env("POSTGRES_PORT", "5432")
	db := env("POSTGRES_DB", "go_recorder")
	user := env("POSTGRES_USER", "go_recorder")
	password := env("POSTGRES_PASSWORD", "go_recorder_pass")
	sslmode := env("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC", host, port, user, password, db, sslmode)
}

// redisAddr формирует хост и порт Redis, используемые клиентом приложения.
//
// @returns:
//   - result1: Настроенный хост и порт Redis.
func redisAddr() string {
	host := env("REDIS_HOST", "redis")
	port := env("REDIS_PORT", "6379")

	return fmt.Sprintf("%s:%s", host, port)
}

// rabbitMQURL формирует соединение AMQP с URL на основе настроенных параметров брокера.
//
// @returns:
//   - result1: Настроенное соединение AMQP с URL.
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

// env считывает переменную окружения и использует резервное значение, если она пуста.
//
// @Parameters:
//   - key: хранилище, кэш или криптографический ключ, используемый в операции.
//   - fallback: значение, используемое при отсутствии или некорректности необязательного параметра.
//
// Возвращаемое значение:
//   - result1: настроенное значение из окружения или указанное резервное значение.
func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

// envInt считывает целочисленный параметр среды, возвращаясь к предыдущему значению, если его
// не удается проанализировать.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - fallback: Значение, используемое при отсутствии или некорректном значении необязательного
//     параметра.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
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

// envBool считывает логический параметр среды, возвращаясь к предыдущему значению, если его не
// удается проанализировать.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - fallback: Значение, используемое при отсутствии или некорректном значении необязательного
//     параметра.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// envDuration считывает параметр среды, определяющий продолжительность, и возвращается к
// предыдущему значению, если его не удается проанализировать.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - fallback: Значение, используемое при отсутствии или некорректном значении необязательного
//     параметра.
//
// @returns:
//   - result1: Продолжительность или ограниченный интервал ожидания, вычисленный в ходе
//     операции.
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

// envList разбивает параметр среды, разделенный запятыми, на непустые сокращенные записи.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Собранные строковые значения.
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
