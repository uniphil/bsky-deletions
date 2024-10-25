package main

import (
	"context"
	"encoding/json"
	"fmt"
	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/jetstream/pkg/client"
	"github.com/bluesky-social/jetstream/pkg/client/schedulers/sequential"
	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/cockroachdb/pebble"
	"log"
	"log/slog"
	"strings"
	"time"
)

type Consumer struct {
	DB      *pebble.DB
	outFeed chan PersistedPost
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
	maxRkeyTimeError time.Duration = MustParseDuration("5m")
	maxRkeySince     time.Duration = MustParseDuration("1h")
	maxPostRetention time.Duration = MustParseDuration("24h") * 2
)

type PersistedPost struct {
	TimeUS int64
	Text   string
	Langs  []string
	Target *PostTargetType
}

func Consume(ctx context.Context, env, dbPath string, logger *slog.Logger, outFeed chan PersistedPost) {
	wsUrl := "wss://jetstream1.us-east.bsky.network/subscribe"

	config := client.DefaultClientConfig()
	config.WebsocketURL = wsUrl
	config.Compress = true
	config.WantedCollections = []string{"app.bsky.feed.post"}

	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		log.Fatalf("failed to open db: %#v", err)
	}

	var cursor int64
	if env == "development" {
		cursor = time.Now().Add(5 * -time.Minute).UnixMicro()
	} else {
		cursor = 0
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

		if p.TimeUS > cursor {
			cursor = p.TimeUS
		}
	} else {
		log.Printf("no last el")
		cursor = time.Now().Add(5 * -time.Minute).UnixMicro()
	}
	if err := iter.Close(); err != nil {
		log.Fatalf("failed to close iterator: %s", err)
	}

	h := &PostHandler{
		DB:      db,
		OutFeed: outFeed,
	}

	scheduler := sequential.NewScheduler("asdf", logger, h.HandleEvent)

	// TODO: does the client already handle reconnects?
	c, err := client.NewClient(config, logger, scheduler)
	if err != nil {
		log.Fatalf("failed to create client: %#v", err)
	}

	// Every 5 seconds print the events read and bytes read and average event size
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-ticker.C:
				eventsRead := c.EventsRead.Load()
				bytesRead := c.BytesRead.Load()
				avgEventSize := bytesRead / eventsRead
				logger.Info("stats", "events_read", eventsRead, "bytes_read", bytesRead, "avg_event_size", avgEventSize)
			}
		}
	}()

	// trim db
	go func() {
		ticker := time.NewTicker(8 * time.Second)
		for {
			select {
			case <-ticker.C:
				// Trim the database
				if err := h.TrimEvents(ctx); err != nil {
					logger.Error("failed to trim events", "error", err)
				}
				logger.Info("trimmed")
			}
		}
	}()

	go func() {
		if err := c.ConnectAndRead(ctx, &cursor); err != nil {
			log.Fatalf("failed to connect: %#v", err)
		}
		h.DB.Close()
		slog.Info("gbyee")
	}()

}

type PostHandler struct {
	DB      *pebble.DB
	OutFeed chan PersistedPost
}

func PostKey(event *models.Event) ([]byte, error) {
	if event.EventType == models.EventCommit && event.Commit != nil && event.Commit.Collection == "app.bsky.feed.post" {
		return []byte(fmt.Sprintf("%s_%s", event.Commit.RKey, event.Did)), nil
	} else {
		return nil, fmt.Errorf("failed to generate key for persisting event: not a valid bsky post commit event")
	}
}

func (h *PostHandler) handlePersistPost(key []byte, post apibsky.FeedPost, time int64) error {
	// todo: validate rkey not too far in future

	redacted := Redact(post.Text, post.Facets)
	redacted = strings.TrimSpace(redacted)
	if redacted == "" {
		// drop empty posts (and updates that become empty)
		return nil
	}

	langs := NormalizeLangs(post.Langs)

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

	return nil
}

