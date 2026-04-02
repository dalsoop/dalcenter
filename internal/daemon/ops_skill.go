package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dalsoop/dalcenter/internal/opsskill"
)

// opsGatewayURL returns the LXC 101 ops gateway URL, or empty if not configured.
func opsGatewayURL() string {
	return strings.TrimRight(os.Getenv("DALCENTER_OPS_GATEWAY_URL"), "/")
}

// opsGatewayToken returns the Bearer token for the ops gateway.
func opsGatewayToken() string {
	return os.Getenv("DALCENTER_OPS_GATEWAY_TOKEN")
}

// handleOpsInvoke proxies an ops skill request to LXC 101.
func (d *Daemon) handleOpsInvoke(w http.ResponseWriter, r *http.Request) {
	gwURL := opsGatewayURL()
	if gwURL == "" {
		http.Error(w, "ops gateway not configured (DALCENTER_OPS_GATEWAY_URL)", http.StatusServiceUnavailable)
		return
	}

	var req opsskill.InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Skill == "" {
		http.Error(w, "skill is required", http.StatusBadRequest)
		return
	}

	endpoint, ok := opsskill.ValidSkills[req.Skill]
	if !ok {
		respondJSON(w, http.StatusBadRequest, opsskill.InvokeResponse{
			OK:    false,
			Skill: req.Skill,
			Error: fmt.Sprintf("unknown skill: %s", req.Skill),
		})
		return
	}

	// Validate required params
	if err := validateSkillParams(req.Skill, req.Params); err != nil {
		respondJSON(w, http.StatusBadRequest, opsskill.InvokeResponse{
			OK:    false,
			Skill: req.Skill,
			Error: err.Error(),
		})
		return
	}

	// Forward to LXC 101
	body, err := json.Marshal(req.Params)
	if err != nil {
		http.Error(w, "failed to marshal params", http.StatusInternalServerError)
		return
	}

	targetURL := gwURL + endpoint
	gwReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create gateway request: %v", err), http.StatusInternalServerError)
		return
	}
	gwReq.Header.Set("Content-Type", "application/json")
	if token := opsGatewayToken(); token != "" {
		gwReq.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	gwResp, err := client.Do(gwReq)
	if err != nil {
		log.Printf("[ops-skill] gateway error for %s: %v", req.Skill, err)
		respondJSON(w, http.StatusBadGateway, opsskill.InvokeResponse{
			OK:    false,
			Skill: req.Skill,
			Error: fmt.Sprintf("gateway unreachable: %v", err),
		})
		return
	}
	defer gwResp.Body.Close()

	respBody, _ := io.ReadAll(gwResp.Body)

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		result = map[string]any{"raw": string(respBody)}
	}

	if gwResp.StatusCode >= 400 {
		errMsg := string(respBody)
		if e, ok := result["error"]; ok {
			errMsg = fmt.Sprintf("%v", e)
		}
		log.Printf("[ops-skill] %s failed (dal=%s): %s", req.Skill, req.Dal, errMsg)
		respondJSON(w, gwResp.StatusCode, opsskill.InvokeResponse{
			OK:    false,
			Skill: req.Skill,
			Error: errMsg,
		})
		return
	}

	log.Printf("[ops-skill] %s ok (dal=%s)", req.Skill, req.Dal)
	respondJSON(w, http.StatusOK, opsskill.InvokeResponse{
		OK:     true,
		Skill:  req.Skill,
		Result: result,
	})
}

// handleOpsSkills returns the list of available ops skills.
func (d *Daemon) handleOpsSkills(w http.ResponseWriter, _ *http.Request) {
	configured := opsGatewayURL() != ""
	respondJSON(w, http.StatusOK, map[string]any{
		"configured": configured,
		"skills":     opsskill.SkillCatalog,
	})
}

// validateSkillParams checks required parameters for a skill.
func validateSkillParams(skill string, params map[string]any) error {
	for _, info := range opsskill.SkillCatalog {
		if info.Name != skill {
			continue
		}
		for _, req := range info.Required {
			v, ok := params[req]
			if !ok {
				return fmt.Errorf("missing required param: %s", req)
			}
			if s, isStr := v.(string); isStr && s == "" {
				return fmt.Errorf("empty required param: %s", req)
			}
		}
		return nil
	}
	return nil
}
