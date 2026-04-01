package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dalsoop/dalcenter/internal/paths"
)

const (
	opsCheckInterval       = 2 * time.Minute
	opsCheckTimeout        = 10 * time.Second
	opsStaleIssueThreshold = 2 * time.Hour
	opsTellRetries         = 2
)

// teamHealth tracks the health state of a single dalcenter team.
type teamHealth struct {
	Name             string    `json:"name"`
	URL              string    `json:"url"`
	Status           string    `json:"status"` // "healthy", "empty", "leader_down", "unreachable"
	DalsRunning      int       `json:"dals_running"`
	LeaderStatus     string    `json:"leader_status"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	LastCheckAt      time.Time `json:"last_check_at,omitempty"`
	LastHealthyAt    time.Time `json:"last_healthy_at,omitempty"`
}

// opsHealthResponse is the JSON shape returned by /api/health.
type opsHealthResponse struct {
	Status       string `json:"status"`
	DalsRunning  int    `json:"dals_running"`
	LeaderStatus string `json:"leader_status"`
}

// startOpsWatcher periodically polls all dalcenter teams and performs auto-recovery.
// This consolidates #599 (doctor dal) and #583 (boot auto-recovery).
func (d *Daemon) startOpsWatcher(ctx context.Context) {
	if os.Getenv("DALCENTER_OPS_ENABLED") == "0" {
		log.Printf("[ops-watcher] disabled via DALCENTER_OPS_ENABLED=0")
		return
	}

	teams := discoverTeams()
	if len(teams) == 0 {
		log.Printf("[ops-watcher] no teams discovered — skipping")
		return
	}

	log.Printf("[ops-watcher] started (interval=%s, teams=%d)", opsCheckInterval, len(teams))

	// Initial delay to let all daemons finish startup
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	healthMap := make(map[string]*teamHealth, len(teams))
	for name, url := range teams {
		healthMap[name] = &teamHealth{
			Name:   name,
			URL:    url,
			Status: "healthy",
		}
	}

	// Initial check
	d.checkAllTeams(ctx, healthMap)

	ticker := time.NewTicker(opsCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[ops-watcher] stopped")
			return
		case <-ticker.C:
			d.checkAllTeams(ctx, healthMap)
		}
	}
}

// discoverTeams reads /etc/dalcenter/*.env to build team name → URL map.
// Excludes common.env. Returns empty map if no env files found.
func discoverTeams() map[string]string {
	hostIP := "localhost"
	if h := readTeamEnvVar(paths.ConfigDir(), "common.env", "DALCENTER_HOST_IP"); h != "" {
		hostIP = h
	}

	// Also check DALCENTER_OPS_TEAMS env for explicit team list
	if explicit := os.Getenv("DALCENTER_OPS_TEAMS"); explicit != "" {
		teams := make(map[string]string)
		for _, entry := range strings.Split(explicit, ",") {
			entry = strings.TrimSpace(entry)
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) == 2 {
				teams[parts[0]] = strings.TrimRight(parts[1], "/")
			}
		}
		return teams
	}

	entries, err := os.ReadDir(paths.ConfigDir())
	if err != nil {
		return nil
	}

	teams := make(map[string]string)
	for _, e := range entries {
		if e.Name() == "common.env" || !strings.HasSuffix(e.Name(), ".env") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".env")
		data, err := os.ReadFile(filepath.Join(paths.ConfigDir(), e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "DALCENTER_PORT=") {
				port := strings.TrimPrefix(line, "DALCENTER_PORT=")
				teams[name] = fmt.Sprintf("http://%s:%s", hostIP, port)
				break
			}
		}
	}
	return teams
}

// readTeamEnvVar reads a variable from an env file in the config directory.
func readTeamEnvVar(dir, file, key string) string {
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

// checkAllTeams polls every discovered team and takes remediation actions.
func (d *Daemon) checkAllTeams(ctx context.Context, healthMap map[string]*teamHealth) {
	client := &http.Client{Timeout: opsCheckTimeout}

	for _, th := range healthMap {
		th.LastCheckAt = time.Now()

		health, err := fetchTeamHealth(client, th.URL)
		if err != nil {
			th.ConsecutiveFails++
			th.Status = "unreachable"
			log.Printf("[ops-watcher] %s unreachable (%d): %v", th.Name, th.ConsecutiveFails, err)

			if th.ConsecutiveFails >= 3 {
				d.alertOps(fmt.Sprintf(":warning: **팀 %s 응답 없음** — %d회 연속 실패 (%s)", th.Name, th.ConsecutiveFails, th.URL))
			}
			continue
		}

		th.DalsRunning = health.DalsRunning
		th.LeaderStatus = health.LeaderStatus
		th.ConsecutiveFails = 0

		// Check 1: zero containers → auto-start leader
		if health.DalsRunning == 0 {
			th.Status = "empty"
			log.Printf("[ops-watcher] %s has 0 running dals — attempting leader wake", th.Name)
			if err := d.wakeTeamLeader(client, th); err != nil {
				log.Printf("[ops-watcher] %s leader wake failed: %v", th.Name, err)
				d.alertOps(fmt.Sprintf(":rotating_light: **팀 %s leader 시작 실패** — %v", th.Name, err))
			} else {
				log.Printf("[ops-watcher] %s leader wake requested", th.Name)
				th.Status = "recovering"
			}
			continue
		}

		// Check 2: leader not running
		if health.LeaderStatus != "running" && health.LeaderStatus != "not_configured" {
			th.Status = "leader_down"
			log.Printf("[ops-watcher] %s leader status: %s — attempting restart", th.Name, health.LeaderStatus)
			if err := d.wakeTeamLeader(client, th); err != nil {
				log.Printf("[ops-watcher] %s leader restart failed: %v", th.Name, err)
				d.alertOps(fmt.Sprintf(":rotating_light: **팀 %s leader 비정상** — status=%s, 복구 실패", th.Name, health.LeaderStatus))
			}
			continue
		}

		th.Status = "healthy"
		th.LastHealthyAt = time.Now()
	}

	// Check stale dispatched issues across all teams
	d.checkStaleIssues(ctx, client, healthMap)
}

// fetchTeamHealth calls GET /api/health on a team's dalcenter.
func fetchTeamHealth(client *http.Client, baseURL string) (*opsHealthResponse, error) {
	resp, err := client.Get(baseURL + "/api/health")
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var health opsHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &health, nil
}

// wakeTeamLeader sends POST /api/wake/leader to a team's dalcenter.
func (d *Daemon) wakeTeamLeader(client *http.Client, th *teamHealth) error {
	req, err := http.NewRequest(http.MethodPost, th.URL+"/api/wake/leader", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Use the team's token if available, otherwise our own
	if token := readTeamEnvVar(paths.ConfigDir(), th.Name+".env", "DALCENTER_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := os.Getenv("DALCENTER_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("wake failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	d.alertOps(fmt.Sprintf(":arrows_counterclockwise: **팀 %s leader 시작 요청** — 자동 복구 시도", th.Name))
	return nil
}

// checkStaleIssues looks for dispatched issues that haven't been resolved within the threshold.
func (d *Daemon) checkStaleIssues(_ context.Context, client *http.Client, healthMap map[string]*teamHealth) {
	for _, th := range healthMap {
		if th.Status == "unreachable" {
			continue
		}

		issues, err := fetchTeamIssues(client, th.URL)
		if err != nil {
			continue
		}

		now := time.Now()
		for _, issue := range issues {
			if issue.Status != "dispatched" {
				continue
			}
			age := now.Sub(issue.DetectedAt)
			if age > opsStaleIssueThreshold {
				d.alertOps(fmt.Sprintf(
					":clock3: **팀 %s 이슈 #%d 장시간 미처리** — %q (dispatched %s ago)",
					th.Name, issue.Number, issue.Title, age.Truncate(time.Minute)))
			}
		}
	}
}

// fetchTeamIssues calls GET /api/issues on a team's dalcenter.
func fetchTeamIssues(client *http.Client, baseURL string) ([]*trackedIssue, error) {
	resp, err := client.Get(baseURL + "/api/issues")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Issues []*trackedIssue `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Issues, nil
}

