package convert

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Runner converts fb2 inputs via fbc.
type Runner interface {
	// DumpDefaults returns fbc's embedded default configuration as YAML.
	DumpDefaults(ctx context.Context) ([]byte, error)
	// Convert runs fbc on inputPath into destDir and returns the produced
	// output file paths. configPath is optional ("" = use fbc defaults).
	Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error)
	// ConvertLogged behaves like Convert but also captures combined stdout+stderr
	// into logPath (created/truncated). Returns outputs even on fbc failure when
	// partial output exists; err is non-nil on non-zero exit.
	ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) ([]string, error)
}

// FBC shells out to the fbc binary.
type FBC struct{ Bin string }

// New returns an FBC runner using the given binary path.
func New(bin string) *FBC { return &FBC{Bin: bin} }

func (f *FBC) DumpDefaults(ctx context.Context) ([]byte, error) {
	dir, err := os.MkdirTemp("", "fbc-defaults-*")
	if err != nil {
		return nil, fmt.Errorf("fbc dumpconfig: %w", err)
	}
	defer os.RemoveAll(dir)

	out := filepath.Join(dir, "defaults.yaml")
	var errb bytes.Buffer
	cmd := exec.CommandContext(ctx, f.Bin, "dumpconfig", "--default", out)
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return nil, fmt.Errorf("fbc dumpconfig: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("fbc dumpconfig: %w", err)
	}
	return os.ReadFile(out)
}

func (f *FBC) Convert(ctx context.Context, inputPath, format, configPath, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	// -c is a GLOBAL flag and must come before the subcommand.
	args := []string{}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	args = append(args, "convert", "--to", format, "--overwrite", "--nd", inputPath, destDir)

	var errb bytes.Buffer
	cmd := exec.CommandContext(ctx, f.Bin, args...)
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg != "" {
			return nil, fmt.Errorf("conversion failed: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("conversion failed: %w", err)
	}
	return collectOutputs(destDir)
}

func (f *FBC) ConvertLogged(ctx context.Context, inputPath, format, configPath, destDir, logPath string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	logf, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer logf.Close()

	// -c is a GLOBAL flag and must come before the subcommand.
	args := []string{}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	args = append(args, "convert", "--to", format, "--overwrite", "--nd", inputPath, destDir)

	cmd := exec.CommandContext(ctx, f.Bin, args...)
	cmd.Stdout = io.MultiWriter(logf)
	cmd.Stderr = io.MultiWriter(logf)
	runErr := cmd.Run()

	outs, collectErr := collectOutputs(destDir)
	if runErr != nil {
		return outs, fmt.Errorf("conversion failed: %w", runErr)
	}
	if collectErr != nil {
		return nil, collectErr
	}
	return outs, nil
}

// collectOutputs returns regular, non-hidden files under destDir.
func collectOutputs(destDir string) ([]string, error) {
	var outs []string
	err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != destDir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		outs = append(outs, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(outs) == 0 {
		return nil, fmt.Errorf("conversion produced no output")
	}
	sort.Strings(outs)
	return outs, nil
}
