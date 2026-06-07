package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"newapi-price-monitor/internal/app"
)

func main() {
	cfg := app.Config{
		Addr:            env("ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/newapi_price_monitor?sslmode=disable"),
		BasicAuthUser:   env("BASIC_AUTH_USER", ""),
		BasicAuthPass:   env("BASIC_AUTH_PASS", ""),
		SessionSecret:   env("SESSION_SECRET", ""),
		MonitorInterval: envDuration("MONITOR_INTERVAL", time.Minute),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	server, err := app.NewServer(ctx, cfg)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}
	defer server.Close()

	log.Printf("newapi price monitor listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, server.Routes()); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("invalid %s=%q, using %s", key, value, fallback)
		return fallback
	}
	return parsed
}
