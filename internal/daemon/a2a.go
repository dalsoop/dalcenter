package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"cuelang.org/go/cue/cuecontext"
)

// A2A protocol types per https://google.github.io/A2A/specification/

type agentCard struct {
	Name            string       `json:"name"`
	Description     string       `json:"description,omitempty"`
	URL             string       `json:"url"`
	Version         string       `json:"version"`
	Capabilities    capabilities `json:"capabilities"`
	Skills          []agentSkill `json:"skills"`
	DefaultInputModes  []string  `json:"defaultInputModes"`
	DefaultOutputModes []string  `json:"defaultOutputModes"`
	Provider        *provider    `json:"provider,omitempty"`
}

type capabilities struct {
	Streaming          bool `json:"streaming"`
	PushNotifications  bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type agentSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type provider struct {
	Organization string `json:"organization"`
}

// A2A JSON-RPC types

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// tasks/send params
type taskSendParams struct {
	ID      string       `json:"id"`
	Message a2aMessage   `json:"message"`
}

type a2aMessage struct {
	Role  string   `json:"role"`
	Parts []a2aPart `json:"parts"`
}

type a2aPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type a2aTask struct {
	ID       string      `json:"id"`
	Status   a2aStatus   `json:"status"`
	Artifacts []a2aArtifact `json:"artifacts,omitempty"`
}

type a2aStatus struct {
	State   string     `json:"state"` // "completed", "failed", "working"
	Message *a2aMessage `json:"message,omitempty"`
}

type a2aArtifact struct {
	Parts []a2aPart `json:"parts"`
}

// handleAgentCard serves GET /.well-known/agent-card.json
// Reads dal.spec.cue from localdalRoot, converts to JSON, wraps in A2A agent card.
func (d *Daemon) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	specPath := fmt.Sprintf("%s/../dal.spec.cue", d.localdalRoot)

	// Try localdalRoot parent (repo root), then fallback to serviceRepo
	data, err := os.ReadFile(specPath)
	if err != nil && d.serviceRepo != "" {
		specPath = fmt.Sprintf("%s/dal.spec.cue", d.serviceRepo)
		data, err = os.ReadFile(specPath)
	}
	if err != nil {
		http.Error(w, "dal.spec.cue not found", http.StatusNotFound)
		return
	}

	// Parse CUE to JSON
	ctx := cuecontext.New()
	val := ctx.CompileBytes(data)
	if val.Err() != nil {
		http.Error(w, fmt.Sprintf("cue parse error: %v", val.Err()), http.StatusInternalServerError)
		return
	}
	cueJSON, err := val.MarshalJSON()
	if err != nil {
		http.Error(w, fmt.Sprintf("cue to json error: %v", err), http.StatusInternalServerError)
		return
	}

	// Build agent card with CUE spec embedded
	var specData any
	json.Unmarshal(cueJSON, &specData)

	// Collect running dal skills
	d.mu.RLock()
	var skills []agentSkill
	for name, c := range d.containers {
		skills = append(skills, agentSkill{
			ID:          name,
			Name:        name,
			Description: fmt.Sprintf("%s agent (%s/%s)", c.Role, c.Player, c.DalName),
		})
	}
	d.mu.RUnlock()

	card := agentCard{
		Name:        "dalcenter",
		Description: "dalcenter — multi-agent orchestration daemon",
		URL:         fmt.Sprintf("http://%s", d.addr),
		Version:     "1.0.0",
		Capabilities: capabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: false,
		},
		Skills:             skills,
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Provider: &provider{
			Organization: "dalsoop",
		},
	}

	resp := map[string]any{
		"agent_card": card,
		"spec":       specData,
	}

	respondJSON(w, http.StatusOK, resp)
}

// handleA2ARPC handles POST /rpc — A2A JSON-RPC endpoint.
// Supports tasks/send method by wrapping the existing /api/task handler.
func (d *Daemon) handleA2ARPC(w http.ResponseWriter, r *http.Request) {
	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}

	if req.JSONRPC != "2.0" {
		respondJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32600, Message: "invalid request: jsonrpc must be 2.0"},
		})
		return
	}

	switch req.Method {
	case "tasks/send":
		d.handleTasksSend(w, req)
	default:
		respondJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		})
	}
}

// handleTasksSend processes A2A tasks/send by mapping to the internal task system.
func (d *Daemon) handleTasksSend(w http.ResponseWriter, req jsonRPCRequest) {
	var params taskSendParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		respondJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "invalid params: " + err.Error()},
		})
		return
	}

	// Extract text from message parts
	var taskText string
	for _, p := range params.Message.Parts {
		if p.Type == "text" && p.Text != "" {
			taskText = p.Text
			break
		}
	}
	if taskText == "" {
		respondJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "no text part in message"},
		})
		return
	}

	// Find target dal: use leader if available, else first running container
	d.mu.RLock()
	var target *Container
	for _, c := range d.containers {
		if c.Role == "leader" && c.Status == "running" {
			target = c
			break
		}
	}
	if target == nil {
		for _, c := range d.containers {
			if c.Status == "running" {
				target = c
				break
			}
		}
	}
	d.mu.RUnlock()

	if target == nil {
		respondJSON(w, http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32000, Message: "no running dal available"},
		})
		return
	}

	log.Printf("[a2a] tasks/send → %s: %s", target.DalName, truncateStr(taskText, 80))

	// Execute task via internal task system
	tr := d.tasks.New(target.DalName, taskText)
	d.execTaskInContainer(target, tr)

	// Map taskResult to A2A task response
	state := "completed"
	if tr.Status == "failed" {
		state = "failed"
	}

	task := a2aTask{
		ID: func() string {
			if params.ID != "" {
				return params.ID
			}
			return tr.ID
		}(),
		Status: a2aStatus{
			State: state,
		},
	}

	// Include output as artifact
	output := tr.Output
	if tr.Error != "" && output == "" {
		output = tr.Error
	}
	if output != "" {
		task.Artifacts = []a2aArtifact{
			{Parts: []a2aPart{{Type: "text", Text: output}}},
		}
	}

	// Include status message for failures
	if state == "failed" && tr.Error != "" {
		task.Status.Message = &a2aMessage{
			Role:  "agent",
			Parts: []a2aPart{{Type: "text", Text: tr.Error}},
		}
	}

	respondJSON(w, http.StatusOK, jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  task,
	})
}
