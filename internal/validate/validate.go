package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// Result describes one manifest validation outcome.
type Result struct {
	Path string
}

// Manifest validates a .dalfactory manifest against dal.spec.cue.
func Manifest(specPath, manifestPath string) (*Result, error) {
	specAbs, err := filepath.Abs(specPath)
	if err != nil {
		return nil, fmt.Errorf("resolve spec path: %w", err)
	}
	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve manifest path: %w", err)
	}

	manifestData, err := os.ReadFile(manifestAbs)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	overlayFile := filepath.Join("/", "virtual", "manifest.cue")
	overlay := map[string]load.Source{
		overlayFile: load.FromString(wrapManifest(manifestData)),
	}

	cfg := &load.Config{
		Dir:     filepath.Dir(specAbs),
		Overlay: overlay,
	}
	instances := load.Instances([]string{filepath.Base(specAbs), overlayFile}, cfg)
	if len(instances) == 0 {
		return nil, fmt.Errorf("load cue instances: no instances returned")
	}

	ctx := cuecontext.New()
	v := ctx.BuildInstance(instances[0])
	if err := v.Err(); err != nil {
		return nil, fmt.Errorf("build cue instance: %w", err)
	}

	manifest := v.LookupPath(cue.ParsePath("manifest"))
	if !manifest.Exists() {
		return nil, fmt.Errorf("build cue instance: manifest root missing")
	}
	if err := manifest.Validate(cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	return &Result{Path: manifestAbs}, nil
}

// ResolveManifestPath maps a repo dir or file path to the actual dal.cue path.
func ResolveManifestPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return abs, nil
	}

	candidates := []string{
		filepath.Join(abs, ".dalfactory", "dal.cue"),
		filepath.Join(abs, "dal.cue"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no dal.cue found under %s", abs)
}

func wrapManifest(data []byte) string {
	trimmed := strings.TrimSpace(string(data))
	return "package dalforge\n\nmanifest: #DalFactory & {\n" + trimmed + "\n}\n"
}
