package main

import (
	"embed"
	"encoding/json"
	"github.com/gorilla/websocket"
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  512,
	WriteBufferSize: 1024,
}

type IndexTemplateData struct {
	KnownLangs   []string
	BrowserLangs []*string
}

type Server struct {
	newObserver chan chan PersistedPost
	knownLangs  *[]string
}

type PostMessageValue struct {
	Text   string          `json:"text"`
	Target *PostTargetType `json:"target"`
}

type PostMessagePost struct {
	Value PostMessageValue `json:"value"`
	Age   int64            `json:"age"`
}

type PostMessage struct {
	Type string          `json:"type"`
	Post PostMessagePost `json:"post"`
}

type ObserversMessage struct {
	Type      string `json:"type"`
	Observers int    `json:"observers"`
}

func (p *PersistedPost) toJson(t time.Time) ([]byte, error) {
	age := (t.UnixMicro() - p.TimeUS) / 1000
	message := PostMessage{
		Type: "post",
		Post: PostMessagePost{
			Age: age,
			Value: PostMessageValue{
				Text:   p.Text,
				Target: p.Target,
			},
		},
	}
	data, err := json.Marshal(&message)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
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
		KnownLangs:   *s.knownLangs,
		BrowserLangs: langs,
	})
}

func (s *Server) wsConnect(w http.ResponseWriter, r *http.Request) {
	log.Println("ws connect hiiiii")
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("failed to upgrade websocket connection", err)
		return
	}

	receiver := make(chan PersistedPost, 5)
	pickLangs := make(chan []*string)
	s.newObserver <- receiver
	go listen(*c, pickLangs)
	go notify(*c, receiver, pickLangs)

	if err := r.ParseForm(); err != nil {
		log.Println("failed to get languages from websocket init. client will receive all.", err)
		return
	}

	var initialLangs = []*string{}
	for _, lang := range r.Form["lang"] {
		if lang == "null" {
			initialLangs = append(initialLangs, nil)
		} else {
			initialLangs = append(initialLangs, &lang)
		}
	}
	pickLangs <- initialLangs
}

func listen(c websocket.Conn, pickLangs chan<- []*string) {
	defer c.Close()
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		newLangs := struct {
			Type  string    `json:"type"`
			Langs []*string `json:"langs"`
		}{}
		err = json.Unmarshal(message, &newLangs)
		if err != nil {
			log.Println("failed to decode client message", message)
			continue
		}
		if newLangs.Type != "setLangs" {
			log.Println("unexpected client message type, ignoring", newLangs.Type)
			continue
		}
		pickLangs <- newLangs.Langs
	}
}

func notify(c websocket.Conn, receiver chan PersistedPost, pickLangs chan []*string) {
	defer func() {
		c.Close()
		close(receiver)
	}()
	var listenerLangs = map[string]bool{}
	var wantsUnknownLangs = false
	for {
		select {
		case post := <-receiver:
			if !ListeningFor(listenerLangs, wantsUnknownLangs, post.Langs) {
				continue
			}
			data, err := post.toJson(time.Now())
			if err != nil {
				log.Println("could not serialize post", post)
				continue
			}
			w, err := c.NextWriter(websocket.TextMessage)
			if err != nil {
				break
			}
			w.Write(data)
			if err := w.Close(); err != nil {
				break
			}
		case newLangs := <-pickLangs:
			listenerLangs = map[string]bool{}
			wantsUnknownLangs = false
			for _, lang := range newLangs {
				if lang == nil {
					wantsUnknownLangs = true
				} else {
					listenerLangs[*lang] = true
				}
			}
		default:
			break
		}
	}
}

func (s *Server) todo(w http.ResponseWriter, r *http.Request) {
	log.Println("todo...")
	w.WriteHeader(200)
	io.WriteString(w, "todo")
}

func (s *Server) oops(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) broadcast(deletedFeed <-chan PersistedPost, knownLangsFeed <-chan []string) {
	observers := make(map[chan PersistedPost]bool)
	observersCountTicker := time.NewTicker(3 * time.Second)
	for {
		select {
		case <-observersCountTicker.C:
			log.Println("tick: notify observers", len(observers))
		case post := <-deletedFeed:
			log.Println("deleted post", post)
			var toDelete = []chan PersistedPost{}
			for c := range observers {
				select {
				case c <- post:
				default:
					log.Println("should drop client", c)
					toDelete = append(toDelete, c)
				}
			}
			for _, c := range toDelete {
				delete(observers, c)
			}
		case newSeenLangs := <-knownLangsFeed:
			s.knownLangs = &newSeenLangs
		case c := <-s.newObserver:
			observers[c] = true
		}
	}
}

func Serve(env, port string, deletedFeed <-chan PersistedPost, topLangsFeed <-chan []string) {

	server := Server{
		newObserver: make(chan chan PersistedPost),
		knownLangs:  &[]string{"pt", "en", "ja"},
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

	go server.broadcast(deletedFeed, topLangsFeed)

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
