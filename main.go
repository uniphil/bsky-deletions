package main

import (
	"context"
	"log/slog"
	"os"
)

func main() {
	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./posts-cache.db"
	}

	ctx := context.TODO()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})))
	logger := slog.Default()

	deletedFeed, languagesFeed := Consume(ctx, env, dbPath, logger)
	topLangsFeed := CountLangs(languagesFeed)
	Serve(env, port, deletedFeed, topLangsFeed)
}
