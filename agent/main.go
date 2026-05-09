package main

import (
	"log/slog"
	"os"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("agent started",
		"poll_interval_sec", cfg.PollIntervalSec,
		"max_iteration", cfg.MaxIteration,
	)

	// TODO: ポーリングループ（後続機能で実装）
}
