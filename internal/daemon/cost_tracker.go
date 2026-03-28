package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TokenUsage holds parsed token counts from a task execution.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CostRecord represents a single token usage entry from a task execution.
type CostRecord struct {
	ID           string    `json:"id"`
	Dal          string    `json:"dal"`
	Repo         string    `json:"repo"`
	TaskID       string    `json:"task_id"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Timestamp    time.Time `json:"timestamp"`
}

// CostSummary holds aggregated cost statistics.
type CostSummary struct {
	Key          string  `json:"key"` // dal name or repo path
	TotalInput   int     `json:"total_input_tokens"`
	TotalOutput  int     `json:"total_output_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	TaskCount    int     `json:"task_count"`
}

// CostAlert is emitted when a cost threshold is exceeded.
type CostAlert struct {
	Dal          string  `json:"dal"`
	Repo         string  `json:"repo"`
	CurrentCost  float64 `json:"current_cost_usd"`
	ThresholdUSD float64 `json:"threshold_usd"`
	Message      string  `json:"message"`
}

// DowngradeSuggestion recommends a cheaper model when cost is high.
type DowngradeSuggestion struct {
	Dal          string  `json:"dal"`
	CurrentModel string  `json:"current_model"`
	SuggestModel string  `json:"suggest_model"`
	CurrentCost  float64 `json:"current_cost_usd"`
	EstSavings   float64 `json:"estimated_savings_pct"`
	Reason       string  `json:"reason"`
}

// CostAlertHandler is called when a cost threshold is exceeded.
type CostAlertHandler func(alert CostAlert)

// DowngradeHandler is called when a model downgrade is suggested.
type DowngradeHandler func(suggestion DowngradeSuggestion)

// costStore tracks token usage and costs with persistence.
type costStore struct {
	mu               sync.RWMutex
	items            []CostRecord
	seq              int
	filePath         string
	logDir           string // orchestration-log/ directory
	thresholdUSD     float64
	onAlert          CostAlertHandler
	onDowngrade      DowngradeHandler
}

const maxCostRecords = 1000

// --- model pricing (USD per 1M tokens) ---

var modelPricing = map[string][2]float64{
	// [input, output] per 1M tokens
	"claude-opus-4-6":          {15.0, 75.0},
	"claude-sonnet-4-6":        {3.0, 15.0},
	"claude-haiku-4-5":         {0.80, 4.0},
	"claude-sonnet-4-5":        {3.0, 15.0},
	"claude-opus-4-5":          {15.0, 75.0},
	"codex":                    {3.0, 15.0},
	"gemini-2.5-pro":           {1.25, 10.0},
	"gemini-2.5-flash":         {0.15, 0.60},
}

// downgrade paths: model -> cheaper alternative
var downgradePaths = map[string]string{
	"claude-opus-4-6":   "claude-sonnet-4-6",
	"claude-opus-4-5":   "claude-sonnet-4-5",
	"claude-sonnet-4-6": "claude-haiku-4-5",
	"claude-sonnet-4-5": "claude-haiku-4-5",
	"gemini-2.5-pro":    "gemini-2.5-flash",
}

func newCostStore() *costStore {
	return &costStore{items: make([]CostRecord, 0)}
}

func newCostStoreWithFile(path, logDir string) *costStore {
	s := &costStore{
		items:    make([]CostRecord, 0),
		filePath: path,
		logDir:   logDir,
	}
	// Default threshold from env (USD)
	if v := os.Getenv("DALCENTER_COST_THRESHOLD_USD"); v != "" {
		if t, err := strconv.ParseFloat(v, 64); err == nil {
			s.thresholdUSD = t
		}
	}
	if s.logDir != "" {
		os.MkdirAll(s.logDir, 0o755)
	}
	s.load()
	return s
}

func (s *costStore) load() {
	if s.filePath == "" {
		return
	}
	var items []CostRecord
	if err := loadJSON(s.filePath, &items); err != nil {
		return
	}
	s.items = items
	for _, c := range items {
		var n int
		fmt.Sscanf(c.ID, "cost-%d", &n)
		if n > s.seq {
			s.seq = n
		}
	}
}

