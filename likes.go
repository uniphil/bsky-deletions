package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var reqClient = http.Client{
	Timeout: 240 * time.Millisecond,
}

type LikedPersistedPost struct {
	Post  *PersistedPost
	Likes *uint32
}

type LinksResult struct {
	Total uint32 `json:"total"`
}

func GetLikes(uncovered UncoveredPost) LikedPersistedPost {
	likes := getLikes(uncovered.Did, uncovered.RKey)
	return LikedPersistedPost{
		Post:  uncovered.Post,
		Likes: likes,
	}
}

func getLikes(did, rkey string) *uint32 {
	// format: at://did:plc:ezxfbsdjjylaoagv5bvz7sqb/app.bsky.feed.post/3lbb2ddbbn22c
	targetUri := "at://" + did + "/app.bsky.feed.post/" + rkey // hack

	aggregatorBase := "https://constellation.microcosm.blue/links/count"
	query := url.Values{}
	query.Set("target", targetUri)
	query.Set("collection", "app.bsky.feed.like")
	query.Set("path", ".subject.uri")

	uri := aggregatorBase + "?" + query.Encode()
	req, err := http.NewRequest("GET", uri, nil);
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "final words (deletions.bsky.bad-example.com)/v1.0")

	res, err := reqClient.Do(req)
	if err != nil {
		var urlErr url.Error
		if errors.As(err, &urlErr.Err) && urlErr.Timeout() {
			likeRequestFails.WithLabelValues("request timeout").Inc()
		} else {
			likeRequestFails.WithLabelValues("request error").Inc()
		}
		return nil
	}
	if res.StatusCode != 200 {
		likeRequestFails.WithLabelValues(fmt.Sprintf("http %d", res.StatusCode)).Inc()
		return nil
	}

	likesRes := LinksResult{}
	err = json.NewDecoder(res.Body).Decode(&likesRes)
	if err != nil {
		likeRequestFails.WithLabelValues("json decode").Inc()
	}

	return &likesRes.Total
}
