package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/guilledipa/praetor/agent/app"
	"github.com/guilledipa/praetor/agent/pki"
	"github.com/spf13/viper"

	"github.com/guilledipa/praetor/agent/plugin"
	_ "github.com/guilledipa/praetor/agent/facts/core"
)

func main() {
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

	agentApp := app.NewAgent(cfg, logger)
	if err := agentApp.Run(); err != nil {
		logger.Error("Agent failed to run", "error", err)
		os.Exit(1)
	}
}
