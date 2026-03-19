package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/guilledipa/praetor/agent/app"
	"github.com/guilledipa/praetor/agent/pki"
	"github.com/kelseyhightower/envconfig"

	_ "github.com/guilledipa/praetor/agent/facts/core"
	_ "github.com/guilledipa/praetor/agent/resources/exec"
	_ "github.com/guilledipa/praetor/agent/resources/file"
	_ "github.com/guilledipa/praetor/agent/resources/pkg"
	_ "github.com/guilledipa/praetor/agent/resources/svc"
)

func main() {
	var cfg app.Config
	if err := envconfig.Process("AGENT", &cfg); err != nil {
		log.Fatalf("Failed to process config: %v", err)
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

	agentApp := app.NewAgent(cfg, logger)
	if err := agentApp.Run(); err != nil {
		logger.Error("Agent failed to run", "error", err)
		os.Exit(1)
	}
}
