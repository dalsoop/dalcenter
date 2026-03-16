package validate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValid(t *testing.T) {
	root := t.TempDir()
	specPath := filepath.Join(root, "dal.spec.cue")
	manifestDir := filepath.Join(root, "repo", ".dalfactory")
	manifestPath := filepath.Join(manifestDir, "dal.cue")

	if err := os.WriteFile(specPath, []byte(`package dalforge
#SemVer: =~"^[0-9]+\\.[0-9]+\\.[0-9]+$"
#DalID: =~"^DAL:[A-Z][A-Z0-9_]+:[a-f0-9]{8}$"
#CategoryID: =~"^[A-Z][A-Z0-9_]+$"
#ContainerSpec: {base!: string, agents!: [string]: _}
#BuildSpec: {language!: string, entry!: string, output!: string}
#DalTemplate: {
	schema_version!: #SemVer
	name!: string
	container!: #ContainerSpec
	build?: #BuildSpec
}
#DalFactory: {
	schema_version!: #SemVer
	dal!: {
		id!: #DalID
		name!: string
		version!: #SemVer
		category!: #CategoryID
	}
	templates!: [string]: #DalTemplate
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema_version: "1.0.0"
dal: {
	id:       "DAL:CLI:639f26a9"
	name:     "dalcli-agent-bridge"
	version:  "0.1.0"
	category: "CLI"
}
templates: default: {
	schema_version: "1.0.0"
	name:           "default"
	container: {
		base:   "ubuntu:24.04"
		agents: {}
	}
	build: {
		language: "shell"
		entry:    "bin/claudebridge"
		output:   "bin/claudebridge"
	}
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := Manifest(specPath, manifestPath)
	if err != nil {
		t.Fatalf("Manifest returned error: %v", err)
	}
	if got.Path != manifestPath {
		t.Fatalf("unexpected path: %s", got.Path)
	}
}

func TestManifestInvalid(t *testing.T) {
	root := t.TempDir()
	specPath := filepath.Join(root, "dal.spec.cue")
	manifestPath := filepath.Join(root, "dal.cue")

	if err := os.WriteFile(specPath, []byte(`package dalforge
#SemVer: =~"^[0-9]+\\.[0-9]+\\.[0-9]+$"
#DalFactory: {schema_version!: #SemVer}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema_version: "bad"`), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Manifest(specPath, manifestPath); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestResolveManifestPath(t *testing.T) {
	root := t.TempDir()
	manifestDir := filepath.Join(root, ".dalfactory")
	manifestPath := filepath.Join(manifestDir, "dal.cue")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("schema_version: \"1.0.0\""), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveManifestPath(root)
	if err != nil {
		t.Fatalf("ResolveManifestPath returned error: %v", err)
	}
	if got != manifestPath {
		t.Fatalf("unexpected path: %s", got)
	}
}
