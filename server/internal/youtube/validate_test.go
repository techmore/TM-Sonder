package youtube

import "testing"

func TestChannelURLAllowlist(t *testing.T) {
	ok := []string{
		"https://www.youtube.com/@some.channel",
		"https://youtube.com/@some-channel/videos",
		"https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv",
		"https://www.youtube.com/c/Name/shorts/",
		"https://www.youtube.com/user/name/streams",
	}
	bad := []string{
		"",
		"http://www.youtube.com/@x", // no TLS
		"https://evil.example/https://www.youtube.com/@x", // other host
		"https://www.youtube.com.evil.example/@x",
		"https://www.youtube.com/@x?list=1",
		"https://www.youtube.com/@x\n--exec=id",
		"--exec=id https://www.youtube.com/@x",
		"https://www.youtube.com/channel/UCshort",
		"https://www.youtube.com/watch?v=abcdefghijk",
	}
	for _, u := range ok {
		if !channelURL.MatchString(u) {
			t.Errorf("rejected valid channel URL %q", u)
		}
	}
	for _, u := range bad {
		if channelURL.MatchString(u) {
			t.Errorf("accepted unsafe channel URL %q", u)
		}
	}
}

func TestVideoIDAllowlist(t *testing.T) {
	for _, id := range []string{"dQw4w9WgXcQ", "a_b-c123456"} {
		if !videoID.MatchString(id) {
			t.Errorf("rejected valid id %q", id)
		}
	}
	// IDs are only ever interpolated into a watch URL placed after "--", so a
	// leading '-' is harmless; the pattern guarantees charset and length.
	for _, id := range []string{"", "short", "dQw4w9WgXcQ-", "dQw4w9WgXc\n", "dQw4w9WgX;Q"} {
		if videoID.MatchString(id) {
			t.Errorf("accepted bad id %q", id)
		}
	}
}

func TestValidateSettings(t *testing.T) {
	good := Settings{MinFreeGB: 500, MaxHeight: 1080, Profile: "efficient"}
	if err := validateSettings(good); err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Settings){
		"free space zero":     func(s *Settings) { s.MinFreeGB = 0 },
		"free space huge":     func(s *Settings) { s.MinFreeGB = 100001 },
		"unsupported height":  func(s *Settings) { s.MaxHeight = 999 },
		"unsupported profile": func(s *Settings) { s.Profile = "ultra" },
	} {
		s := good
		mutate(&s)
		if err := validateSettings(s); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
