package main

import (
	"sort"
	"strings"
	"time"
)

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

func topLangs(topLangCount int64, langsSeen map[string]int64) []string {
	langs := []string{}
	for lang, count := range langsSeen {
		if count > (topLangCount / 1000) {
			langs = append(langs, lang)
		}
	}
	sort.Slice(langs, func(i, j int) bool {
		return langsSeen[langs[i]] > langsSeen[langs[j]]
	})
	return langs
}

func CountLangs(postLangs <-chan []string) <-chan []string {
	topLangsFeed := make(chan []string)

	go func() {
		var topLangCount int64
		langsSeen := map[string]int64{}
		langsUpdateTicker := time.NewTicker(4 * time.Second)
		for {
			select {
			case langs := <-postLangs:
				for _, lang := range langs {
					langsSeen[lang] += 1
					topLangCount = max(topLangCount, langsSeen[lang])
				}
			case <-langsUpdateTicker.C:
				topLangsFeed <- topLangs(topLangCount, langsSeen)
			}
		}
	}()

	return topLangsFeed
}

func ListeningFor(listenerLangs map[string]bool, wantsUnknown bool, postLangs []string) bool {
	if len(listenerLangs) == 0 {
		if wantsUnknown {
			return len(postLangs) == 0 // only want unspecified posts
		} else {
			return true // no langs select => all posts
		}
	}
	if len(postLangs) == 0 {
		return wantsUnknown // no unknown langs unless asked for
	}
	for _, postLang := range postLangs {
		if listenerLangs[postLang] {
			return true
		}
	}
	return false
}
