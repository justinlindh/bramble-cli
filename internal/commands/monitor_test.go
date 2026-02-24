package commands

import (
	"regexp"
	"testing"
	"time"
)

func TestParseMonitorTopicsCSV(t *testing.T) {
	t.Parallel()

	topics := parseMonitorTopicsCSV(" wifi, gps,mesh ,,location ")

	for _, want := range []string{"wifi", "gps", "mesh", "location"} {
		if _, ok := topics[want]; !ok {
			t.Fatalf("expected topic %q in parsed set: %#v", want, topics)
		}
	}
}

func TestMonitorEventMatches_TopicFilter(t *testing.T) {
	t.Parallel()

	opts := monitorFilterOptions{topics: map[string]struct{}{"mesh": {}}}
	evt := monitorEvent{Topic: "traffic", Timestamp: time.Now(), SearchText: "hello"}

	if monitorEventMatches(opts, evt) {
		t.Fatal("expected event with non-matching topic to be filtered out")
	}
}

func TestMonitorEventMatches_GrepFilter(t *testing.T) {
	t.Parallel()

	opts := monitorFilterOptions{grep: regexp.MustCompile("(?i)packet dropped")}

	if monitorEventMatches(opts, monitorEvent{Timestamp: time.Now(), SearchText: "tx ok"}) {
		t.Fatal("expected grep filter to reject non-matching text")
	}
	if !monitorEventMatches(opts, monitorEvent{Timestamp: time.Now(), SearchText: "PACKET dropped on retry"}) {
		t.Fatal("expected grep filter to accept matching text")
	}
}

func TestMonitorEventMatches_SinceFilter(t *testing.T) {
	t.Parallel()

	now := time.Unix(2000, 0)
	opts := monitorFilterOptions{since: 5 * time.Minute, now: func() time.Time { return now }}

	oldEvt := monitorEvent{Timestamp: now.Add(-10 * time.Minute), SearchText: "old"}
	if monitorEventMatches(opts, oldEvt) {
		t.Fatal("expected event older than --since cutoff to be filtered out")
	}

	newEvt := monitorEvent{Timestamp: now.Add(-1 * time.Minute), SearchText: "new"}
	if !monitorEventMatches(opts, newEvt) {
		t.Fatal("expected recent event to pass --since filter")
	}
}

func TestMonitorShouldStopAfterMatchWhenNotFollow(t *testing.T) {
	t.Parallel()

	if !monitorShouldStopAfterMatch(false) {
		t.Fatal("expected non-follow mode to stop after first matching event")
	}
	if monitorShouldStopAfterMatch(true) {
		t.Fatal("expected follow mode to keep streaming")
	}
}
