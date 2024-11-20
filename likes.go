package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type LikedPersistedPost struct {
	Post  *PersistedPost
	Likes *uint32
}

type LikesResult struct {
	Likes uint32 `json:"total_likes"`
	// Dids  []string `json:"latest_dids"`
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

	aggregatorBase := "https://atproto-link-aggregator.fly.dev/likes"
	query := url.Values{}
	query.Set("uri", targetUri)
	url := aggregatorBase + "?" + query.Encode()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Printf("failed to build request: %w\n", err)
		return nil
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("failed to do request: %w\n", err)
		return nil
	}
	if res.StatusCode != 200 {
		fmt.Printf("non-200 response: %w\n", err)
		return nil
	}

	likesRes := LikesResult{}
	err = json.NewDecoder(res.Body).Decode(&likesRes)
	if err != nil {
		fmt.Printf("failed to decode: %w\n", err)
		return nil
	}

	return &likesRes.Likes
}
