package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/befship/tencent-cert-auto/internal/app"
)

func main() {
	once := flag.Bool("once", false, "run a single check/renew cycle and exit")
	force := flag.Bool("force", false, "force renew even if certificate is still valid")
	flag.Parse()

	app.SetupLogger()

	cfg := app.MustLoad()
	if *force {
		cfg.ForceRenew = true
	}

	svc, err := app.New(cfg)
	if err != nil {
		slog.Error("init failed", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if *once {
		if err := svc.RunOnce(ctx); err != nil {
			os.Exit(1)
		}
		return
	}

	if err := svc.Run(ctx); err != nil {
		slog.Error("run failed", "err", err)
		os.Exit(1)
	}
}
