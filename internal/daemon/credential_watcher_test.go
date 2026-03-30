package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsApproachingExpiry_ClaudeNear(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cred.json")
	// Expires in 30 minutes — within 1h threshold
	exp := time.Now().Add(30 * time.Minute).UnixMilli()
	os.WriteFile(f, []byte(fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, exp)), 0600)

	approaching, err := isApproachingExpiry(f, time.Hour)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !approaching {
		t.Fatal("should be approaching expiry")
	}
}

func TestIsApproachingExpiry_ClaudeFar(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cred.json")
	// Expires in 5 hours — not within 1h threshold
	exp := time.Now().Add(5 * time.Hour).UnixMilli()
	os.WriteFile(f, []byte(fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, exp)), 0600)

	approaching, err := isApproachingExpiry(f, time.Hour)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if approaching {
		t.Fatal("should not be approaching expiry")
	}
}

func TestIsApproachingExpiry_CodexNear(t *testing.T) {
	f := filepath.Join(t.TempDir(), "auth.json")
	exp := time.Now().Add(30 * time.Minute).Format(time.RFC3339)
	os.WriteFile(f, []byte(fmt.Sprintf(`{"tokens":{"expires_at":"%s"}}`, exp)), 0600)

	approaching, err := isApproachingExpiry(f, time.Hour)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !approaching {
		t.Fatal("should be approaching expiry")
	}
}

func TestIsApproachingExpiry_MissingFile(t *testing.T) {
	_, err := isApproachingExpiry("/nonexistent", time.Hour)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsApproachingExpiry_UnknownFormat(t *testing.T) {
	f := filepath.Join(t.TempDir(), "unknown.json")
	os.WriteFile(f, []byte(`{"other":"data"}`), 0600)

	approaching, err := isApproachingExpiry(f, time.Hour)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if approaching {
		t.Fatal("unknown format should not be approaching")
	}
}

func TestRefreshCredential_UnknownPlayer(t *testing.T) {
	// Should not panic
	refreshCredential("unknown-player")
}

func TestCheckAndRefresh_EmptyPaths(t *testing.T) {
	// Should not panic
	checkAndRefresh(&Daemon{credSyncLast: newCredentialSyncMap()}, map[string]string{})
}

func TestCheckAndRefresh_MissingFiles(t *testing.T) {
	// Should not panic
	checkAndRefresh(&Daemon{credSyncLast: newCredentialSyncMap()}, map[string]string{
		"claude": "/nonexistent/cred.json",
		"codex":  "/nonexistent/auth.json",
	})
}

func TestApplyGitCredentials_NewerGitUpdatesActive(t *testing.T) {
	dir := t.TempDir()

	// Active credential file (older)
	activePath := filepath.Join(dir, "active.json")
	os.WriteFile(activePath, []byte(`{"claudeAiOauth":{"expiresAt":1000}}`), 0600)
	// Make active file older
	past := time.Now().Add(-1 * time.Hour)
	os.Chtimes(activePath, past, past)

	// Git credential file (newer, different content)
	gitPath := filepath.Join(dir, "git.json")
	os.WriteFile(gitPath, []byte(`{"claudeAiOauth":{"expiresAt":9999}}`), 0600)

	gitFiles := map[string]string{"claude": gitPath}
	credPaths := map[string]string{"claude": activePath}

	applyGitCredentials(gitFiles, credPaths)

	data, _ := os.ReadFile(activePath)
	if string(data) != `{"claudeAiOauth":{"expiresAt":9999}}` {
		t.Fatalf("active file not updated, got: %s", string(data))
	}
}

func TestApplyGitCredentials_OlderGitSkips(t *testing.T) {
	dir := t.TempDir()

	// Active credential file (newer)
	activePath := filepath.Join(dir, "active.json")
	os.WriteFile(activePath, []byte(`{"claudeAiOauth":{"expiresAt":9999}}`), 0600)

	// Git credential file (older)
	gitPath := filepath.Join(dir, "git.json")
	os.WriteFile(gitPath, []byte(`{"claudeAiOauth":{"expiresAt":1000}}`), 0600)
	past := time.Now().Add(-1 * time.Hour)
	os.Chtimes(gitPath, past, past)

	gitFiles := map[string]string{"claude": gitPath}
	credPaths := map[string]string{"claude": activePath}

	applyGitCredentials(gitFiles, credPaths)

	data, _ := os.ReadFile(activePath)
	if string(data) != `{"claudeAiOauth":{"expiresAt":9999}}` {
		t.Fatalf("active file should not be changed, got: %s", string(data))
	}
}

func TestApplyGitCredentials_MissingActiveCreatesFile(t *testing.T) {
	dir := t.TempDir()

	activePath := filepath.Join(dir, "active.json")
	// Active file doesn't exist

	gitPath := filepath.Join(dir, "git.json")
	os.WriteFile(gitPath, []byte(`{"claudeAiOauth":{"expiresAt":5555}}`), 0600)

	gitFiles := map[string]string{"claude": gitPath}
	credPaths := map[string]string{"claude": activePath}

	applyGitCredentials(gitFiles, credPaths)

	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("active file should be created: %v", err)
	}
	if string(data) != `{"claudeAiOauth":{"expiresAt":5555}}` {
		t.Fatalf("wrong content: %s", string(data))
	}
}

