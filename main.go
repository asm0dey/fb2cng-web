package main

import (
	"log"

	"fb2cng-web/internal/config"
)

func main() {
	cfg := config.FromEnv()
	log.Printf("fb2cng-web starting on %s (fbc=%s)", cfg.Addr, cfg.FBCBin)
	// Server wiring is added in Task 5.
}
