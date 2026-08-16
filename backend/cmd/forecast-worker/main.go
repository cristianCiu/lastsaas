// forecast-worker is the durable v6 execution process. It claims one Mongo
// lease at a time and exits only on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"lastsaas/internal/config"
	"lastsaas/internal/db"
	"lastsaas/internal/forecast"
	"lastsaas/internal/syslog"
)

func main() {
	config.LoadEnvFile()
	cfg, err := config.Load(config.GetEnv())
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	database, err := db.NewMongoDB(cfg.Database.URI, cfg.Database.Name)
	if err != nil {
		slog.Error("failed to connect to MongoDB", "error", err)
		os.Exit(1)
	}
	defer database.Close(context.Background())
	logger := syslog.New(database, nil)
	owner, _ := os.Hostname()
	if owner == "" {
		owner = "forecast-worker"
	}
	owner += ":" + strconv.Itoa(os.Getpid())
	worker := &forecast.Worker{Jobs: forecast.NewJobStore(database, forecast.DefaultLeaseConfig()), Store: forecast.NewMongoStore(database), Log: logger, Owner: owner}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	interval := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := worker.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("forecast worker iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
