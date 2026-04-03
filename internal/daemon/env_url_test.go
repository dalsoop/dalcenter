package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{":11190", "http://localhost:11190"},
		{"0.0.0.0:8080", "http://localhost:8080"},
		{"127.0.0.1:9999", "http://localhost:9999"},
		{"invalid", ""},
	}
	for _, tt := range tests {
		got := buildURL(tt.addr)
		if got != tt.want {
			t.Errorf("buildURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestEnsureEnvLine(t *testing.T) {
	dir := t.TempDir()

	t.Run("create new", func(t *testing.T) {
		f := filepath.Join(dir, "new.env")
		if err := ensureEnvLine(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		if !strings.Contains(string(data), "DALCENTER_URL=http://localhost:11190") {
			t.Fatalf("unexpected content: %s", data)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		f := filepath.Join(dir, "idem.env")
		os.WriteFile(f, []byte("DALCENTER_URL=http://localhost:11190\n"), 0644)
		if err := ensureEnvLine(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		if strings.Count(string(data), "DALCENTER_URL") != 1 {
			t.Fatalf("duplicate entries: %s", data)
		}
	})

	t.Run("update existing", func(t *testing.T) {
		f := filepath.Join(dir, "update.env")
		os.WriteFile(f, []byte("FOO=bar\nDALCENTER_URL=http://localhost:9999\nBAZ=qux\n"), 0644)
		if err := ensureEnvLine(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		content := string(data)
		if !strings.Contains(content, "DALCENTER_URL=http://localhost:11190") {
			t.Fatalf("not updated: %s", content)
		}
		if strings.Contains(content, "9999") {
			t.Fatalf("old value remains: %s", content)
		}
		if !strings.Contains(content, "FOO=bar") || !strings.Contains(content, "BAZ=qux") {
			t.Fatalf("other lines lost: %s", content)
		}
	})

	t.Run("append to existing file", func(t *testing.T) {
		f := filepath.Join(dir, "append.env")
		os.WriteFile(f, []byte("FOO=bar\n"), 0644)
		if err := ensureEnvLine(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		content := string(data)
		if !strings.Contains(content, "FOO=bar") {
			t.Fatalf("original lost: %s", content)
		}
		if !strings.Contains(content, "DALCENTER_URL=http://localhost:11190") {
			t.Fatalf("not appended: %s", content)
		}
	})
}

func TestEnsureBashrcExport(t *testing.T) {
	dir := t.TempDir()

	t.Run("create new", func(t *testing.T) {
		f := filepath.Join(dir, ".bashrc")
		if err := ensureBashrcExport(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		if !strings.Contains(string(data), "export DALCENTER_URL=http://localhost:11190") {
			t.Fatalf("unexpected content: %s", data)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		f := filepath.Join(dir, ".bashrc2")
		os.WriteFile(f, []byte("export DALCENTER_URL=http://localhost:11190\n"), 0644)
		if err := ensureBashrcExport(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		if strings.Count(string(data), "DALCENTER_URL") != 1 {
			t.Fatalf("duplicate entries: %s", data)
		}
	})

	t.Run("update existing", func(t *testing.T) {
		f := filepath.Join(dir, ".bashrc3")
		os.WriteFile(f, []byte("# my bashrc\nexport DALCENTER_URL=http://localhost:9999\n"), 0644)
		if err := ensureBashrcExport(f, "DALCENTER_URL", "http://localhost:11190"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(f)
		content := string(data)
		if !strings.Contains(content, "export DALCENTER_URL=http://localhost:11190") {
			t.Fatalf("not updated: %s", content)
		}
		if strings.Contains(content, "9999") {
			t.Fatalf("old value remains: %s", content)
		}
	})
}
