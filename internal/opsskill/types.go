package opsskill

// Skill names for ops gateway.
const (
	SkillCFPagesDeploy  = "cf-pages-deploy"
	SkillDNSManage      = "dns-manage"
	SkillGitPush        = "git-push"
	SkillCertManage     = "cert-manage"
	SkillServiceRestart = "service-restart"
)

// ValidSkills is the whitelist of allowed skill names.
var ValidSkills = map[string]string{
	SkillCFPagesDeploy:  "/api/cf-pages/deploy",
	SkillDNSManage:      "/api/dns/manage",
	SkillGitPush:        "/api/git/push",
	SkillCertManage:     "/api/cert/manage",
	SkillServiceRestart: "/api/service/restart",
}

// SkillInfo describes an available ops skill.
type SkillInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    []string `json:"required_params"`
}

// SkillCatalog lists all available skills with metadata.
var SkillCatalog = []SkillInfo{
	{Name: SkillCFPagesDeploy, Description: "Cloudflare Pages deploy", Required: []string{"project"}},
	{Name: SkillDNSManage, Description: "DNS record management", Required: []string{"action", "zone", "name", "type", "content"}},
	{Name: SkillGitPush, Description: "Git push via gateway", Required: []string{"repo", "branch"}},
	{Name: SkillCertManage, Description: "TLS certificate management", Required: []string{"action", "domain"}},
	{Name: SkillServiceRestart, Description: "Systemd service restart", Required: []string{"service"}},
}

// InvokeRequest is sent by ops dals to execute a skill.
type InvokeRequest struct {
	Skill  string         `json:"skill"`
	Params map[string]any `json:"params"`
	Dal    string         `json:"dal"`
}

// InvokeResponse is returned to ops dals after skill execution.
type InvokeResponse struct {
	OK     bool           `json:"ok"`
	Skill  string         `json:"skill"`
	Result map[string]any `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}
