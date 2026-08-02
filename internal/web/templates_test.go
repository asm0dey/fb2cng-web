package web

import (
	"strings"
	"testing"
)

func TestTemplatesParse(t *testing.T) {
	tpl, err := Templates()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"base", "convert", "convert_card"} {
		if tpl.Lookup(name) == nil {
			t.Fatalf("template %q not defined", name)
		}
	}
}

func TestAppCSSCarriesTokens(t *testing.T) {
	b, err := FS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, tok := range []string{
		// the 8 tokens
		"--bg", "--bg-sunk", "--line", "--fg", "--fg-muted", "--fg-dim",
		"--accent", "--changed",
		// exact light values from the mockup Tokens table
		"oklch(.99 .002 255)", "oklch(.52 .14 255)", "oklch(.72 .14 75)",
		// exact dark values
		"@media (prefers-color-scheme: dark)", "oklch(.19 .012 255)", "oklch(.66 .13 255)",
		// the two extra dark rules: log pre near-black + success/error tints
		"oklch(.15 .01 255)",
		"[data-theme=",
	} {
		if !strings.Contains(css, tok) {
			t.Errorf("app.css missing %q", tok)
		}
	}
}
