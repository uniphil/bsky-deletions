package main

import (
	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"sort"
	"strings"
)

type redactable struct {
	replacement []byte
	index       apibsky.RichtextFacet_ByteSlice
}

func isRedactable(facet apibsky.RichtextFacet) *redactable {
	if facet.Index == nil {
		return nil
	}
	for _, feat := range facet.Features {
		if feat == nil {
			continue
		}
		if feat.RichtextFacet_Mention != nil {
			return &redactable{
				index:       *facet.Index,
				replacement: []byte("@█████████"),
			}
		} else if feat.RichtextFacet_Link != nil {
			return &redactable{
				index:       *facet.Index,
				replacement: []byte("www.█████████"),
			}
		}
	}
	return nil
}

func Redact(text string, facets []*apibsky.RichtextFacet) string {
	allFacets := facets
	if len(allFacets) == 0 {
		return text
	}
	sourceBytes := []byte(text)

	var sourceEnd = int64(len(sourceBytes) - 1)
	var lastEnd int64 = 0
	var redactedText []byte

	// https://docs.bsky.app/docs/advanced-guides/post-richtext

	// 0. discard facets we don't care about
	redactions := []redactable{}
	for _, facet := range allFacets {
		if facet == nil {
			continue
		}
		if redaction := isRedactable(*facet); redaction != nil {
			redactions = append(redactions, *redaction)
		}
	}

	// 1. sort facets by start index
	sort.Slice(redactions, func(i, j int) bool {
		return redactions[i].index.ByteStart < redactions[j].index.ByteStart
	})

	for _, redaction := range redactions {
		// 2. discard any facets that overlap eachother
		if redaction.index.ByteStart < lastEnd {
			continue
		}
		// 2.1. discard any facets that are out of range
		if redaction.index.ByteStart > sourceEnd {
			break // since we sorted by start index, there cannot be any more valid starts
		}
		// 2.2 discard any facets that are invalid (end <= start)
		if redaction.index.ByteEnd <= redaction.index.ByteStart {
			continue
		}
		// 3. apply redactions
		redactedText = append(redactedText, sourceBytes[lastEnd:redaction.index.ByteStart]...)
		redactedText = append(redactedText, redaction.replacement...)
		lastEnd = redaction.index.ByteEnd
	}

	if lastEnd < sourceEnd {
		redactedText = append(redactedText, sourceBytes[lastEnd:]...)
	}

	return string(redactedText)
}

func NormalizeLangs(langs []string) []string {
	normalized := []string{}
	seen := map[string]bool{}
	for _, lang := range langs {
		before, _, _ := strings.Cut(lang, "-")
		k := strings.ToLower(before)
		if !seen[k] {
			normalized = append(normalized, k)
			seen[k] = true
		}
	}
	return normalized
}
