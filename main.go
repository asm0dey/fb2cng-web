package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"fb2cng-web/internal/auth"
	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/jobs"
	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/schema"
	"fb2cng-web/internal/server"
	"fb2cng-web/internal/web"
)

func main() {
	cfg := config.FromEnv()

	authn, err := auth.New(context.Background(), cfg)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	sch, err := schema.Load()
	if err != nil {
		log.Fatalf("load embedded schema: %v", err)
	}

	if err := os.MkdirAll(cfg.JobsDir, 0o755); err != nil {
		log.Fatalf("jobs dir %s: %v", cfg.JobsDir, err)
	}
	// Fail fast with an actionable message if the preset store dir can't be
	// created, rather than surfacing a cryptic mkdir error on the first write.
	if err := os.MkdirAll(cfg.PresetsDir, 0o755); err != nil {
		log.Fatalf("presets dir %s not writable (set PRESETS_DIR to a writable path): %v", cfg.PresetsDir, err)
	}
	tpl, err := web.Templates()
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}
	jobStore := jobs.NewStore(cfg.JobsDir, cfg.JobsTTL)
	presetStore := presets.NewStore(cfg.PresetsDir)

	// Background sweeper: drop job dirs past their TTL.
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			if n := jobStore.Sweep(time.Now()); n > 0 {
				log.Printf("swept %d expired job(s)", n)
			}
		}
	}()

	srv := server.New(cfg, server.Deps{
		Runner:  convert.New(cfg.FBCBin),
		Static:  web.FS,
		Tpl:     tpl,
		Jobs:    jobStore,
		Presets: presetStore,
		Schema:  sch,
		Auth:    authn,
	})
	log.Printf("fb2cng-web listening on %s (fbc=%s, auth=%s, jobs=%s, ttl=%s)",
		cfg.Addr, cfg.FBCBin, cfg.AuthMode, cfg.JobsDir, cfg.JobsTTL)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
