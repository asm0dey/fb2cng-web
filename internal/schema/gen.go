package schema

// Regenerate options.json from a real fbc binary (run from this directory):
//   FBC_BIN=/path/to/fbc go generate ./...
// schemagen prints a DRIFT report (keys added/removed/retyped) so the maintainer
// knows what to re-annotate after an fbc version bump.
//go:generate go run ../../cmd/schemagen -out options.json
