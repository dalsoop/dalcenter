package daemon

import (
	"encoding/json"
	"net/http"
)

func (d *Daemon) handlePipelineInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PaneID string `json:"pane_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PaneID == "" {
		http.Error(w, "pane_id required", 400)
		return
	}
	ch, err := d.pipeline.Init(req.PaneID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, ch)
}

func (d *Daemon) handlePipelineSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PaneID  string `json:"pane_id"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PaneID == "" || req.Message == "" {
		http.Error(w, "pane_id and message required", 400)
		return
	}
	if err := d.pipeline.Send(req.PaneID, req.Message); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, map[string]string{"status": "sent"})
}

func (d *Daemon) handlePipelineReceive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PaneID string `json:"pane_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PaneID == "" {
		http.Error(w, "pane_id required", 400)
		return
	}
	msgs, err := d.pipeline.Receive(req.PaneID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, map[string]any{"messages": msgs})
}

func (d *Daemon) handlePipelineBroadcast(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, "message required", 400)
		return
	}
	if err := d.pipeline.Broadcast(req.Message); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, map[string]string{"status": "broadcast_sent"})
}

func (d *Daemon) handlePipelineSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PaneID  string `json:"pane_id"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PaneID == "" {
		http.Error(w, "pane_id required", 400)
		return
	}
	msgs, err := d.pipeline.Sync(req.PaneID, req.Message)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, map[string]any{"messages": msgs})
}

func (d *Daemon) handlePipelineHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, 200, d.pipeline.Health())
}

func (d *Daemon) handlePipelineList(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, 200, map[string]any{"channels": d.pipeline.List()})
}
