package main

import (
	"encoding/json"
	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"testing"
)

func redactJson(t *testing.T, input, expected, rawFacets string) {
	var facets []*apibsky.RichtextFacet
	if err := json.Unmarshal([]byte(rawFacets), &facets); err != nil {
		t.Fatalf("json unmartial failed on: %#v\nbecause: %#v", input, err)
	}
	redacted := Redact(input, facets)
	if redacted != expected {
		t.Fatalf("redacted text was not as expected.\ngot: %#v\nexpected: %#v\ninput: %#v", redacted, expected, input)
	}
}

func TestRedactEmpty(t *testing.T) {
	redactJson(t, "", "", "[]")
}

func TestRedactNothing(t *testing.T) {
	redactJson(t, "nothing to redact", "nothing to redact", "[]")
}

func TestRedact(t *testing.T) {
	redactJson(t,
		"@me testing tagging @someone in a post https://example.com",
		"@█████████ testing tagging @█████████ in a post www.█████████",
		`[
	      {
	        "$type": "app.bsky.richtext.facet",
	        "features": [
	          {
	            "$type": "app.bsky.richtext.facet#mention",
	            "did": "did:plc:xxxxxx"
	          }
	        ],
	        "index": { "byteStart": 0, "byteEnd": 3 }
	      },
	      {
	        "$type": "app.bsky.richtext.facet",
	        "features": [
	          {
	            "$type": "app.bsky.richtext.facet#mention",
	            "did": "did:plc:xxxxxx"
	          }
	        ],
	        "index": { "byteStart": 20, "byteEnd": 28 }
	      },
	      {
	        "$type": "app.bsky.richtext.facet",
	        "features": [
	          {
	            "$type": "app.bsky.richtext.facet#link",
	            "uri": "https://www.example.com/0123456789"
	          }
	        ],
	        "index": { "byteStart": 39, "byteEnd": 58 }
	      }
	    ]`)
}

func TestRedactOverExtendedFacet(t *testing.T) {
	redactJson(t, "short @tag", "short @█████████", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 6, "byteEnd": 10 }
      }
	]`)
	redactJson(t, "short @tag", "short @█████████", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 6, "byteEnd": 11 }
      }
	]`)
	redactJson(t, "short @tag", "short @█████████", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 6, "byteEnd": 20 }
      }
	]`)
}
func TestRedactFloatingFacet(t *testing.T) {
	redactJson(t, "short", "short", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 20, "byteEnd": 30 }
      }
	]`)
}
func TestRedactInvalidFacet(t *testing.T) {
	redactJson(t, "one two three", "one two three", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 5, "byteEnd": 5 }
      }
	]`)
	redactJson(t, "one two three", "one two three", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 5, "byteEnd": 4 }
      }
	]`)
}
func TestRedactOverlappingFacets(t *testing.T) {
	redactJson(t, "0123456789", "01@█████████6789", `[
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 2, "byteEnd": 6 }
      },
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": { "byteStart": 4, "byteEnd": 8 }
      }
	]`)
}
