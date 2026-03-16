package provision

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dalforge-hub/dalcenter/internal/state"
)

func TestBuildCommandDefaults(t *testing.T) {
	args := BuildCommand(Spec{
		Base:         "ubuntu:24.04",
		InstanceName: "dalcli-agent-coach",
	})

	cmd := strings.Join(args, " ")
	if !strings.Contains(cmd, "create 0") {
		t.Fatalf("expected VMID 0 (auto), got: %s", cmd)
	}
	if !strings.Contains(cmd, "--ostemplate local:vztmpl/ubuntu-2404-standard_amd64.tar.zst") {
		t.Fatalf("expected ubuntu template, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--hostname dalcli-agent-coach") {
		t.Fatalf("expected hostname, got: %s", cmd)
	}
}

func TestBuildCommandWithVMID(t *testing.T) {
	args := BuildCommand(Spec{
		Base:         "debian:12",
		InstanceName: "test-instance",
		VMID:         "211500",
	})

	cmd := strings.Join(args, " ")
	if !strings.Contains(cmd, "create 211500") {
		t.Fatalf("expected VMID 211500, got: %s", cmd)
	}
	if !strings.Contains(cmd, "debian-12-standard") {
		t.Fatalf("expected debian template, got: %s", cmd)
	}
}

func TestBuildCommandCustomTemplate(t *testing.T) {
	args := BuildCommand(Spec{
		Base:         "local:vztmpl/custom.tar.gz",
		InstanceName: "custom",
	})

	cmd := strings.Join(args, " ")
	if !strings.Contains(cmd, "--ostemplate local:vztmpl/custom.tar.gz") {
		t.Fatalf("expected passthrough template, got: %s", cmd)
	}
}

func TestDryRunDoesNotExecute(t *testing.T) {
	result := Provision("/nonexistent", Spec{
		Base:         "ubuntu:24.04",
		InstanceName: "test",
	}, true)

	if result.Error != nil {
		t.Fatalf("dry-run should not error: %v", result.Error)
	}
	if !strings.Contains(result.Command, "pct create") {
		t.Fatalf("expected pct command, got: %s", result.Command)
	}
}

func TestProvisionWithoutPctRecordsError(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	os.MkdirAll(stateDir, 0755)

	// Ensure pct is NOT in a restricted PATH
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	result := Provision(dir, Spec{
		Base:         "ubuntu:24.04",
		InstanceName: "test",
	}, false)

	if result.Error == nil {
		t.Fatal("expected error when pct not found")
	}
	if !strings.Contains(result.Error.Error(), "pct not found") {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// State should record error
	hs, err := state.Read(dir)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if hs.ProvisionStatus != "error" {
		t.Fatalf("expected error status, got %q", hs.ProvisionStatus)
	}
}

func TestSanitizeHostname(t *testing.T) {
	cases := []struct{ in, want string }{
		{"dalcli-agent-coach", "dalcli-agent-coach"},
		{"my_tool.v2", "my-tool-v2"},
		{"a/b/c", "a-b-c"},
	}
	for _, tc := range cases {
		got := sanitizeHostname(tc.in)
		if got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
