package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dalsoop/dalcenter/internal/paths"
)

// startMatterbridge starts matterbridge as a child process.
// Returns nil if binary not found or config not present (non-fatal).
func startMatterbridge(ctx context.Context, confPath string) (*exec.Cmd, error) {
	if confPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(confPath); err != nil {
		log.Printf("[matterbridge] config not found: %s (skipping)", confPath)
		return nil, nil
	}

	bin, err := exec.LookPath("matterbridge")
	if err != nil {
		log.Printf("[matterbridge] binary not found (skipping)")
		return nil, nil
	}

	if matterbridgeAlreadyRunning() {
		log.Printf("[matterbridge] existing instance detected, skipping")
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, bin, "-conf", confPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	time.Sleep(2 * time.Second)
	log.Printf("[matterbridge] started (pid=%d, conf=%s)", cmd.Process.Pid, confPath)

	return cmd, nil
}

func matterbridgeAlreadyRunning() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:" + DefaultBridgePort, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// parseBridgePort reads the API BindAddress port from a matterbridge TOML config.
func parseBridgePort(confPath string) string {
	data, err := os.ReadFile(confPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "BindAddress") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				port := strings.Trim(parts[len(parts)-1], "\" ")
				return port
			}
		}
	}
	return ""
}

// mmPost posts a message directly to MM API, bypassing matterbridge.
// This avoids the self-skip issue where matterbridge ignores its own messages.
func (d *Daemon) mmPost(text string) error {
	mmURL := os.Getenv("DALCENTER_MM_URL")
	mmToken := os.Getenv("DALCENTER_MM_TOKEN")
	if mmURL == "" || mmToken == "" {
		return fmt.Errorf("DALCENTER_MM_URL or DALCENTER_MM_TOKEN not set")
	}

	// Resolve channel ID from matterbridge config
	channelID := d.resolveMMChannelID(mmURL, mmToken)
	if channelID == "" {
		return fmt.Errorf("could not resolve MM channel ID")
	}

	body := fmt.Sprintf(`{"channel_id":%q,"message":%q}`, channelID, text)
	req, _ := http.NewRequest("POST", strings.TrimRight(mmURL, "/")+"/api/v4/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mmToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mm post %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// parseBridgeToken reads the MM bot Token from a matterbridge TOML config.
// It looks for lines like: Token = "xxx" under [mattermost.*] sections.
func parseBridgeToken(confPath string) string {
	data, err := os.ReadFile(confPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Token") && strings.Contains(line, "=") {
			// Extract value: Token = "xxx" or Token="xxx"
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, "\"")
				if val != "" {
					return val
				}
			}
		}
	}
	return ""
}

// bridgeTokenEntry holds team name and its bridge token for duplicate detection.
type bridgeTokenEntry struct {
	Team    string
	Token   string
	ConfPath string
}

// CheckBridgeTokens scans /etc/dalcenter/*.env for DALCENTER_BRIDGE_CONF paths,
// extracts the MM bot token from each, and returns duplicate groups.
// Returns nil if no duplicates found.
func CheckBridgeTokens() map[string][]string {
	configDir := paths.ConfigDir()
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil
	}

	var tokens []bridgeTokenEntry
	for _, e := range entries {
		if e.Name() == "common.env" || !strings.HasSuffix(e.Name(), ".env") {
			continue
		}
		teamName := strings.TrimSuffix(e.Name(), ".env")
		data, err := os.ReadFile(filepath.Join(configDir, e.Name()))
		if err != nil {
			continue
		}

		// Look for DALCENTER_BRIDGE_CONF in the env file
		confPath := ""
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "DALCENTER_BRIDGE_CONF=") {
				confPath = strings.TrimPrefix(line, "DALCENTER_BRIDGE_CONF=")
				break
			}
		}
		if confPath == "" {
			// Try default path: /etc/dalcenter/<team>.matterbridge.toml
			defaultConf := filepath.Join(configDir, teamName+".matterbridge.toml")
			if _, err := os.Stat(defaultConf); err == nil {
				confPath = defaultConf
			}
		}
		if confPath == "" {
			continue
		}

		token := parseBridgeToken(confPath)
		if token != "" {
			tokens = append(tokens, bridgeTokenEntry{
				Team:     teamName,
				Token:    token,
				ConfPath: confPath,
			})
		}
	}

	// Group by token to find duplicates
	groups := make(map[string][]string)
	for _, t := range tokens {
		groups[t.Token] = append(groups[t.Token], t.Team)
	}

	// Filter to only duplicates
	dupes := make(map[string][]string)
	for token, teams := range groups {
		if len(teams) > 1 {
			dupes[token] = teams
		}
	}
	if len(dupes) == 0 {
		return nil
	}
	return dupes
}

// resolveMMChannelID finds the MM channel ID from the matterbridge config.
func (d *Daemon) resolveMMChannelID(mmURL, mmToken string) string {
	// Parse channel name from matterbridge config
	channelName := ""
	if d.bridgeConf != "" {
		data, err := os.ReadFile(d.bridgeConf)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "channel = ") && !strings.Contains(line, "api") {
					channelName = strings.Trim(strings.TrimPrefix(line, "channel = "), "\"")
					break
				}
			}
		}
	}
	if channelName == "" {
		// Fallback: use repo name
		channelName = filepath.Base(d.serviceRepo)
	}

	// Resolve team ID first
	mmTeam := os.Getenv("DALCENTER_MM_TEAM")
	if mmTeam == "" {
		mmTeam = "dalsoop"
	}

	url := fmt.Sprintf("%s/api/v4/teams/name/%s/channels/name/%s", strings.TrimRight(mmURL, "/"), mmTeam, channelName)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+mmToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID
}
