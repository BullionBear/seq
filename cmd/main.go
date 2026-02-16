package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/catalog"
	coreconfig "github.com/BullionBear/seq/core/config"
	"github.com/BullionBear/seq/core/env"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/node"
	"github.com/BullionBear/seq/strategy/actor/obtest"
	"github.com/BullionBear/seq/strategy/actor/xarb"
)

// strategyFactory maps strategy type names to their constructors.
var strategyFactory = map[string]func(*catalog.Catalog, *msgbus.MsgBus) actor.Actor{
	"xarb": func(cat *catalog.Catalog, bus *msgbus.MsgBus) actor.Actor {
		return xarb.NewXArb(cat, bus)
	},
	"obtest": func(cat *catalog.Catalog, bus *msgbus.MsgBus) actor.Actor {
		return obtest.NewOBTest(cat, bus)
	},
}

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
	cfg, err := coreconfig.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load configuration from %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	// Initialize logger from configuration
	if err := logger.Init(cfg.Logger.ToOptions()); err != nil {
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

	// Initialize Catalog service
	catalogService := catalog.NewCatalog(cfg.Catalog.BaseURL, cfg.Catalog.APIToken)
	if catalogService == nil {
		log.Error().Msg("Failed to initialize catalog service")
		os.Exit(1)
	}
	log.Info().Msg("Catalog service initialized successfully")

	// Create context that cancels on SIGINT (Ctrl+C) or SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create the Node
	n := node.NewNode(catalogService)

	// Set up binary message logger if configured
	if cfg.MsgBus.MsgLog.Enabled && cfg.MsgBus.MsgLog.Dir != "" {
		msgLogger, err := msgbus.NewMsgLogger(cfg.MsgBus.MsgLog.Dir)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize message logger")
			os.Exit(1)
		}
		defer msgLogger.Close()
		n.MsgBus().SetMsgLogger(msgLogger)
		log.Info().Str("dir", cfg.MsgBus.MsgLog.Dir).Msg("MsgLogger enabled")
	}

	// Build strategy actors from config entries
	strategyActors := make([]actor.Actor, 0, len(cfg.Node.Engine.Strategy))
	for _, entry := range cfg.Node.Engine.Strategy {
		factory, ok := strategyFactory[entry.Type]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: Unknown strategy type '%s'. Available: xarb, obtest\n", entry.Type)
			os.Exit(1)
		}
		sa := factory(catalogService, n.MsgBus())
		strategyActors = append(strategyActors, sa)
		log.Info().Str("type", entry.Type).Msg("Strategy actor created")
	}

	n.Init(cfg.Node, strategyActors)
	n.Start(ctx)
	go n.Run(ctx)

	// Wait for context cancellation (signal)
	<-ctx.Done()
	log.Info().Msg("Node stopped")
}