func (h *PostHandler) HandleEvent(ctx context.Context, event *models.Event) error {

	if !(event.EventType == models.EventCommit &&
		event.Commit != nil &&
		event.Commit.Collection == "app.bsky.feed.post") {
		// ignore non-post commits
		return nil
	}

	if event.Commit.OpType == models.CommitCreateRecord {
		parsed, err := syntax.ParseTID(event.Commit.RKey)
		if err != nil {
			fmt.Printf("failed to parse rkey %#v to TID, ignoring event.\n", err)
			return nil
		}
		rkeyTime := parsed.Time()

		eventTime := time.UnixMicro(event.TimeUS)
		timeError := rkeyTime.Sub(eventTime).Abs()
		if timeError > maxRkeyTimeError {
			fmt.Printf("rkey TID differs too much from post time, by %.s. ignoring event.\n", timeError)
			return nil
		}

		since := time.Since(rkeyTime).Abs()
		if since > maxRkeySince {
			fmt.Printf("rkey %#v may not be current time, since it differs by %.1fh. ignoring event.\n", event.Commit.RKey, since.Hours())
			return nil
		}
	}

	key := []byte(fmt.Sprintf("%s_%s", event.Commit.RKey, event.Did))

	if event.Commit.OpType == models.CommitCreateRecord || event.Commit.OpType == models.CommitUpdateRecord {
		var post apibsky.FeedPost
		if err := json.Unmarshal(event.Commit.Record, &post); err != nil {
			return fmt.Errorf("failed to unmarshal post: %#v", err)
		}

		postTime := event.TimeUS
		if event.Commit.OpType == models.CommitUpdateRecord {
			existing, err := h.TakeEvent(key)
			if err != nil {
				if err == pebble.ErrNotFound {
					// cache miss: ignore
					return nil
				} else {
					return fmt.Errorf("failed to get existing event: %#v", err)
				}
			}
			if existing == nil {
				// ignore update commits for posts we don't have
				return nil
			}
			postTime = existing.TimeUS
		}

		return h.handlePersistPost(key, post, postTime)
	} else if event.Commit.OpType == models.CommitDeleteRecord {
		post, err := h.TakeEvent(key)
		if err != nil {
			if err == pebble.ErrNotFound {
				// cache miss: ignore
				return nil
			} else {
				return err
			}
		}
		if post != nil {
			h.OutFeed <- *post
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

	// Keys are stored as strings of the event time in microseconds
	// We can range delete events older than the event TTL
	trimUntil := time.Now().Add(-maxPostRetention)
	log.Printf("trimUntil?? %s mpr: %s\n", time.Since(trimUntil), maxPostRetention)
	trimUntilRkey := syntax.NewTID(trimUntil.UnixMicro(), 0)
	trimKey := []byte(trimUntilRkey.String())
	watTid := syntax.TID(trimUntilRkey)
	log.Printf("trimkey since: %s\n", time.Since(watTid.Time()))

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
		log.Printf("earliest ts before: %s, k: %s\n", dt, iter.Key())
	}
	if err := iter.Close(); err != nil {
		return err
	}

	log.Printf("trimming until %s\n", trimUntilRkey)

	// Delete all numeric keys older than the trim key
	if err := h.DB.DeleteRange([]byte("0"), trimKey, pebble.Sync); err != nil {
		log.Printf("no, bad, failed to delete %s", err)
		return fmt.Errorf("failed to delete old events: %#v", err)
	}

	log.Printf("ok so it did delete...")

	iter, err = h.DB.NewIter(nil)
	if err != nil {
		log.Fatalf("failed to get db iter: %#v", err)
	}
	if iter.First() {
		var p PersistedPost
		if err := json.Unmarshal(iter.Value(), &p); err != nil {
			log.Fatalf("failed to read latest entry: %#v", err)
		}
		dt := time.Since(time.UnixMicro(p.TimeUS))
		log.Printf("earliest ts after: %s, k: %s\n", dt, iter.Key())
	} else {
		log.Printf("nothing left in db after delete i guess")
	}
	if err := iter.Close(); err != nil {
		return err
	}

	return nil
}
