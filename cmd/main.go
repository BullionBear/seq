package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/BullionBear/seq/env"
	"github.com/BullionBear/seq/internal/config"
	"github.com/BullionBear/seq/internal/db"
	pms "github.com/BullionBear/seq/internal/srv/catalog"
	"github.com/BullionBear/seq/pkg/logger"
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

	// Initialize logger from configuration
	loggerOpts := logger.Options{
		Level:          cfg.Logger.Level,
		Output:         cfg.Logger.Output,
		Path:           cfg.Logger.Path,
		MaxByteSize:    cfg.Logger.MaxByteSize,
		MaxBackupFiles: cfg.Logger.MaxBackupFiles,
	}
	if err := logger.Init(loggerOpts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Get the singleton logger
	log := logger.Get()
	log.Info().Msg("Starting Seq...")
	log.Info().Msg("Version: " + env.Version)
	log.Info().Msg("Build Time: " + env.BuildTime)
	log.Info().Msg("Commit Hash: " + env.CommitHash)
	log.Info().Msgf("Configuration loaded from: %s", *configPath)

	// Initialize PostgreSQL database connection
	db, err := db.ConnectPostgres(cfg.PMS.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL database")
	}

	// Initialize PMS service (InstrumentCatalog)
	pmsService, err := pms.NewInstrumentCatalog(db)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize PMS service")
	}
	_ = pmsService // TODO: Use PMS service as needed

	log.Info().Msg("PMS service initialized successfully")
}
