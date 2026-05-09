package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("agent started",
		"poll_interval_sec", cfg.PollIntervalSec,
		"max_iteration", cfg.MaxIteration,
	)

	// TODO: ポーリングループ（後続機能で実装）
}
