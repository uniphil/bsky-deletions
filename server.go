package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
)

// "github.com/gorilla/websocket"

//go:embed *.html
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "*.html"))

type IndexTemplateData struct {
	KnownLangs   []string
	BrowserLangs []*string
}

type Server struct {
}

func (s Server) hello(w http.ResponseWriter, r *http.Request) {

	seenLangs := map[string]bool{}
	langs := []*string{}
	acceptHeaderValue := r.Header.Get("Accept-Language")
	for _, lang := range strings.Split(acceptHeaderValue, ",") {
		var l = strings.TrimSpace(lang)
		l, _, _ = strings.Cut(l, ";")
		l, _, _ = strings.Cut(l, "-")
		if !seenLangs[l] {
			langs = append(langs, &l)
			seenLangs[l] = true
		}
	}

	w.Header().Add("Content-Type", "text/html")
	w.Header().Add("Cache-Control", "public, max-age=300, immutable")
	w.Header().Add("Vary", "accept-language")

	t.ExecuteTemplate(w, "index.html", IndexTemplateData{
		KnownLangs:   []string{"en", "b", "c"},
		BrowserLangs: langs, //[]*string{&bleh, nil},
	})
}

func (s Server) todo(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	io.WriteString(w, "todo")
}

func (s Server) oops(w http.ResponseWriter, r *http.Request) {
	errInfo := map[string]interface{}{}
	errInfo["ua"] = r.UserAgent()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(&errInfo)
	if err != nil {
		log.Println("failed to decode json during error report, trying to continue..")
	}
	jsonData, err := json.Marshal(&errInfo)
	log.Println("client error report", string(jsonData))
	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, "got it. and sorry :/")
}

func Serve(env, port string, deletedFeed chan PersistedPost) {

	server := Server{}

	router := http.NewServeMux()
	router.HandleFunc("GET /", server.hello)
	router.HandleFunc("GET /ready", server.todo)
	router.HandleFunc("GET /stats", server.todo)
	router.HandleFunc("GET /metrics", server.todo)
	router.HandleFunc("POST /oops", server.oops)

	go func() {
		// for m := range deletedFeed {
		for range deletedFeed {
			// log.Println("ayyy", m.Text)
		}
	}()

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
