package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Cache     CacheConfig     `mapstructure:"cache"`
	Telemetry TelemetryConfig `mapstructure:"telemetry"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Payment   PaymentConfig   `mapstructure:"payment"`
}

type ServerConfig struct {
	Port         int    `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"dbname"`
	SSLMode         string `mapstructure:"sslmode"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

type CacheConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type TelemetryConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ServiceName string `mapstructure:"service_name"`
	Environment string `mapstructure:"environment"`
	Version     string `mapstructure:"version"`
}

type RateLimitConfig struct {
	Enabled     bool  `mapstructure:"enabled"`
	RequestsPer int   `mapstructure:"requests_per"`
	Window      int64 `mapstructure:"window"`
}

type PaymentConfig struct {
	GatewayURL     string               `mapstructure:"gateway_url"`
	APIKey         string               `mapstructure:"api_key"`
	SecretKey      string               `mapstructure:"secret_key"`
	WebhookSecret  string               `mapstructure:"webhook_secret"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

type CircuitBreakerConfig struct {
	Enabled          bool  `mapstructure:"enabled"`
	FailureThreshold int   `mapstructure:"failure_threshold"`
	RecoveryTimeout  int64 `mapstructure:"recovery_timeout"`
	HalfOpenRequests int   `mapstructure:"half_open_requests"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")

	// Set defaults
	setDefaults()

	// Read environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.read_timeout", 15)
	viper.SetDefault("server.write_timeout", 15)

	// Database defaults
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.dbname", "paywall")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_open_conns", 25)
	viper.SetDefault("database.max_idle_conns", 5)
	viper.SetDefault("database.conn_max_lifetime", 300)

	// Cache defaults
	viper.SetDefault("cache.host", "localhost")
	viper.SetDefault("cache.port", 6379)
	viper.SetDefault("cache.db", 0)
	viper.SetDefault("cache.pool_size", 10)

	// Telemetry defaults
	viper.SetDefault("telemetry.enabled", true)
	viper.SetDefault("telemetry.service_name", "scalable-paywall")
	viper.SetDefault("telemetry.environment", "development")
	viper.SetDefault("telemetry.version", "1.0.0")

	// Rate limiting defaults
	viper.SetDefault("rate_limit.enabled", true)
	viper.SetDefault("rate_limit.requests_per", 100)
	viper.SetDefault("rate_limit.window", 60)

	// Payment gateway defaults
	viper.SetDefault("payment.circuit_breaker.enabled", true)
	viper.SetDefault("payment.circuit_breaker.failure_threshold", 5)
	viper.SetDefault("payment.circuit_breaker.recovery_timeout", 60)
	viper.SetDefault("payment.circuit_breaker.half_open_requests", 3)
}
