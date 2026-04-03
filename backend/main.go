package main

import (
	"flag"
	"log"

	"taterli-agent-chat/backend/internal/backend"
	"taterli-agent-chat/backend/internal/config"
	"taterli-agent-chat/backend/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	manager, err := backend.NewManager(cfg)
	if err != nil {
		log.Fatalf("init backend manager: %v", err)
	}

	srv := server.New(manager, cfg.Server.Host, cfg.Server.Port)
	log.Printf("backend listening on http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := srv.Run(); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
