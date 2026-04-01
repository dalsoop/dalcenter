package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Workflow states for the issue lifecycle.
const (
	WorkflowDetected       = "detected"        // issue detected by watcher
	WorkflowDispatched     = "dispatched"       // task dispatched to leader
	WorkflowAnalyzing      = "analyzing"        // leader is analyzing the issue
	WorkflowMemberWoken    = "member_woken"     // member container woken with issue branch
	WorkflowAssigned       = "assigned"         // task assigned to member
	WorkflowWorking        = "working"          // member is working on the task
	WorkflowPRCreated      = "pr_created"       // PR created by member
	WorkflowCompleted      = "completed"        // workflow finished successfully
	WorkflowFailed         = "failed"           // workflow failed (with error)
	WorkflowEscalated      = "escalated"        // escalated due to repeated failures
)

// issueWorkflow tracks the end-to-end lifecycle of a GitHub issue.
type issueWorkflow struct {
	IssueNumber int               `json:"issue_number"`
	IssueTitle  string            `json:"issue_title"`
	IssueURL    string            `json:"issue_url"`
	Author      string            `json:"author"`
	State       string            `json:"state"`
	LeaderTask  string            `json:"leader_task,omitempty"`  // task ID dispatched to leader
	MemberDal   string            `json:"member_dal,omitempty"`   // member assigned to work
	MemberTask  string            `json:"member_task,omitempty"`  // task ID for member work
	PRUrl       string            `json:"pr_url,omitempty"`
	Error       string            `json:"error,omitempty"`
	Retries     int               `json:"retries"`
	Events      []workflowEvent   `json:"events"`
	DetectedAt  time.Time         `json:"detected_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

type workflowEvent struct {
	At      time.Time `json:"at"`
	State   string    `json:"state"`
	Message string    `json:"message"`
}

// workflowStore manages issue workflow state.
type workflowStore struct {
	mu        sync.RWMutex
	workflows map[int]*issueWorkflow // issue number -> workflow
	filePath  string
}

const maxWorkflows = 100

func newWorkflowStore(path string) *workflowStore {
	s := &workflowStore{workflows: make(map[int]*issueWorkflow), filePath: path}
	var items []*issueWorkflow
	if err := loadJSON(path, &items); err == nil {
		for _, wf := range items {
			s.workflows[wf.IssueNumber] = wf
		}
	}
	return s
}

func (s *workflowStore) Get(issueNumber int) *issueWorkflow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workflows[issueNumber]
}

func (s *workflowStore) List() []*issueWorkflow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*issueWorkflow, 0, len(s.workflows))
	for _, wf := range s.workflows {
		result = append(result, wf)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DetectedAt.After(result[j].DetectedAt)
	})
	return result
}

// Create initializes a new workflow for a detected issue.
func (s *workflowStore) Create(issue ghIssue) *issueWorkflow {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	wf := &issueWorkflow{
		IssueNumber: issue.Number,
		IssueTitle:  issue.Title,
		IssueURL:    issue.URL,
		Author:      issue.Author.Login,
		State:       WorkflowDetected,
		DetectedAt:  now,
		UpdatedAt:   now,
	}
	wf.addEvent(WorkflowDetected, fmt.Sprintf("Issue #%d detected", issue.Number))
	s.workflows[issue.Number] = wf
	s.evictAndSave()
	return wf
}

// Transition moves a workflow to a new state with a message.
func (s *workflowStore) Transition(issueNumber int, state, message string) *issueWorkflow {
	s.mu.Lock()
	defer s.mu.Unlock()

	wf, ok := s.workflows[issueNumber]
	if !ok {
		return nil
	}

	wf.State = state
	wf.UpdatedAt = time.Now().UTC()
	wf.addEvent(state, message)

	if state == WorkflowCompleted || state == WorkflowFailed || state == WorkflowEscalated {
		now := time.Now().UTC()
		wf.CompletedAt = &now
	}

	s.save()
	return wf
}

// SetLeaderTask records the leader task ID.
func (s *workflowStore) SetLeaderTask(issueNumber int, taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wf, ok := s.workflows[issueNumber]; ok {
		wf.LeaderTask = taskID
		wf.State = WorkflowDispatched
		wf.UpdatedAt = time.Now().UTC()
		wf.addEvent(WorkflowDispatched, fmt.Sprintf("Leader task %s dispatched", taskID))
		s.save()
	}
}

// SetMember records the member dal assigned to the issue.
func (s *workflowStore) SetMember(issueNumber int, dalName string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wf, ok := s.workflows[issueNumber]; ok {
		wf.MemberDal = dalName
		wf.State = WorkflowMemberWoken
		wf.UpdatedAt = time.Now().UTC()
		wf.addEvent(WorkflowMemberWoken, fmt.Sprintf("Member %s woken for issue #%d", dalName, issueNumber))
		s.save()
	}
}

// SetMemberTask records the member task ID.
func (s *workflowStore) SetMemberTask(issueNumber int, taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wf, ok := s.workflows[issueNumber]; ok {
		wf.MemberTask = taskID
		wf.State = WorkflowAssigned
		wf.UpdatedAt = time.Now().UTC()
		wf.addEvent(WorkflowAssigned, fmt.Sprintf("Member task %s assigned", taskID))
		s.save()
	}
}

// SetPRUrl records the PR URL and transitions to pr_created state.
func (s *workflowStore) SetPRUrl(issueNumber int, prURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wf, ok := s.workflows[issueNumber]; ok {
		wf.PRUrl = prURL
		wf.State = WorkflowPRCreated
		wf.UpdatedAt = time.Now().UTC()
		wf.addEvent(WorkflowPRCreated, fmt.Sprintf("PR created: %s", prURL))
		s.save()
	}
}

// SetError records a failure with retry tracking.
func (s *workflowStore) SetError(issueNumber int, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wf, ok := s.workflows[issueNumber]; ok {
		wf.Retries++
		wf.Error = errMsg
		wf.UpdatedAt = time.Now().UTC()
		if wf.Retries >= 3 {
			wf.State = WorkflowEscalated
			now := time.Now().UTC()
			wf.CompletedAt = &now
			wf.addEvent(WorkflowEscalated, fmt.Sprintf("Escalated after %d retries: %s", wf.Retries, errMsg))
		} else {
			wf.State = WorkflowFailed
			wf.addEvent(WorkflowFailed, fmt.Sprintf("Retry %d: %s", wf.Retries, errMsg))
		}
		s.save()
	}
}

// FindByTask looks up a workflow by its leader or member task ID.
func (s *workflowStore) FindByTask(taskID string) *issueWorkflow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, wf := range s.workflows {
		if wf.LeaderTask == taskID || wf.MemberTask == taskID {
			return wf
		}
	}
	return nil
}

// FindByMember looks up an active workflow by member dal name.
func (s *workflowStore) FindByMember(dalName string) *issueWorkflow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, wf := range s.workflows {
		if wf.MemberDal == dalName && !isTerminalState(wf.State) {
			return wf
		}
	}
	return nil
}

func isTerminalState(state string) bool {
	return state == WorkflowCompleted || state == WorkflowFailed || state == WorkflowEscalated
}

func (wf *issueWorkflow) addEvent(state, message string) {
	wf.Events = append(wf.Events, workflowEvent{
		At:      time.Now().UTC(),
		State:   state,
		Message: message,
	})
}

func (s *workflowStore) save() {
	if s.filePath == "" {
		return
	}
	items := make([]*issueWorkflow, 0, len(s.workflows))
	for _, wf := range s.workflows {
		items = append(items, wf)
	}
	persistJSON(s.filePath, items, nil)
}

func (s *workflowStore) evictAndSave() {
	if len(s.workflows) > maxWorkflows {
		var oldest int
		var oldestTime time.Time
		first := true
		for num, wf := range s.workflows {
			if isTerminalState(wf.State) && (first || wf.DetectedAt.Before(oldestTime)) {
				oldest = num
				oldestTime = wf.DetectedAt
				first = false
			}
		}
		if !first {
			delete(s.workflows, oldest)
		}
	}
	s.save()
}

// HTTP handlers

// handleWorkflows returns all tracked issue workflows.
// GET /api/workflows
func (d *Daemon) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows := d.workflows.List()
	respondJSON(w, http.StatusOK, map[string]any{
		"workflows": workflows,
		"total":     len(workflows),
	})
}

// handleWorkflow returns a single issue workflow by issue number.
// GET /api/workflow/{issue}
func (d *Daemon) handleWorkflow(w http.ResponseWriter, r *http.Request) {
	issueStr := r.PathValue("issue")
	issueNum, err := strconv.Atoi(issueStr)
	if err != nil {
		http.Error(w, "invalid issue number", http.StatusBadRequest)
		return
	}
	wf := d.workflows.Get(issueNum)
	if wf == nil {
		http.Error(w, fmt.Sprintf("workflow for issue #%d not found", issueNum), http.StatusNotFound)
		return
	}
	respondJSON(w, http.StatusOK, wf)
}

// handleWorkflowTransition manually advances a workflow state.
// POST /api/workflow/{issue}/transition
// Body: {"state": "...", "message": "..."}
func (d *Daemon) handleWorkflowTransition(w http.ResponseWriter, r *http.Request) {
	issueStr := r.PathValue("issue")
	issueNum, err := strconv.Atoi(issueStr)
	if err != nil {
		http.Error(w, "invalid issue number", http.StatusBadRequest)
		return
	}

	var req struct {
		State   string `json:"state"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.State == "" {
		http.Error(w, "state is required", http.StatusBadRequest)
		return
	}

	wf := d.workflows.Transition(issueNum, req.State, req.Message)
	if wf == nil {
		http.Error(w, fmt.Sprintf("workflow for issue #%d not found", issueNum), http.StatusNotFound)
		return
	}

	log.Printf("[workflow] issue #%d → %s: %s", issueNum, req.State, req.Message)
	respondJSON(w, http.StatusOK, wf)
}

