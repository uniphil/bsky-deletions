package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"math"
)

var postCacheDepth = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "post_cache_depth",
	Help: "Seconds since the oldest item was created",
})

var postCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "posts",
	Help: "Count of new posts",
}, []string{"lang", "target"})

var skippedPostCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "posts_skipped",
	Help: "Count of new post events that are not persisted in the cache",
}, []string{"reason"})

var postDeleteCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "post_deletes",
	Help: "Count of deleted posts, lang and target only available for cach hits",
}, []string{"lang", "target", "cache"})

func rounded(buckets []float64) []float64 {
	// the number of seconds is always ~large, so rounding has minimal effect
	// while labels on graphs are nicer
	out := make([]float64, len(buckets), len(buckets))
	for i, n := range buckets {
		out[i] = math.Round(n)
	}
	return out
}

var postAge = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "post_deleted_age",
	Help:    "Histogram of ages of deleted posts, cache misses excluded",
	Buckets: rounded(prometheus.ExponentialBuckets(20, 1.48, 24)),
}, []string{"target"})

var observersCount = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "post_deletion_observers",
	Help: "Number of people observing the deleted posts",
})

var likeRequestFails = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "post_like_request_fails",
	Help: "Failures to fetch likes for a post from atproto-link-aggregator",
}, []string{"reason"})
