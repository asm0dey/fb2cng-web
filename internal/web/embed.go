// Package web holds the embedded static frontend and templates.
package web

import (
	"embed"
	"html/template"
	"strings"
)

//go:embed static all:templates
var FS embed.FS

// Templates parses the layout, pages, and partials once. The result exposes named
// templates: "base" (shared chrome), one per page ("convert" here; Plans 2/3 add
// "settings"/"editor"), and partials ("convert_card", …).
//
// base.gohtml dispatches to the page body with {{content .ContentName .}}. Go's
// {{template}} action requires a *constant* name, so dynamic dispatch by field goes
// through this "content" func, which renders the named sub-template into safe HTML.
func Templates() (*template.Template, error) {
	root := template.New("root")
	root.Funcs(template.FuncMap{
		"content": func(name string, data any) (template.HTML, error) {
			var b strings.Builder
			if err := root.ExecuteTemplate(&b, name, data); err != nil {
				return "", err
			}
			return template.HTML(b.String()), nil
		},
	})
	return root.ParseFS(FS, "templates/*.gohtml")
}