func (s *costStore) save() {
	if s.filePath == "" {
		return
	}
	persistJSON(s.filePath, s.items, nil)
}

// Add records a token usage entry and returns the cost record.
func (s *costStore) Add(dal, repo, taskID, model string, input, output int) CostRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++

	costUSD := calcCost(model, input, output)

	rec := CostRecord{
		ID:           fmt.Sprintf("cost-%04d", s.seq),
		Dal:          dal,
		Repo:         repo,
		TaskID:       taskID,
		Model:        model,
		InputTokens:  input,
		OutputTokens: output,
		CostUSD:      costUSD,
		Timestamp:    time.Now().UTC(),
	}
	s.items = append(s.items, rec)

	if len(s.items) > maxCostRecords {
		s.items = s.items[len(s.items)-maxCostRecords:]
	}
	s.save()
	s.writeLog(rec)

	// Check threshold (unlocked callbacks)
	go s.checkThreshold(dal, repo)
	go s.checkDowngrade(dal, model, input, output)

	return rec
}

// SummaryByDal aggregates costs per dal.
func (s *costStore) SummaryByDal() []CostSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aggregate(func(r CostRecord) string { return r.Dal })
}

// SummaryByRepo aggregates costs per repo.
func (s *costStore) SummaryByRepo() []CostSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aggregate(func(r CostRecord) string { return r.Repo })
}

func (s *costStore) aggregate(keyFn func(CostRecord) string) []CostSummary {
	m := make(map[string]*CostSummary)
	for _, r := range s.items {
		k := keyFn(r)
		cs, ok := m[k]
		if !ok {
			cs = &CostSummary{Key: k}
			m[k] = cs
		}
		cs.TotalInput += r.InputTokens
		cs.TotalOutput += r.OutputTokens
		cs.TotalCostUSD += r.CostUSD
		cs.TaskCount++
	}
	result := make([]CostSummary, 0, len(m))
	for _, cs := range m {
		result = append(result, *cs)
	}
	return result
}

// List returns cost records, optionally filtered by dal.
func (s *costStore) List(dal string) []CostRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []CostRecord
	for _, r := range s.items {
		if dal == "" || r.Dal == dal {
			result = append(result, r)
		}
	}
	return result
}

