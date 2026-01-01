package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BullionBear/seq/internal/config"
	"github.com/BullionBear/seq/pkg/logger"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("c", "", "Path to configuration file")
	flag.Parse()

	// Determine config path: flag takes precedence over environment variable
	if *configPath == "" {
		*configPath = os.Getenv("CONFIG")
	}

	// Exit if no config path provided
	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "Error: Configuration file path is required.\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -c <config-file> or set CONFIG environment variable\n", os.Args[0])
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load configuration from %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	// Initialize logger (minimal for migrations)
	loggerOpts := logger.Options{
		Level:  "info",
		Output: "stdout",
	}
	if err := logger.Init(loggerOpts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	log := logger.Get()

	// Build database connection string
	dbCfg := cfg.PMS.Database
	sslMode := dbCfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	port := dbCfg.Port
	if port == 0 {
		port = 5432
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		dbCfg.User,
		dbCfg.Password,
		dbCfg.Host,
		port,
		dbCfg.DBName,
		sslMode,
	)

	// Get migrations directory (relative to project root)
	// Get absolute path to migrations directory
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get current working directory")
	}
	migrationsPath := filepath.Join(workDir, "migrations")
	migrationsDir := "file://" + migrationsPath

	log.Info().Str("dsn", fmt.Sprintf("postgres://%s@%s:%d/%s", dbCfg.User, dbCfg.Host, port, dbCfg.DBName)).
		Str("migrations", migrationsDir).
		Msg("Running database migrations...")

	// Open database connection
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open database connection")
	}
	defer db.Close()

	// Create postgres driver instance
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create postgres driver")
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(migrationsDir, dbCfg.DBName, driver)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create migrate instance")
	}
	defer m.Close()

	// Run migrations
	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			log.Info().Msg("No migrations to apply - database is up to date")
			return
		}
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	log.Info().Msg("Migrations completed successfully")
}
