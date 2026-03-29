package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSoftServeAlreadyRunning_FromPIDFile(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("SOFT_SERVE_DATA_PATH", dataDir)

	if err := os.WriteFile(filepath.Join(dataDir, "soft-serve.pid"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	if !softServeAlreadyRunning() {
		t.Fatal("expected existing soft-serve instance from live pid")
	}
}

func TestSoftServeAlreadyRunning_FromPorts(t *testing.T) {
	sshLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ssh: %v", err)
	}
	defer sshLn.Close()

	gitLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen git: %v", err)
	}
	defer gitLn.Close()

	t.Setenv("SOFT_SERVE_SSH_PORT", strconv.Itoa(sshLn.Addr().(*net.TCPAddr).Port))
	t.Setenv("SOFT_SERVE_GIT_PORT", strconv.Itoa(gitLn.Addr().(*net.TCPAddr).Port))
	t.Setenv("SOFT_SERVE_DATA_PATH", t.TempDir())

	if !softServeAlreadyRunning() {
		t.Fatal("expected existing soft-serve instance from listening ports")
	}
}