// advanceWorkflowOnTaskComplete checks if a completed task is part of an issue workflow
// and advances the workflow state accordingly.
func (d *Daemon) advanceWorkflowOnTaskComplete(dalName string, tr *taskResult) {
	wf := d.workflows.FindByTask(tr.ID)
	if wf == nil {
		// Also check by member name for tasks not explicitly linked
		wf = d.workflows.FindByMember(dalName)
	}
	if wf == nil {
		return
	}

	issueNum := wf.IssueNumber

	switch {
	case tr.Status == "failed" || tr.Status == "blocked":
		d.workflows.SetError(issueNum, fmt.Sprintf("%s task failed: %s", dalName, tr.Error))
		log.Printf("[workflow] issue #%d: task failed (%s)", issueNum, dalName)

		// If escalated, post notice
		updated := d.workflows.Get(issueNum)
		if updated != nil && updated.State == WorkflowEscalated {
			d.postWorkflowEscalation(updated)
		}

	case wf.LeaderTask == tr.ID:
		// Leader task completed — leader should have woken and assigned a member
		if wf.State == WorkflowDispatched || wf.State == WorkflowAnalyzing {
			d.workflows.Transition(issueNum, WorkflowAnalyzing, "Leader analysis complete")
		}

	case wf.MemberTask == tr.ID || wf.MemberDal == dalName:
		// Member task completed
		prURL := extractPRUrl(tr.Output)
		if prURL != "" {
			d.workflows.SetPRUrl(issueNum, prURL)
			d.workflows.Transition(issueNum, WorkflowCompleted,
				fmt.Sprintf("Member %s completed with PR: %s", dalName, prURL))
			log.Printf("[workflow] issue #%d: completed with PR %s", issueNum, prURL)
		} else if tr.GitChanges > 0 {
			d.workflows.Transition(issueNum, WorkflowWorking,
				fmt.Sprintf("Member %s made %d file changes", dalName, tr.GitChanges))
		} else {
			d.workflows.Transition(issueNum, WorkflowCompleted,
				fmt.Sprintf("Member %s completed (no PR detected)", dalName))
		}

		// Notify dalroot about completion
		d.notifyWorkflowComplete(wf)
	}
}

