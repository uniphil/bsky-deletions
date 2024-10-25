package main

import (
	"embed"
	"encoding/json"
	"github.com/coder/websocket"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

//go:embed *.html
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "*.html"))

type IndexTemplateData struct {
	KnownLangs   []string
	BrowserLangs []*string
}

type Server struct {
	newClientFeed chan websocket.Conn
}

func (s Server) index(w http.ResponseWriter, r *http.Request) {
	log.Println("index hmmmm", r.URL)
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

func (s Server) wsConnect(w http.ResponseWriter, r *http.Request) {
	log.Println("ws connect hiiiii")
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println("failed to upgrade websocket connection", err)
		return
	}
	if c == nil {
		log.Println("oops, conn is nil?")
		return
	}
	s.newClientFeed <- *c
	c.Close(websocket.StatusNormalClosure, "blah")
}

func (s Server) todo(w http.ResponseWriter, r *http.Request) {
	log.Println("todo...")
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

func (s Server) broadcast(deletedFeed chan PersistedPost) {
	observers := map[*websocket.Conn]bool{}
	observersCountTicker := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-observersCountTicker.C:
			log.Println("tick: notify observers", len(observers))
		case post := <-deletedFeed:
			log.Println("deleted post", post)
			for _, c := range observers {
				log.Println("send to", c)
			}
		case conn := <-s.newClientFeed:
			log.Println("new client", conn)
			observers[&conn] = true
		}
	}
}

func Serve(env, port string, deletedFeed chan PersistedPost) {

	server := Server{
		newClientFeed: make(chan websocket.Conn),
	}

	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		log.Println("incoming...")
		if r.Header.Get("Upgrade") == "websocket" {
			log.Println("seems like a ws upgrade...")
			server.wsConnect(w, r)
		} else {
			log.Println("not a ws upgrade...")
			server.index(w, r)
		}
	})
	router.HandleFunc("GET /ready", server.todo)
	router.HandleFunc("GET /stats", server.todo)
	router.HandleFunc("GET /metrics", server.todo)
	router.HandleFunc("POST /oops", server.oops)

	go server.broadcast(deletedFeed)

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
