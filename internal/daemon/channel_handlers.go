package daemon

import (
	"encoding/json"
	"net/http"
)

func (d *Daemon) handleChannelCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Purpose string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	ch, err := d.pipeline.CreateNamedChannel(req.Name, req.Purpose)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 201, ch)
}

func (d *Daemon) handleChannelDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	if err := d.pipeline.DeleteNamedChannel(req.Name); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, map[string]string{"status": "deleted", "name": req.Name})
}

func (d *Daemon) handleChannelList(w http.ResponseWriter, r *http.Request) {
	channels, err := d.pipeline.ListTeamChannels()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	respondJSON(w, 200, map[string]any{"channels": channels, "count": len(channels)})
}
