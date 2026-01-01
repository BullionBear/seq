package database

import (
	"fmt"

	"github.com/BullionBear/seq/internal/config"
	"github.com/BullionBear/seq/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ConnectPostgres initializes a PostgreSQL database connection using GORM
func ConnectPostgres(cfg config.ConfigDatabase) (*gorm.DB, error) {
	// Set default SSL mode if not specified
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	// Set default port if not specified
	port := cfg.Port
	if port == 0 {
		port = 5432
	}

	// Build DSN (Data Source Name)
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s",
		cfg.Host,
		cfg.User,
		cfg.Password,
		cfg.DBName,
		port,
		sslMode,
	)

	// Open database connection
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	log := logger.Get()
	log.Info().
		Str("host", cfg.Host).
		Int("port", port).
		Str("database", cfg.DBName).
		Msg("Successfully connected to PostgreSQL database")

	return db, nil
}