// writeLog appends a cost record to orchestration-log/ as a JSON line file.
func (s *costStore) writeLog(rec CostRecord) {
	if s.logDir == "" {
		return
	}
	date := rec.Timestamp.Format("2006-01-02")
	logFile := filepath.Join(s.logDir, fmt.Sprintf("costs-%s.jsonl", date))
	b, err := json.Marshal(rec)
	if err != nil {
		log.Printf("[cost-tracker] log marshal error: %v", err)
		return
	}
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[cost-tracker] log write error: %v", err)
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

func (s *costStore) checkThreshold(dal, repo string) {
	if s.thresholdUSD <= 0 || s.onAlert == nil {
		return
	}
	s.mu.RLock()
	var total float64
	for _, r := range s.items {
		if r.Dal == dal {
			total += r.CostUSD
		}
	}
	s.mu.RUnlock()

	if total >= s.thresholdUSD {
		s.onAlert(CostAlert{
			Dal:          dal,
			Repo:         repo,
			CurrentCost:  total,
			ThresholdUSD: s.thresholdUSD,
			Message:      fmt.Sprintf("dal %q exceeded cost threshold: $%.4f >= $%.4f", dal, total, s.thresholdUSD),
		})
	}
}

func (s *costStore) checkDowngrade(dal, model string, input, output int) {
	if s.onDowngrade == nil {
		return
	}
	cheaper, ok := downgradePaths[model]
	if !ok {
		return
	}
	currentCost := calcCost(model, input, output)
	cheaperCost := calcCost(cheaper, input, output)
	if currentCost <= 0 {
		return
	}
	savings := (currentCost - cheaperCost) / currentCost * 100

	if savings >= 50 {
		s.onDowngrade(DowngradeSuggestion{
			Dal:          dal,
			CurrentModel: model,
			SuggestModel: cheaper,
			CurrentCost:  currentCost,
			EstSavings:   savings,
			Reason:       fmt.Sprintf("switching from %s to %s saves ~%.0f%% on this task", model, cheaper, savings),
		})
	}
}

// --- token parsing ---

// reTokens matches Claude's token usage output lines.
// Example: "Total tokens: 1234 input, 5678 output"
// Example: "Input tokens: 1234\nOutput tokens: 5678"
var (
	reTokenTotal  = regexp.MustCompile(`(?i)total\s+(?:cost|tokens?).*?(\d[\d,]*)\s*input.*?(\d[\d,]*)\s*output`)
	reInputToken  = regexp.MustCompile(`(?i)input\s+tokens?\s*[:=]\s*(\d[\d,]*)`)
	reOutputToken = regexp.MustCompile(`(?i)output\s+tokens?\s*[:=]\s*(\d[\d,]*)`)
)

// ParseTokenUsage extracts input/output token counts from task output text.
func ParseTokenUsage(output string) TokenUsage {
	var usage TokenUsage

	// Try combined pattern first
	if m := reTokenTotal.FindStringSubmatch(output); len(m) == 3 {
		usage.InputTokens = parseIntComma(m[1])
		usage.OutputTokens = parseIntComma(m[2])
		return usage
	}

	// Fall back to separate patterns
	if m := reInputToken.FindStringSubmatch(output); len(m) == 2 {
		usage.InputTokens = parseIntComma(m[1])
	}
	if m := reOutputToken.FindStringSubmatch(output); len(m) == 2 {
		usage.OutputTokens = parseIntComma(m[1])
	}
	return usage
}

func parseIntComma(s string) int {
	s = strings.ReplaceAll(s, ",", "")
	n, _ := strconv.Atoi(s)
	return n
}

// calcCost calculates USD cost based on model pricing.
func calcCost(model string, inputTokens, outputTokens int) float64 {
	prices, ok := modelPricing[model]
	if !ok {
		// Default to sonnet pricing for unknown models
		prices = modelPricing["claude-sonnet-4-6"]
	}
	inputCost := float64(inputTokens) / 1_000_000 * prices[0]
	outputCost := float64(outputTokens) / 1_000_000 * prices[1]
	return inputCost + outputCost
}

// orchestrationLogDir returns the orchestration-log directory path.
func orchestrationLogDir(serviceRepo string) string {
	dir := filepath.Join(stateDir(serviceRepo), "orchestration-log")
	os.MkdirAll(dir, 0o755)
	return dir
}

// --- HTTP handlers ---

// POST /api/cost — record token usage
func (d *Daemon) handleCostRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dal          string `json:"dal"`
		TaskID       string `json:"task_id"`
		Model        string `json:"model"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Dal == "" {
		http.Error(w, "dal is required", http.StatusBadRequest)
		return
	}

	rec := d.costs.Add(req.Dal, d.serviceRepo, req.TaskID, req.Model, req.InputTokens, req.OutputTokens)
	respondJSON(w, http.StatusOK, rec)
}

// GET /api/costs?dal=dev — list cost records
func (d *Daemon) handleCostList(w http.ResponseWriter, r *http.Request) {
	dal := r.URL.Query().Get("dal")
	items := d.costs.List(dal)
	if items == nil {
		items = []CostRecord{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"costs": items,
		"count": len(items),
	})
}

// GET /api/costs/summary — aggregated cost summary
func (d *Daemon) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	groupBy := r.URL.Query().Get("by")
	var summary []CostSummary
	switch groupBy {
	case "repo":
		summary = d.costs.SummaryByRepo()
	default:
		summary = d.costs.SummaryByDal()
	}
	if summary == nil {
		summary = []CostSummary{}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"summary":  summary,
		"group_by": groupBy,
	})
}
