package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/jobs"
	"fb2cng-web/internal/presets"
	"fb2cng-web/internal/server"
	"fb2cng-web/internal/web"
)

func main() {
	cfg := config.FromEnv()

	if err := os.MkdirAll(cfg.JobsDir, 0o755); err != nil {
		log.Fatalf("jobs dir %s: %v", cfg.JobsDir, err)
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

	srv := server.New(cfg, convert.New(cfg.FBCBin), web.FS, tpl, jobStore, presetStore)
	log.Printf("fb2cng-web listening on %s (fbc=%s, auth=%v, jobs=%s, ttl=%s)",
		cfg.Addr, cfg.FBCBin, cfg.ForwardAuth, cfg.JobsDir, cfg.JobsTTL)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
