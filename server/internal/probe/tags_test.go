package probe

import "testing"

func TestParseReadsContainerTags(t *testing.T) {
	data := []byte(`{"streams":[{"index":0,"codec_type":"audio","codec_name":"aac","channels":2}],
	"format":{"duration":"3600.0","tags":{
		"title":"Project Hail Mary (2021)",
		"artist":"Andy Weir",
		"album_artist":"Andy Weir",
		"album":"Project Hail Mary (2021)",
		"date":"2021",
		"comment":"Narrated by Ray Porter",
		"genre":"Science Fiction",
		"lyrics":"Ryland Grace is the sole survivor."}}}`)
	res, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tags.Title != "Project Hail Mary (2021)" {
		t.Errorf("title = %q", res.Tags.Title)
	}
	if res.Tags.AlbumArtist != "Andy Weir" {
		t.Errorf("album_artist = %q", res.Tags.AlbumArtist)
	}
	if res.Tags.Comment != "Narrated by Ray Porter" {
		t.Errorf("comment = %q", res.Tags.Comment)
	}
	if res.Tags.Genre != "Science Fiction" {
		t.Errorf("genre = %q", res.Tags.Genre)
	}
	if res.Tags.Date != "2021" {
		t.Errorf("date = %q", res.Tags.Date)
	}
}

func TestParseTagsEmpty(t *testing.T) {
	res, err := Parse([]byte(`{"streams":[{"codec_type":"audio","codec_name":"aac"}],"format":{"duration":"1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Tags != (FileTags{}) {
		t.Errorf("tags = %+v, want zero value", res.Tags)
	}
}

// MP4 atom tags arrive lower-cased from ffprobe, FFMETADATA1 files use the
// uppercase names, and ID3-derived containers use the "©" forms. All three must
// resolve to the same field.
func TestParseTagKeySpellings(t *testing.T) {
	cases := []struct {
		key, field string
	}{
		{"ARTIST", "Artist"},
		{"artist", "Artist"},
		{"©ART", "Artist"},
		{"album_artist", "AlbumArtist"},
		{"ALBUM_ARTIST", "AlbumArtist"},
		{"Year", "Date"},
		{"DESCRIPTION", "Description"},
	}
	for _, c := range cases {
		raw := []byte(`{"streams":[],"format":{"tags":{"` + c.key + `":"value"}}}`)
		res, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", c.key, err)
		}
		got := map[string]string{
			"Artist":      res.Tags.Artist,
			"AlbumArtist": res.Tags.AlbumArtist,
			"Date":        res.Tags.Date,
			"Description": res.Tags.Description,
		}[c.field]
		if got != "value" {
			t.Errorf("%s: %s = %q, want %q", c.key, c.field, got, "value")
		}
	}
}

// A container listing both "©ART" and "artist" must not end up blank, and
// empty values must not clobber a populated one.
func TestParseTagsPrefersNonEmpty(t *testing.T) {
	raw := []byte(`{"streams":[],"format":{"tags":{"©ART":"","artist":"Real Name","genre":"   "}}}`)
	res, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tags.Artist != "Real Name" {
		t.Errorf("artist = %q, want %q", res.Tags.Artist, "Real Name")
	}
	if res.Tags.Genre != "" {
		t.Errorf("genre = %q, want empty", res.Tags.Genre)
	}
}

func TestNormalizeTagKey(t *testing.T) {
	cases := map[string]string{
		"©ART":         "art",
		"album_artist": "album_artist",
		"  Title  ":    "title",
		"Track Number": "track_number",
		// A freeform atom is reduced to its trailing field name.
		"----:com.apple.iTunes:ALBUMARTIST": "albumartist",
	}
	for in, want := range cases {
		if got := normalizeTagKey(in); got != want {
			t.Errorf("normalizeTagKey(%q) = %q, want %q", in, got, want)
		}
	}
}
