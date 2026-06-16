package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilledipa/praetor/agent/app"
	"github.com/guilledipa/praetor/agent/pki"
	"github.com/spf13/viper"

	"github.com/guilledipa/praetor/pkg/telemetry"
	"github.com/guilledipa/praetor/agent/plugin"
	_ "github.com/guilledipa/praetor/agent/facts/core"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Enable dry-run mode (simulate changes without applying them)")
	flag.Parse()

	viper.SetEnvPrefix("PRAETOR_AGENT")
	viper.AutomaticEnv()
	viper.SetConfigFile("/etc/praetor/agent.yaml")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("No config file found or error reading it: %v - falling back to env vars", err)
	}

	var cfg app.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}

	if *dryRun {
		cfg.DryRun = true
	}

	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})).With("node_id", cfg.NodeID)
	slog.SetDefault(logger)

	logger.Info("Agent starting...")

	tp, err := telemetry.InitProvider(context.Background(), logger, "praetor-agent")
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry", "error", err)
	} else if tp != nil {
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				logger.Error("Error shutting down tracer provider", "error", err)
			}
		}()
	}

	if err := plugin.InitPlugins(logger); err != nil {
		logger.Error("Failed to initialize RPC plugins", "error", err)
	}
	defer plugin.Cleanup()

	bootstrapCfg := pki.BootstrapConfig{
		ClientCertPath:    cfg.NatsClientCert,
		ClientKeyPath:     cfg.NatsClientKey,
		MasterRootCA:      cfg.MasterRootCA,
		MasterGRPCAddress: cfg.MasterGRPCAddress,
		NodeID:            cfg.NodeID,
		BootstrapToken:    cfg.AgentBootstrapToken,
		Logger:            logger,
	}

	if err := pki.RunBootstrapWorkflow(bootstrapCfg); err != nil {
		logger.Error("Bootstrap enrollment failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	agentApp := app.NewAgent(cfg, logger)
	if err := agentApp.Run(ctx); err != nil {
		logger.Error("Agent failed to run", "error", err)
		os.Exit(1)
	}
}
