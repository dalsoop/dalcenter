package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBridgeToken_Basic(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "team.matterbridge.toml")
	content := `[mattermost.mybot]
Server = "https://mm.example.com"
Token = "abc123token456"
Team = "dalsoop"

[api]
BindAddress = ":4242"
`
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := parseBridgeToken(conf)
	if got != "abc123token456" {
		t.Errorf("parseBridgeToken = %q, want %q", got, "abc123token456")
	}
}

func TestParseBridgeToken_NoToken(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "empty.toml")
	if err := os.WriteFile(conf, []byte("[api]\nBindAddress = \":4242\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := parseBridgeToken(conf)
	if got != "" {
		t.Errorf("parseBridgeToken = %q, want empty", got)
	}
}

func TestParseBridgeToken_FileNotFound(t *testing.T) {
	got := parseBridgeToken("/nonexistent/path.toml")
	if got != "" {
		t.Errorf("parseBridgeToken = %q, want empty", got)
	}
}

func TestParseBridgeToken_NoSpaces(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "nospace.toml")
	content := `[mattermost.bot]
Token="tokenvalue"
`
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := parseBridgeToken(conf)
	if got != "tokenvalue" {
		t.Errorf("parseBridgeToken = %q, want %q", got, "tokenvalue")
	}
}

func TestCheckBridgeTokens_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DALCENTER_CONFIG_DIR", dir)

	// Create two teams with unique tokens
	writeTeamConfig(t, dir, "team-a", "token-aaa")
	writeTeamConfig(t, dir, "team-b", "token-bbb")

	dupes := CheckBridgeTokens()
	if dupes != nil {
		t.Errorf("expected no duplicates, got %v", dupes)
	}
}

func TestCheckBridgeTokens_WithDuplicates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DALCENTER_CONFIG_DIR", dir)

	// Create two teams sharing the same token
	writeTeamConfig(t, dir, "team-a", "shared-token")
	writeTeamConfig(t, dir, "team-b", "shared-token")
	writeTeamConfig(t, dir, "team-c", "unique-token")

	dupes := CheckBridgeTokens()
	if dupes == nil {
		t.Fatal("expected duplicates, got nil")
	}

	teams, ok := dupes["shared-token"]
	if !ok {
		t.Fatal("expected shared-token in duplicates")
	}
	if len(teams) != 2 {
		t.Errorf("expected 2 teams sharing token, got %d", len(teams))
	}

	// unique-token should not appear
	if _, ok := dupes["unique-token"]; ok {
		t.Error("unique-token should not be in duplicates")
	}
}

func TestCheckBridgeTokens_NoConfigDir(t *testing.T) {
	t.Setenv("DALCENTER_CONFIG_DIR", "/nonexistent")
	dupes := CheckBridgeTokens()
	if dupes != nil {
		t.Errorf("expected nil for missing config dir, got %v", dupes)
	}
}

func TestCheckBridgeTokens_SkipsCommonEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DALCENTER_CONFIG_DIR", dir)

	// common.env should be skipped
	if err := os.WriteFile(filepath.Join(dir, "common.env"), []byte("DALCENTER_HOST_IP=10.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeTeamConfig(t, dir, "team-a", "token-a")

	dupes := CheckBridgeTokens()
	if dupes != nil {
		t.Errorf("expected no duplicates, got %v", dupes)
	}
}

func TestCheckBridgeTokens_DefaultTomlPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DALCENTER_CONFIG_DIR", dir)

	// Team env without DALCENTER_BRIDGE_CONF — should fall back to <team>.matterbridge.toml
	envContent := "DALCENTER_PORT=11190\n"
	if err := os.WriteFile(filepath.Join(dir, "team-x.env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	tomlContent := "[mattermost.bot]\nToken = \"fallback-token\"\n"
	if err := os.WriteFile(filepath.Join(dir, "team-x.matterbridge.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Single team — no duplicates, but verify it was scanned
	dupes := CheckBridgeTokens()
	if dupes != nil {
		t.Errorf("expected no duplicates for single team, got %v", dupes)
	}
}

// writeTeamConfig creates a team env file and corresponding matterbridge TOML with the given token.
func writeTeamConfig(t *testing.T, dir, team, token string) {
	t.Helper()

	tomlPath := filepath.Join(dir, team+".matterbridge.toml")
	tomlContent := "[mattermost.bot]\nToken = \"" + token + "\"\n"
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	envContent := "DALCENTER_PORT=11190\nDALCENTER_BRIDGE_CONF=" + tomlPath + "\n"
	if err := os.WriteFile(filepath.Join(dir, team+".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}
}
