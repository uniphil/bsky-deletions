package main

import (
	"reflect"
	"testing"
)

func TestNormalizeLangs(t *testing.T) {
	if !reflect.DeepEqual(NormalizeLangs([]string{}), []string{}) {
		t.Fatalf("empty slice should stay empty")
	}
	if !reflect.DeepEqual(NormalizeLangs([]string{"en"}), []string{"en"}) {
		t.Fatalf("single element should remain")
	}
	if !reflect.DeepEqual(NormalizeLangs([]string{"en", "en"}), []string{"en"}) {
		t.Fatalf("repeated elements should be removed")
	}
	if !reflect.DeepEqual(NormalizeLangs([]string{"pt", "en"}), []string{"pt", "en"}) {
		t.Fatalf("distinct elements should remain")
	}
	if !reflect.DeepEqual(NormalizeLangs([]string{"en-CA", "en"}), []string{"en"}) {
		t.Fatalf("hyphenated bits should be stripped")
	}
	if !reflect.DeepEqual(NormalizeLangs([]string{"EN", "en"}), []string{"en"}) {
		t.Fatalf("casing should be normalized")
	}
}

func TestTopLangs(t *testing.T) {
	if !reflect.DeepEqual(topLangs(0, map[string]int64{}), []string{}) {
		t.Fatalf("empty map should yield empty slice")
	}
	if !reflect.DeepEqual(topLangs(1, map[string]int64{"en": 1}), []string{"en"}) {
		t.Fatalf("lang seen is not dropped")
	}
	if !reflect.DeepEqual(topLangs(10_000, map[string]int64{"pt": 10_000, "en": 1}), []string{"pt"}) {
		t.Fatalf("lang with too few sightings is dropped")
	}
	if !reflect.DeepEqual(topLangs(10_000, map[string]int64{
		"en":   5_000,
		"hu":   11,
		"ja":   3_000,
		"pt":   10_000,
		"spam": 9,
	}), []string{"pt", "en", "ja", "hu"}) {
		t.Fatalf("langs are sorted descending")
	}
}

func TestListeningFor(t *testing.T) {
	if ListeningFor(map[string]bool{}, false, []string{}) != true {
		t.Fatalf("should hear all langs when none are specified")
	}
	if ListeningFor(map[string]bool{}, false, []string{"en"}) != true {
		t.Fatalf("should hear any lang when none is specified")
	}
	if ListeningFor(map[string]bool{}, true, []string{}) != true {
		t.Fatalf("should hear unspecified lang when unspecified is specified")
	}
	if ListeningFor(map[string]bool{}, true, []string{"en"}) != false {
		t.Fatalf("should not hear known langs when none are specified and unknown is true")
	}

	if ListeningFor(map[string]bool{"en": true}, false, []string{}) != false {
		t.Fatalf("should not hear when unknown is false and lang is unspecified")
	}
	if ListeningFor(map[string]bool{"en": true}, false, []string{"en"}) != true {
		t.Fatalf("should hear specified lang")
	}
	if ListeningFor(map[string]bool{"en": true}, false, []string{"pt"}) != false {
		t.Fatalf("should not hear non-matching lang")
	}
	if ListeningFor(map[string]bool{"en": true}, false, []string{"pt", "en", "ja"}) != true {
		t.Fatalf("should here for any matching post lang")
	}

	if ListeningFor(map[string]bool{"en": true}, true, []string{}) != true {
		t.Fatalf("should hear unspecified post when wantsUnknown")
	}
	if ListeningFor(map[string]bool{"en": true}, true, []string{"en"}) != true {
		t.Fatalf("should hear specified lang")
	}
	if ListeningFor(map[string]bool{"en": true}, true, []string{"pt"}) != false {
		t.Fatalf("should not hear specified lang thats not listened for")
	}
	if ListeningFor(map[string]bool{"en": true}, true, []string{"pt", "en", "ja"}) != true {
		t.Fatalf("should here for any matching post lang")
	}
}
