package main

import (
	"context"
	"encoding/json"
	"fmt"
	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/client/schedulers/parallel"
	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/cockroachdb/pebble"
	"log"
	"log/slog"
	"strings"
	"time"
)

type PostHandler struct {
	DB            *pebble.DB
	DeletedFeed   chan<- LikedPersistedPost
	LanguagesFeed chan<- []string
}

type PostTargetType string

const (
	ReplyTarget PostTargetType = "reply"
	QuoteTarget PostTargetType = "quote"
)

func MustParseDuration(d string) time.Duration {
	parsed, err := time.ParseDuration(d)
	if err != nil {
		panic(fmt.Sprintf("failed to parse duration '%s': %#v", d, err))
	}
	return parsed
}

var ( // gross: duration can't be const
	maxRkeyTimeError  time.Duration = MustParseDuration("1h")
	maxRkeySince      time.Duration = MustParseDuration("25h") // allow backfill: jetstream max retention plus an hour
	maxPostRetention  time.Duration = MustParseDuration("24h") * 2
	connectRetryReset time.Duration = MustParseDuration("15m")
	connectRetryWait  time.Duration = MustParseDuration("3s")
)

type PersistedPost struct {
	TimeUS int64
	Text   string
	Langs  []string
	Target *PostTargetType
}

type UncoveredPost struct {
	Post *PersistedPost
	Did  string
	RKey string
}

func (p *PersistedPost) AgeMs(t time.Time) int64 {
	return (t.UnixMicro() - p.TimeUS) / 1000
}

func (p *PersistedPost) FirstLang() string {
	// only for metrics: for simplicity we only keep the first language.
	// posts with no language get the special value "-", which can never occur
	// in the saved languages after normalization.
	if len(p.Langs) == 0 {
		return "-"
	} else {
		return p.Langs[0]
	}
}

func (p *PersistedPost) TargetName() string {
	if p.Target == nil {
		return "post"
	} else {
		return string(*p.Target)
	}
}

func Consume(ctx context.Context, env, jsUrl, dbPath string, logger *slog.Logger) (<-chan LikedPersistedPost, <-chan []string) {
	config := client.DefaultClientConfig()
	config.WebsocketURL = jsUrl
	config.Compress = true
	config.WantedCollections = []string{"app.bsky.feed.post"}

	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		log.Fatalf("failed to open db: %#v", err)
	}

	iter, err := db.NewIter(nil)
	if err != nil {
		log.Fatalf("failed to get db iter: %#v", err)
	}
	if iter.Last() {
		var p PersistedPost
		if err := json.Unmarshal(iter.Value(), &p); err != nil {
			log.Fatalf("failed to read latest entry: %#v", err)
		}
		log.Printf("latest ts: %d : %s\n", p.TimeUS, p.Text)
	} else {
		log.Printf("no last el")
	}
	if err := iter.Close(); err != nil {
		log.Fatalf("failed to close iterator: %s", err)
	}

	deletedFeed := make(chan LikedPersistedPost, 120)
	languagesFeed := make(chan []string, 2)

	h := &PostHandler{
		DB:            db,
		LanguagesFeed: languagesFeed,
		DeletedFeed:   deletedFeed,
	}

	scheduler := parallel.NewScheduler(21, "asdf", logger, h.HandleEvent)

	c, err := client.NewClient(config, logger, scheduler)
	if err != nil {
		log.Fatalf("failed to create client: %#v", err)
	}

	go func() {
		trimTicker := time.NewTicker(8 * time.Second)
		for range trimTicker.C {
			if err := h.TrimEvents(ctx); err != nil {
				logger.Error("failed to trim events", "error", err)
			}
		}
	}()

	go func() {
		defer h.DB.Close()

		var retry = 0
		var lastConnect = time.Now()
		for {
			if err := c.ConnectAndRead(ctx, nil); err != nil {
				if time.Since(lastConnect) >= connectRetryReset {
					retry = 0
					logger.Info("jetstream connection ended with error, will retry", err)
				} else {
					retry += 1
					if retry >= 7 {
						log.Fatalf("jetstream: connection ended with error and no more retries. exiting", err)
					} else {
						logger.Info("jetstream connection ended with error", err, "retry:", retry)
					}
				}
				time.Sleep(connectRetryWait)
				lastConnect = time.Now()
			} else {
				logger.Info("jetstream ended without error. exiting..?")
				break
			}
		}
		logger.Info("gbyeee from jetstream")
	}()

	return deletedFeed, languagesFeed
}

func PostKey(event *models.Event) ([]byte, error) {
	if event.Kind == models.EventKindCommit && event.Commit != nil && event.Commit.Collection == "app.bsky.feed.post" {
		return []byte(fmt.Sprintf("%s_%s", event.Commit.RKey, event.Did)), nil
	} else {
		return nil, fmt.Errorf("failed to generate key for persisting event: not a valid bsky post commit event")
	}
}

func (h *PostHandler) handlePersistPost(key []byte, post apibsky.FeedPost, time int64) error {
	redacted := Redact(post.Text, post.Facets)
	redacted = strings.TrimSpace(redacted)
	if redacted == "" { // drop empty posts (and updates that become empty)
		return nil
	}

	langs := NormalizeLangs(post.Langs)
	h.LanguagesFeed <- langs

	var target *PostTargetType = nil
	if post.Reply != nil {
		var addressable = ReplyTarget
		target = &addressable
	} else if post.Embed != nil && post.Embed.EmbedRecord != nil {
		var addressable = QuoteTarget
		target = &addressable
	}

	persistable := PersistedPost{
		TimeUS: time,
		Text:   redacted,
		Langs:  langs,
		Target: target,
	}

	if err := h.PersistEvent(key, persistable); err != nil {
		return fmt.Errorf("failed to persist post: %#v", err)
	}

	postCounter.WithLabelValues(persistable.FirstLang(), persistable.TargetName()).Inc()

	return nil
}