// advanceWorkflowOnWake updates workflow when a member is woken for an issue.
func (d *Daemon) advanceWorkflowOnWake(dalName string, issueID string) {
	if issueID == "" {
		return
	}
	issueNum, err := strconv.Atoi(issueID)
	if err != nil {
		return
	}
	d.workflows.SetMember(issueNum, dalName)
	log.Printf("[workflow] issue #%d: member %s woken", issueNum, dalName)
}

// notifyWorkflowComplete sends a notification to dalroot when a workflow finishes.
func (d *Daemon) notifyWorkflowComplete(wf *issueWorkflow) {
	event := "issue_workflow_complete"
	if wf.State == WorkflowFailed || wf.State == WorkflowEscalated {
		event = "issue_workflow_failed"
	}

	dispatchWebhook(WebhookEvent{
		Event:     event,
		Dal:       wf.MemberDal,
		Task:      fmt.Sprintf("Issue #%d: %s", wf.IssueNumber, wf.IssueTitle),
		PRUrl:     wf.PRUrl,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// postWorkflowEscalation posts an escalation notice when a workflow exceeds retry limits.
func (d *Daemon) postWorkflowEscalation(wf *issueWorkflow) {
	msg := fmt.Sprintf("⚠️ Issue #%d workflow escalated after %d retries: %s\nURL: %s\nError: %s",
		wf.IssueNumber, wf.Retries, wf.IssueTitle, wf.IssueURL, wf.Error)
	d.postWorkflowMessage(msg)
}

// postWorkflowMessage posts a workflow message via matterbridge if configured.
func (d *Daemon) postWorkflowMessage(text string) {
	go func() {
		if err := d.bridgePost(text, "dalcenter"); err != nil {
			log.Printf("[workflow] bridge post failed: %v", err)
		}
	}()
}
