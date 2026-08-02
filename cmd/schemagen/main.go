package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func main() {
	dumpPath := flag.String("dump", "", "path to an `fbc dumpconfig --default` YAML; empty runs -fbc")
	docsPath := flag.String("docs", "docs/config.md", "path to config.md for descriptions")
	outPath := flag.String("out", "internal/schema/options.json", "output options.json path")
	fbcBin := flag.String("fbc", os.Getenv("FBC_BIN"), "fbc binary (used when -dump is empty)")
	flag.Parse()

	var dumpBytes []byte
	var err error
	if *dumpPath != "" {
		dumpBytes, err = os.ReadFile(*dumpPath)
	} else {
		dumpBytes, err = runDump(*fbcBin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read dump:", err)
		os.Exit(1)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(dumpBytes, &root); err != nil {
		fmt.Fprintln(os.Stderr, "parse dump:", err)
		os.Exit(1)
	}
	opts := buildOptions(&root)

	if md, derr := os.ReadFile(*docsPath); derr == nil {
		applyDescriptions(opts, parseDescriptions(string(md)))
	} else {
		fmt.Fprintf(os.Stderr, "note: %s not read (%v); descriptions left blank\n", *docsPath, derr)
	}

	d := computeDrift(loadExisting(*outPath), opts)
	fmt.Fprintf(os.Stderr, "DRIFT added=%d removed=%d retyped=%d\n", len(d.Added), len(d.Removed), len(d.Retyped))
	for _, k := range d.Added {
		fmt.Fprintln(os.Stderr, "  + "+k)
	}
	for _, k := range d.Removed {
		fmt.Fprintln(os.Stderr, "  - "+k)
	}
	for _, k := range d.Retyped {
		fmt.Fprintln(os.Stderr, "  ~ "+k)
	}

	if err := writeOptions(*outPath, opts); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %d options to %s\n", len(opts), *outPath)
}

// runDump invokes `fbc dumpconfig --default <tmp>` and returns the YAML bytes.
func runDump(bin string) ([]byte, error) {
	if bin == "" {
		return nil, fmt.Errorf("no -dump path and no -fbc/FBC_BIN binary")
	}
	dir, err := os.MkdirTemp("", "schemagen-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "defaults.yaml")
	if err := exec.Command(bin, "dumpconfig", "--default", out).Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}
