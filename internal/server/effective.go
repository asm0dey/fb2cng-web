package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"fb2cng-web/internal/convert"
	"gopkg.in/yaml.v3"
)

// computeEffective merges the given sparse overrides over fbc defaults via
// convert.BuildConfig (overrides as rawYAML, form options nil), then asks fbc to
// parse the result to decide validity.
func (s *Server) computeEffective(ctx context.Context, flat map[string]any) effectiveVM {
	vm := effectiveVM{OverrideCount: len(flat)}

	rawYAML, err := yaml.Marshal(unflatten(flat))
	if err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	cfg, err := convert.BuildConfig(string(rawYAML), convert.FormOptions{})
	if err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	vm.YAML = string(cfg)

	dir, err := os.MkdirTemp("", "effective-*")
	if err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	defer os.RemoveAll(dir)
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, cfg, 0o644); err != nil {
		vm.Invalid, vm.Error = true, err.Error()
		return vm
	}
	if err := s.runner.Validate(ctx, cfgPath); err != nil {
		vm.Invalid, vm.Error = true, err.Error()
	} else {
		vm.Valid = true
	}
	return vm
}

func (s *Server) handlePresetEffective(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.renderEffective(w, s.computeEffective(r.Context(), s.collectOverrides(r)))
}

func (s *Server) renderEffective(w http.ResponseWriter, vm effectiveVM) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "_effective", vm); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
