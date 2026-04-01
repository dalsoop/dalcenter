package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverTeams_FromEnv(t *testing.T) {
	t.Setenv("DALCENTER_OPS_TEAMS", "team1=http://localhost:11190,team2=http://localhost:11191")
	teams := discoverTeams()

	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if teams["team1"] != "http://localhost:11190" {
		t.Fatalf("team1 = %q, want http://localhost:11190", teams["team1"])
	}
	if teams["team2"] != "http://localhost:11191" {
		t.Fatalf("team2 = %q, want http://localhost:11191", teams["team2"])
	}
}

func TestDiscoverTeams_FromEnvFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DALCENTER_CONFIG_DIR", dir)

	// Write common.env with host IP
	os.WriteFile(filepath.Join(dir, "common.env"), []byte("DALCENTER_HOST_IP=10.0.0.1\n"), 0644)

	// Write team env files
	os.WriteFile(filepath.Join(dir, "dalcenter.env"), []byte("DALCENTER_PORT=11190\n"), 0644)
	os.WriteFile(filepath.Join(dir, "veilkey.env"), []byte("DALCENTER_PORT=11191\n"), 0644)

	teams := discoverTeams()

	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if teams["dalcenter"] != "http://10.0.0.1:11190" {
		t.Fatalf("dalcenter = %q, want http://10.0.0.1:11190", teams["dalcenter"])
	}
	if teams["veilkey"] != "http://10.0.0.1:11191" {
		t.Fatalf("veilkey = %q, want http://10.0.0.1:11191", teams["veilkey"])
	}
}

func TestDiscoverTeams_ExplicitOverridesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DALCENTER_CONFIG_DIR", dir)
	t.Setenv("DALCENTER_OPS_TEAMS", "only=http://localhost:9999")

	// Even if env files exist, explicit env takes priority
	os.WriteFile(filepath.Join(dir, "ignored.env"), []byte("DALCENTER_PORT=11190\n"), 0644)

	teams := discoverTeams()
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if teams["only"] != "http://localhost:9999" {
		t.Fatalf("only = %q, want http://localhost:9999", teams["only"])
	}
}

func TestFetchTeamHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(opsHealthResponse{
			Status:       "ok",
			DalsRunning:  3,
			LeaderStatus: "running",
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	health, err := fetchTeamHealth(client, srv.URL)
	if err != nil {
		t.Fatalf("fetchTeamHealth: %v", err)
	}
	if health.DalsRunning != 3 {
		t.Fatalf("DalsRunning = %d, want 3", health.DalsRunning)
	}
	if health.LeaderStatus != "running" {
		t.Fatalf("LeaderStatus = %q, want running", health.LeaderStatus)
	}
}

func TestFetchTeamHealth_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := fetchTeamHealth(client, srv.URL)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchTeamIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/issues" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"issues": []map[string]any{
				{
					"number":      42,
					"title":       "test issue",
					"status":      "dispatched",
					"detected_at": time.Now().Add(-3 * time.Hour).Format(time.RFC3339),
				},
			},
			"total": 1,
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	issues, err := fetchTeamIssues(client, srv.URL)
	if err != nil {
		t.Fatalf("fetchTeamIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Number != 42 {
		t.Fatalf("issue number = %d, want 42", issues[0].Number)
	}
}

func TestWakeTeamLeader(t *testing.T) {
	waked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/wake/leader" && r.Method == "POST" {
			waked = true
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		// Bridge post for alert
		if r.URL.Path == "/api/message" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := &Daemon{
		bridgeURL:   srv.URL,
		serviceRepo: "/tmp/test-repo",
	}

	th := &teamHealth{Name: "test-team", URL: srv.URL}
	client := &http.Client{Timeout: 5 * time.Second}

	if err := d.wakeTeamLeader(client, th); err != nil {
		t.Fatalf("wakeTeamLeader: %v", err)
	}
	if !waked {
		t.Fatal("expected wake request to be sent")
	}
}

func TestCheckAllTeams_HealthyTeam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			json.NewEncoder(w).Encode(opsHealthResponse{
				Status:       "ok",
				DalsRunning:  2,
				LeaderStatus: "running",
			})
		case "/api/issues":
			json.NewEncoder(w).Encode(map[string]any{"issues": []any{}, "total": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := &Daemon{
		serviceRepo: "/tmp/test-repo",
	}

	healthMap := map[string]*teamHealth{
		"test": {Name: "test", URL: srv.URL, Status: "healthy"},
	}

	d.checkAllTeams(nil, healthMap)

	if healthMap["test"].Status != "healthy" {
		t.Fatalf("status = %q, want healthy", healthMap["test"].Status)
	}
	if healthMap["test"].DalsRunning != 2 {
		t.Fatalf("DalsRunning = %d, want 2", healthMap["test"].DalsRunning)
	}
}

func TestCheckAllTeams_EmptyTeam(t *testing.T) {
	waked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			json.NewEncoder(w).Encode(opsHealthResponse{
				Status:       "ok",
				DalsRunning:  0,
				LeaderStatus: "sleeping",
			})
		case "/api/wake/leader":
			waked = true
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/api/message":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := &Daemon{
		bridgeURL:   srv.URL,
		serviceRepo: "/tmp/test-repo",
	}

	healthMap := map[string]*teamHealth{
		"test": {Name: "test", URL: srv.URL, Status: "healthy"},
	}

	d.checkAllTeams(nil, healthMap)

	if !waked {
		t.Fatal("expected leader wake for empty team")
	}
	if healthMap["test"].Status != "recovering" {
		t.Fatalf("status = %q, want recovering", healthMap["test"].Status)
	}
}
