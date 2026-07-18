package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/BullionBear/seq/core/catalog"
	coreconfig "github.com/BullionBear/seq/core/config"
	"github.com/BullionBear/seq/core/env"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/core/telemetry"
	"github.com/BullionBear/seq/core/tradingmode"
	"github.com/BullionBear/seq/node"

	// Register actor factories via init().
	_ "github.com/BullionBear/seq/data/actor/orderbook"
	_ "github.com/BullionBear/seq/execution/actor/oms"
	_ "github.com/BullionBear/seq/portfolio/actor/balance"
	_ "github.com/BullionBear/seq/risk/actor/ratelimiter"
	_ "github.com/BullionBear/seq/risk/actor/tpnl"
	_ "github.com/BullionBear/seq/strategy/actor/obtest"
	_ "github.com/BullionBear/seq/strategy/actor/xarb"
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

	// Resolve paper/live gate before any venue clients or engines start.
	mode, err := tradingmode.Resolve(cfg.TradingMode, os.Getenv)
	if err != nil {
		log.Error().Err(err).
			Str("config_trading_mode", cfg.TradingMode).
			Str("env_trading_mode", os.Getenv(tradingmode.EnvTradingMode)).
			Msg("Trading mode gate refused to start")
		os.Exit(1)
	}
	tradingmode.Set(mode)
	if mode.IsLive() {
		log.Warn().
			Str("trading_mode", mode.String()).
			Msg("TRADING MODE: LIVE — venue order submit/cancel enabled")
	} else {
		log.Info().
			Str("trading_mode", mode.String()).
			Msg("TRADING MODE: PAPER — venue order submit/cancel refused; set trading_mode=live to enable")
	}

	// Fence the Go runtime (P2-4): optional GOMAXPROCS cap, memory limit,
	// and GC-off (requires the memory limit as a fuse).
	if err := cfg.Runtime.Apply(); err != nil {
		log.Error().Err(err).Msg("Runtime fencing refused to start")
		os.Exit(1)
	}
	if cfg.Runtime.GCOff {
		log.Info().
			Int64("mem_limit_bytes", cfg.Runtime.MemLimitBytes).
			Msg("Runtime fencing: GC disabled, memory limit acts as fuse (see docs/DEPLOYMENT.md)")
	}

	// Initialize Catalog service from local instruments file and configured accounts
	catalogService, err := catalog.NewCatalog(cfg.Catalog)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize catalog service")
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

	// Start the metrics endpoint (P2-4): /metrics for runtime histograms and
	// overflow counters, POST /gc as the quiet-window collection hook.
	if srv, err := telemetry.StartMetricsServer(cfg.Metrics, n.MsgBus()); err != nil {
		log.Error().Err(err).Msg("Failed to start metrics server")
		os.Exit(1)
	} else if srv != nil {
		defer srv.Close()
		log.Info().Str("addr", srv.Addr).Msg("Metrics server enabled")
	}

	// Initialize, start, and run the node (Run blocks until graceful shutdown completes)
	n.Init(cfg.Node, cfg.ExecRouter, cfg.DataRouter, mode)
	n.Start(ctx)
	n.Run(ctx)
}
