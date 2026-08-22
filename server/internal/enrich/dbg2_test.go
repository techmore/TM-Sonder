package enrich

import (
	"context"
	"os"
	"testing"
)

func TestDbgInceptionLive(t *testing.T) {
	cache := os.Getenv("HOME") + "/.config/sonder/data/metadata-cache"
	e := New(cache)
	got, err := e.Enrich(context.Background(), Input{
		Title: "Inception", Kind: "movie", Year: 2010,
	})
	t.Logf("err=%v nil=%v", err, got == nil)
	if got != nil {
		s := got.Summary
		if len(s) > 40 {
			s = s[:40]
		}
		t.Logf("summary=%q provider=%q poster=%v", s, got.Provider, got.PosterPath != "")
	}
}