// alertOps sends an ops alert via the bridge channel.
func (d *Daemon) alertOps(message string) {
	log.Printf("[ops-watcher] alert: %s", message)
	if d.bridgeURL == "" {
		return
	}
	if err := d.bridgePost(message, "dalcenter-ops"); err != nil {
		log.Printf("[ops-watcher] failed to post alert: %v", err)
	}
}

// tellTeam sends a message to another team's dalcenter via /api/message with retry.
func (d *Daemon) tellTeam(client *http.Client, th *teamHealth, message string) error {
	from := filepath.Base(d.serviceRepo)

	for attempt := 0; attempt <= opsTellRetries; attempt++ {
		body := fmt.Sprintf(`{"from":%q,"message":%q}`, from, message)
		req, err := http.NewRequest(http.MethodPost, th.URL+"/api/message", strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		if token := readTeamEnvVar(paths.ConfigDir(), th.Name+".env", "DALCENTER_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			if attempt < opsTellRetries {
				log.Printf("[ops-watcher] tell %s failed (attempt %d/%d): %v", th.Name, attempt+1, opsTellRetries+1, err)
				time.Sleep(5 * time.Second)
				continue
			}
			return fmt.Errorf("tell %s failed after %d attempts: %w", th.Name, opsTellRetries+1, err)
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			if attempt < opsTellRetries {
				time.Sleep(5 * time.Second)
				continue
			}
			return fmt.Errorf("tell %s failed (%d)", th.Name, resp.StatusCode)
		}

		return nil
	}
	return nil
}
