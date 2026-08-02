package schema

import "testing"

const fixtureJSON = `[
  {"key":"version","group":"version","label":"version","kind":"int","default":1},
  {"key":"document.toc_type","group":"document","label":"toc_type","kind":"string","default":"normal"},
  {"key":"document.images.optimize","group":"document","label":"optimize","kind":"bool","default":true}
]`

func TestParseAndGet(t *testing.T) {
	s, err := parse([]byte(fixtureJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Options) != 3 {
		t.Fatalf("want 3 options, got %d", len(s.Options))
	}
	o, ok := s.Get("document.toc_type")
	if !ok || o.Label != "toc_type" || o.Kind != KindString {
		t.Fatalf("Get(document.toc_type) = %+v ok=%v", o, ok)
	}
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get(nope) should be false")
	}
}

func TestGroupsAndInGroup(t *testing.T) {
	s, err := parse([]byte(fixtureJSON))
	if err != nil {
		t.Fatal(err)
	}
	groups := s.Groups()
	if len(groups) != 2 || groups[0] != "version" || groups[1] != "document" {
		t.Fatalf("groups first-seen order wrong: %v", groups)
	}
	doc := s.InGroup("document")
	if len(doc) != 2 || doc[0].Key != "document.toc_type" {
		t.Fatalf("InGroup(document) = %+v", doc)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := parse([]byte("{not json")); err == nil {
		t.Fatal("parse of invalid JSON should error")
	}
}

// TestLoadEmbedded confirms the //go:embed-ed options.json parses. In Task 2 the
// embedded file is the placeholder `[]` (0 options); after Task 4 regenerates it
// this still passes with the real option count.
func TestLoadEmbedded(t *testing.T) {
	if _, err := Load(); err != nil {
		t.Fatalf("Load() embedded options.json: %v", err)
	}
}