func (h *PostHandler) HandleEvent(ctx context.Context, event *models.Event) error {

	if !(event.Kind == models.EventKindCommit &&
		event.Commit != nil &&
		event.Commit.Collection == "app.bsky.feed.post") {
		// ignore non-post commits
		return nil
	}

	if event.Commit.Operation == models.CommitOperationCreate {
		parsed, err := syntax.ParseTID(event.Commit.RKey)
		if err != nil {
			skippedPostCounter.WithLabelValues("TID failed to parse from rkey").Inc()
			return nil
		}
		rkeyTime := parsed.Time()

		eventTime := time.UnixMicro(event.TimeUS)
		timeError := rkeyTime.Sub(eventTime).Abs()
		if timeError > maxRkeyTimeError {
			skippedPostCounter.WithLabelValues("TID from rkey too far from event time").Inc()
			return nil
		}

		since := time.Since(rkeyTime).Abs()
		if since > maxRkeySince {
			skippedPostCounter.WithLabelValues("TID from rkey too far from now").Inc()
			return nil
		}
	}

	key := []byte(fmt.Sprintf("%s_%s", event.Commit.RKey, event.Did))

	if event.Commit.Operation == models.CommitOperationCreate || event.Commit.Operation == models.CommitOperationUpdate {
		var post apibsky.FeedPost
		if err := json.Unmarshal(event.Commit.Record, &post); err != nil {
			skippedPostCounter.WithLabelValues("unmarshalling record failed").Inc()
			return fmt.Errorf("failed to unmarshal post: %#v", err)
		}

		postTime := event.TimeUS
		if event.Commit.Operation == models.CommitOperationUpdate {
			existing, err := h.TakeEvent(key)
			if err != nil {
				if err == pebble.ErrNotFound {
					// cache miss: ignore
					return nil
				} else {
					skippedPostCounter.WithLabelValues("pebble existing post query failed").Inc()
					return fmt.Errorf("failed to get existing event: %#v", err)
				}
			}
			if existing == nil {
				// ignore update commits for posts we don't have
				return nil
			}
			postTime = existing.TimeUS
		}

		if err := h.handlePersistPost(key, post, postTime); err != nil {
			skippedPostCounter.WithLabelValues("persisting failed").Inc()
			return err
		}
		return nil
	} else if event.Commit.Operation == models.CommitOperationDelete {
		post, err := h.TakeEvent(key)
		if err != nil {
			if err == pebble.ErrNotFound { // cache miss: ignore
				postDeleteCounter.WithLabelValues("-", "-", "miss").Inc()
				return nil
			} else {
				return err
			}
		}
		if post != nil {
			uncovered := UncoveredPost{
				Post: post,
				Did:  event.Did,
				RKey: event.Commit.RKey,
			}
			liked := GetLikes(uncovered)
			select {
			case h.DeletedFeed <- liked:
			default:
				fmt.Printf("dropping deleted post because the channel is full\n")
			}
			postAge.WithLabelValues(post.TargetName()).Observe(float64(post.AgeMs(time.Now())) / 1000)
			postDeleteCounter.WithLabelValues(post.FirstLang(), post.TargetName(), "hit").Inc()
		}
	}
	return nil
}

func (h *PostHandler) PersistEvent(key []byte, post PersistedPost) error {
	data, err := json.Marshal(&post)
	if err != nil {
		return fmt.Errorf("failed to marshal post to entry: %#v", err)
	}

	err = h.DB.Set(key, data, pebble.NoSync)
	if err != nil {
		fmt.Printf("failed to write event to pebble: %#v", err)
		return fmt.Errorf("failed to write event to pebble: %#v", err)
	}
	return nil
}

func (h *PostHandler) TakeEvent(key []byte) (*PersistedPost, error) {
	data, closer, err := h.DB.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	if err := h.DB.Delete(key, pebble.NoSync); err != nil {
		return nil, err
	}
	var p PersistedPost
	err = json.Unmarshal(data, &p)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal from pebble: %#v", err)
	}
	return &p, nil
}

func (h *PostHandler) TrimEvents(ctx context.Context) error {

	// register the oldest event pre-trim: how  much we are overshooting
	iter, err := h.DB.NewIter(nil)
	if err != nil {
		log.Fatalf("failed to get db iter: %#v", err)
	}
	if iter.First() {
		var p PersistedPost
		if err := json.Unmarshal(iter.Value(), &p); err != nil {
			log.Fatalf("failed to read latest entry: %#v", err)
		}
		dt := time.Since(time.UnixMicro(p.TimeUS))
		postCacheDepth.Set(dt.Seconds())
	} else {
		log.Printf("nothing in db to set cache depth gauge from")
	}
	if err := iter.Close(); err != nil {
		return err
	}

	// Keys are stored as strings of the event time in microseconds
	// We can range delete events older than the event TTL
	trimUntil := time.Now().Add(-maxPostRetention)
	trimUntilRkey := syntax.NewTID(trimUntil.UnixMicro(), 0)
	trimKey := []byte(trimUntilRkey.String())

	// Delete all numeric keys older than the trim key
	if err := h.DB.DeleteRange([]byte("0"), trimKey, pebble.Sync); err != nil {
		log.Printf("no, bad, failed to delete %s", err)
		return fmt.Errorf("failed to delete old events: %#v", err)
	}

	return nil
}
