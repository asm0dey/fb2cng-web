package convert

import (
	"strings"
	"testing"
)

func TestBothOptimizeAndCoverInImages(t *testing.T) {
	// Ensure both optimize and cover write to the same images map without collision
	out, err := BuildConfig("", FormOptions{
		ImagesOptimize: ptr(false),
		CoverGenerate:  ptr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "optimize: false") {
		t.Errorf("optimize should be set, got:\n%s", s)
	}
	if !strings.Contains(s, "generate: false") {
		t.Errorf("cover generate should be set, got:\n%s", s)
	}
}
