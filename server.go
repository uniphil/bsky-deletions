package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
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
	newObserver chan chan ObserverMessage
	langsLock   sync.Mutex
	knownLangs  *[]string
}

type PostMessageValue struct {
	Text   string          `json:"text"`
	Target *PostTargetType `json:"target"`
}

type PostMessagePost struct {
	Value PostMessageValue `json:"value"`
	Age   int64            `json:"age"`
	Likes *uint32          `json:"likes"`
}

type PostMessage struct {
	Type string          `json:"type"`
	Post PostMessagePost `json:"post"`
}

type ObserversMessage struct {
	Type      string `json:"type"`
	Observers int    `json:"observers"`
}

type ObserverMessageType string

const (
	ObserverMessageTypePost      ObserverMessageType = "post"
	ObserverMessageTypeObservers ObserverMessageType = "observers"
)

type ObserverMessage struct {
	Type           ObserverMessageType `json:"type"`
	ObserversCount int                 `json:"observers"`
	Post           *LikedPersistedPost `json:"post"`
}

func (om *ObserverMessage) toJson(t time.Time) ([]byte, error) {
	switch om.Type {
	case ObserverMessageTypePost:
		return json.Marshal(PostMessage{
			Type: "post",
			Post: PostMessagePost{
				Age:   om.Post.Post.AgeMs(t),
				Likes: om.Post.Likes,
				Value: PostMessageValue{
					Text:   om.Post.Post.Text,
					Target: om.Post.Post.Target,
				},
			},
		})
	case ObserverMessageTypeObservers:
		return json.Marshal(ObserversMessage{
			Type:      "observers",
			Observers: om.ObserversCount,
		})
	default:
		return nil, fmt.Errorf("unhandled message type %s", om.Type)
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	log.Println("hello from", r.Header.Get("Referer"))
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
		KnownLangs:   s.getKnownLangs(),
		BrowserLangs: langs,
	})
}

func (s *Server) wsConnect(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("failed to upgrade websocket connection", err)
		return
	}

	receiver := make(chan ObserverMessage, 2)
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

func notify(c websocket.Conn, receiver <-chan ObserverMessage, pickLangs chan []*string) {
	defer c.Close()
	var listenerLangs = map[string]bool{}
	var wantsUnknownLangs = false
	for {
		select {
		case message := <-receiver:
			if message.Type == ObserverMessageTypePost &&
				!ListeningFor(listenerLangs, wantsUnknownLangs, message.Post.Post.Langs) {
				continue
			}
			data, err := message.toJson(time.Now())
			w, err := c.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(data)
			if err := w.Close(); err != nil {
				return
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
		}
	}
}

func (s *Server) oops(w http.ResponseWriter, r *http.Request) {
	errInfo := map[string]interface{}{}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(&errInfo)
	if err != nil {
		log.Println("failed to decode json during error report, trying to continue..")
	}
	errInfo["ua"] = r.UserAgent()
	jsonData, err := json.Marshal(&errInfo)
	log.Println("client error report", string(jsonData))
	w.WriteHeader(http.StatusCreated)
	io.WriteString(w, "got it. and sorry :/")
}

func (s *Server) getKnownLangs() []string {
	s.langsLock.Lock()
	defer s.langsLock.Unlock()
	return *s.knownLangs
}

func (s *Server) updateLangs(newLangs *[]string) {
	s.langsLock.Lock()
	defer s.langsLock.Unlock()
	s.knownLangs = newLangs
}

func (s *Server) broadcast(deletedFeed <-chan LikedPersistedPost, knownLangsFeed <-chan []string) {
	observers := make(map[chan ObserverMessage]bool)
	observersCountRefresh := 7 * time.Second
	observersCountTicker := time.NewTicker(observersCountRefresh)

	sendMessage := func(message ObserverMessage) bool {
		var toRemove = []chan ObserverMessage{}
		for c := range observers {
			select {
			case c <- message:
			default:
				log.Println("dropping client", c)
				toRemove = append(toRemove, c)
			}
		}
		for _, c := range toRemove {
			delete(observers, c)
		}
		return len(toRemove) > 0
	}

	for {
		select {
		case <-observersCountTicker.C:
			sendMessage(ObserverMessage{
				Type:           ObserverMessageTypeObservers,
				ObserversCount: len(observers),
			})
		case likedPost := <-deletedFeed:
			if sendMessage(ObserverMessage{
				Type: ObserverMessageTypePost,
				Post: &likedPost,
			}) {
				observersCountTicker.Reset(observersCountRefresh)
				sendMessage(ObserverMessage{
					Type:           ObserverMessageTypeObservers,
					ObserversCount: len(observers),
				})
				observersCount.Set(float64(len(observers)))
			}
		case newSeenLangs := <-knownLangsFeed:
			s.updateLangs(&newSeenLangs)
		case c := <-s.newObserver:
			observersCountTicker.Reset(observersCountRefresh)
			observers[c] = true
			sendMessage(ObserverMessage{
				Type:           ObserverMessageTypeObservers,
				ObserversCount: len(observers),
			})
			observersCount.Set(float64(len(observers)))
		}
	}
}

func redirectHost(host string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			newURL := r.URL
			newURL.Host = host
			newURL.Scheme = "https"
			http.Redirect(w, r, newURL.String(), 301)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Server) withReadyEndpoint(route string, app http.Handler) http.Handler {
	router := http.NewServeMux()
	router.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		io.WriteString(w, "ready")
	})
	router.Handle("/", app)
	return router
}

func Serve(env, port, host string, deletedFeed <-chan LikedPersistedPost, topLangsFeed <-chan []string) {

	server := Server{
		newObserver: make(chan chan ObserverMessage),
		knownLangs:  &[]string{"pt", "en", "ja"},
	}

	router := http.NewServeMux()
	router.Handle("GET /metrics", promhttp.Handler())
	router.HandleFunc("POST /oops", server.oops)
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" { // surprise, what a default :/
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Upgrade") == "websocket" {
			server.wsConnect(w, r)
		} else {
			server.index(w, r)
		}
	})

	var app http.Handler
	if host == "" {
		log.Println("no HOST set, allowing any host")
		app = router
	} else {
		log.Println("will serve for Host:", host, "(301 redirects for others)")
		app = redirectHost(host, router)
	}

	// fly health checks don't use our custom domain for HOST, so register
	// the /ready healthcheck endpoint outside the host redirect
	app = server.withReadyEndpoint("GET /ready", app)

	go server.broadcast(deletedFeed, topLangsFeed)

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, app))
}
