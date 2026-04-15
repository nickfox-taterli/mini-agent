package main

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/db"
	"taterli-agent-chat/backend/internal/mcpserver"
	"taterli-agent-chat/backend/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logPath, err := initLogger()
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	manager, err := backend.NewManager(cfg)
	if err != nil {
		log.Fatalf("init backend manager: %v", err)
	}

	mcpserver.InitConfig(cfg.Server.FrontendURL)
	mcpserver.InitMiniMaxTools(cfg.MinimaxTools.APIKeys, cfg.MinimaxTools.APIHost)
	if err := mcpserver.InitDockerRuntime(mcpserver.DockerRuntimeConfigFromExternal(
		cfg.DockerRuntime.Enabled,
		cfg.DockerRuntime.Host,
		cfg.DockerRuntime.SessionTTLSeconds,
		cfg.DockerRuntime.MaxLifetimeSeconds,
		cfg.DockerRuntime.DefaultTimeoutSeconds,
		cfg.DockerRuntime.MaxTimeoutSeconds,
		cfg.DockerRuntime.MemoryLimit,
		cfg.DockerRuntime.CPULimit,
		cfg.DockerRuntime.PidsLimit,
		cfg.DockerRuntime.WorkspaceRoot,
	)); err != nil {
		log.Fatalf("init docker runtime: %v", err)
	}

	mcpserver.InitLibreOfficeConfig(
		cfg.LibreOffice.DockerImage,
		cfg.LibreOffice.DefaultTimeoutSeconds,
		cfg.LibreOffice.MaxTimeoutSeconds,
		cfg.LibreOffice.MemoryLimit,
		cfg.LibreOffice.CPULimit,
		cfg.LibreOffice.PidsLimit,
	)

	// 初始化 SQLite 数据库
	dbPath := filepath.Join("data", "chat.db")
	if err := db.Init(dbPath); err != nil {
		log.Fatalf("init database: %v", err)
	}
	log.Printf("database initialized: %s", dbPath)

	srv := server.New(manager, cfg.Server.Host, cfg.Server.Port, cfg.Server.FrontendURL, cfg.Auth.Enabled(), cfg.Auth.Password)
	log.Printf("backend listening on http://%s:%d, log_file=%s", cfg.Server.Host, cfg.Server.Port, logPath)
	if err := srv.Run(); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func initLogger() (string, error) {
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", err
	}
	logPath := filepath.Join(logDir, "backend.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return logPath, nil
}