func TestApplyGitCredentials_TooSmallFileSkips(t *testing.T) {
	dir := t.TempDir()

	activePath := filepath.Join(dir, "active.json")
	os.WriteFile(activePath, []byte(`{"old":1}`), 0600)
	past := time.Now().Add(-1 * time.Hour)
	os.Chtimes(activePath, past, past)

	gitPath := filepath.Join(dir, "git.json")
	os.WriteFile(gitPath, []byte(`{}`), 0600) // < 10 bytes

	gitFiles := map[string]string{"claude": gitPath}
	credPaths := map[string]string{"claude": activePath}

	applyGitCredentials(gitFiles, credPaths)

	data, _ := os.ReadFile(activePath)
	if string(data) != `{"old":1}` {
		t.Fatalf("active file should not be changed for small git file, got: %s", string(data))
	}
}

func TestApplyGitCredentials_MissingGitFileSkips(t *testing.T) {
	dir := t.TempDir()

	activePath := filepath.Join(dir, "active.json")
	os.WriteFile(activePath, []byte(`{"keep":"this"}`), 0600)

	gitFiles := map[string]string{"claude": filepath.Join(dir, "nonexistent.json")}
	credPaths := map[string]string{"claude": activePath}

	applyGitCredentials(gitFiles, credPaths)

	data, _ := os.ReadFile(activePath)
	if string(data) != `{"keep":"this"}` {
		t.Fatalf("active file should not be changed, got: %s", string(data))
	}
}

func TestApplyGitCredentials_UnmatchedPlayerSkips(t *testing.T) {
	dir := t.TempDir()

	gitPath := filepath.Join(dir, "git.json")
	os.WriteFile(gitPath, []byte(`{"data":"new"}`), 0600)

	// Git has "claude" but credPaths only has "codex"
	gitFiles := map[string]string{"claude": gitPath}
	credPaths := map[string]string{"codex": filepath.Join(dir, "codex.json")}

	// Should not panic
	applyGitCredentials(gitFiles, credPaths)
}

func TestPullCredentialsFromGit_NoEnvVar(t *testing.T) {
	orig := os.Getenv(credGitRepoEnv)
	os.Unsetenv(credGitRepoEnv)
	defer func() {
		if orig != "" {
			os.Setenv(credGitRepoEnv, orig)
		}
	}()
	// Should return immediately without panic
	pullCredentialsFromGit(map[string]string{"claude": "/tmp/test"})
}

func TestPullCredentialsFromGit_NoGitDir(t *testing.T) {
	dir := t.TempDir()
	os.Setenv(credGitRepoEnv, dir)
	defer os.Unsetenv(credGitRepoEnv)
	// No .git dir — should return without panic
	pullCredentialsFromGit(map[string]string{"claude": "/tmp/test"})
}
