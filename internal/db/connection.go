package db

import (
	"database/sql"
	"fmt"
	"time"

	"scalable-paywall/internal/config"

	_ "github.com/lib/pq"
)

type Connection struct {
	*sql.DB
}

func NewConnection(cfg config.DatabaseConfig) (*Connection, error) {
	// Build connection string with proper handling of empty password
	var dsn string
	if cfg.Password == "" {
		dsn = fmt.Sprintf("host=%s port=%d user=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.Port, cfg.User, cfg.DBName, cfg.SSLMode)
	} else {
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Connection{DB: db}, nil
}

func (c *Connection) HealthCheck() error {
	return c.Ping()
}

func (c *Connection) Close() error {
	return c.DB.Close()
}
