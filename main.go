package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
)

//go:embed *.html
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "*.html"))

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

	deletedFeed := make(chan PersistedPost)

	Consume(ctx, env, dbPath, logger, deletedFeed)

	go func() {
		// for m := range deletedFeed {
		for _ = range deletedFeed {
			// log.Println("ayyy", m.Text)
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}

		t.ExecuteTemplate(w, "index.html", data)
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
