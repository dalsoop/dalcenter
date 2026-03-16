package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type HealthState struct {
	Status          string `json:"status"`
	HealthExit      int    `json:"health_exit"`
	HealthOutput    string `json:"health_output"`
	CheckedAt       string `json:"checked_at"`
	Pid             int    `json:"pid,omitempty"`
	RunStatus       string `json:"run_status,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	VMID            string `json:"vmid,omitempty"`
	ProvisionStatus string `json:"provision_status,omitempty"` // provisioned, error, ""
	ProvisionedAt   string `json:"provisioned_at,omitempty"`
	ProvisionError  string `json:"provision_error,omitempty"`
	RollbackStatus  string                    `json:"rollback_status,omitempty"`
	AgentStatuses   map[string]string         `json:"agent_statuses,omitempty"` // name -> installed/skipped/error
}

func Path(instanceRoot string) string {
	return filepath.Join(instanceRoot, "state", "state.json")
}

func Write(instanceRoot string, s HealthState) error {
	dir := filepath.Dir(Path(instanceRoot))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(instanceRoot), data, 0644)
}

func Read(instanceRoot string) (*HealthState, error) {
	data, err := os.ReadFile(Path(instanceRoot))
	if err != nil {
		return nil, err
	}
	var s HealthState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func New(status string, exit int, output string) HealthState {
	return HealthState{
		Status:       status,
		HealthExit:   exit,
		HealthOutput: output,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}
