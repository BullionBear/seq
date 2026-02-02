package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/BullionBear/seq/actor"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/env"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/strategy"
	"github.com/BullionBear/seq/strategy/impl/obtest"
	"github.com/BullionBear/seq/strategy/impl/xarb"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("c", "", "Path to configuration file")
	strategyName := flag.String("s", "xarb", "Strategy to run (xarb, obtest)")
	flag.Parse()

	// Determine config path: flag takes precedence over environment variable
	if *configPath == "" {
		*configPath = os.Getenv("CONFIG")
	}

	// Exit if no config path provided
	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "Error: Configuration file path is required.\n")
		fmt.Fprintf(os.Stderr, "Usage: %s -c <config-file> [-s <strategy>] or set CONFIG environment variable\n", os.Args[0])
		os.Exit(1)
	}

	// Load configuration
	cfg, err := strategy.LoadConfig(*configPath)
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
	log.Info().Msgf("Strategy: %s", *strategyName)

	// Initialize Catelog service (InstrumentCatalog)
	catalogService := catalog.NewCatalog(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	if catalogService == nil {
		log.Error().Msg("Failed to initialize catalog service")
		os.Exit(1)
	}
	log.Info().Msg("Catalog service initialized successfully")

	// Select strategy based on flag
	var strategyImpl actor.Actor
	switch *strategyName {
	case "xarb":
		strategyImpl = xarb.NewXArb()
	case "obtest":
		strategyImpl = obtest.NewOBTest()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown strategy '%s'. Available strategies: xarb, obtest\n", *strategyName)
		os.Exit(1)
	}

	// Create context that cancels on SIGINT (Ctrl+C) or SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	engine := strategy.NewEngine(strategyImpl, catalogService)
	engine.Init(cfg)
	engine.Start()
	go engine.Run(ctx, cfg)

	// Wait for context cancellation (signal)
	<-ctx.Done()
	log.Info().Msg("Engine stopped")
}
