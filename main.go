package main

import (
	"log"
	"net/http"

	"fb2cng-web/internal/config"
	"fb2cng-web/internal/convert"
	"fb2cng-web/internal/server"
	"fb2cng-web/internal/web"
)

func main() {
	cfg := config.FromEnv()
	srv := server.New(cfg, convert.New(cfg.FBCBin), web.FS)
	log.Printf("fb2cng-web listening on %s (fbc=%s, auth=%v)", cfg.Addr, cfg.FBCBin, cfg.ForwardAuth)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
