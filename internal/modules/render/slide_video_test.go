package render

import (
	"strings"
	"testing"
)

func TestVideoEmbedURL(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=abc123":     "https://www.youtube-nocookie.com/embed/abc123",
		"https://youtu.be/abc123":                    "https://www.youtube-nocookie.com/embed/abc123",
		"https://youtube.com/embed/abc123":           "https://youtube.com/embed/abc123",
		"https://vimeo.com/12345":                    "https://player.vimeo.com/video/12345",
		"https://example.com/clip.mp4":               "", // direct file stays a <video>
		"https://www.youtube.com/watch?list=novideo": "", // no v param
	}
	for in, want := range cases {
		if got := videoEmbedURL(in); got != want {
			t.Errorf("videoEmbedURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmitSlideBlock_Video(t *testing.T) {
	// A YouTube URL becomes an iframe player.
	if got := emitSlideBlock("video", "https://youtu.be/xyz", "", nil); !strings.Contains(got, `<iframe src="https://www.youtube-nocookie.com/embed/xyz"`) {
		t.Errorf("youtube video should embed as iframe; got %q", got)
	}
	// A direct file stays a <video> element.
	got := emitSlideBlock("video", "https://example.com/a.mp4", "", nil)
	if !strings.Contains(got, "<video ") || !strings.Contains(got, `src="https://example.com/a.mp4"`) {
		t.Errorf("direct file should be a <video>; got %q", got)
	}
}
