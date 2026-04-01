package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dalsoop/dalcenter/internal/daemon"
	"github.com/dalsoop/dalcenter/internal/localdal"
	"github.com/spf13/cobra"
)

func newRepoCreateCmd() *cobra.Command {
	var (
		org         string
		name        string
		lang        string
		description string
		teamSize    int
		bridgeURL   string
		skipRegister bool
		skipMM       bool
	)

	cmd := &cobra.Command{
		Use:   "repo-create",
		Short: "Create a new repository with .dal/ scaffold, register, and MM channel",
		Long: `Create a GitHub repository and configure it for dal team operation.

Steps:
  1. Create GitHub repo (gh repo create)
  2. Generate language-specific initial files
  3. Initialize .dal/ scaffold with leader + dev dals
  4. Register with dalcenter (port, systemd, tokens)
  5. Create Mattermost channel and bot`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if lang != "go" && lang != "rust" && lang != "node" {
				return fmt.Errorf("--lang must be one of: go, rust, node")
			}

			fullName := org + "/" + name
			repoPath := filepath.Join("/root", name)

			// --- Stage 1: Create GitHub repo ---
			fmt.Printf("[1/5] creating GitHub repo: %s\n", fullName)
			ghArgs := []string{"repo", "create", fullName, "--public", "--clone"}
			if description != "" {
				ghArgs = append(ghArgs, "--description", description)
			}
			ghCmd := exec.Command("gh", ghArgs...)
			ghCmd.Dir = "/root"
			ghCmd.Stdout = os.Stdout
			ghCmd.Stderr = os.Stderr
			if err := ghCmd.Run(); err != nil {
				return fmt.Errorf("gh repo create: %w", err)
			}

			// --- Stage 2: Language-specific initial files ---
			fmt.Printf("[2/5] generating %s project files\n", lang)
			if err := generateLangFiles(repoPath, name, org, lang); err != nil {
				return fmt.Errorf("generate lang files: %w", err)
			}

			// --- Stage 3: .dal/ scaffold ---
			fmt.Printf("[3/5] initializing .dal/ scaffold\n")
			dalRoot := filepath.Join(repoPath, ".dal")
			if err := localdal.Init(dalRoot); err != nil {
				return fmt.Errorf("init .dal/: %w", err)
			}
			if err := scaffoldTeam(dalRoot, name, lang, teamSize); err != nil {
				return fmt.Errorf("scaffold team: %w", err)
			}

			// --- Stage 4: Register (port, systemd, tokens, initial commit + push) ---
			if !skipRegister {
				fmt.Printf("[4/5] registering with dalcenter\n")
				if err := registerRepo(repoPath, name, bridgeURL); err != nil {
					return fmt.Errorf("register: %w", err)
				}
			} else {
				fmt.Printf("[4/5] skipped (--skip-register)\n")
			}

			// Initial commit + push
			fmt.Println("  committing and pushing...")
			if err := initialCommitAndPush(repoPath, name); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: initial commit/push: %v\n", err)
			}

			// --- Stage 5: Mattermost channel ---
			if !skipMM {
				fmt.Printf("[5/5] creating Mattermost channel\n")
				if err := createMMChannel(name); err != nil {
					fmt.Fprintf(os.Stderr, "[5/5] warning: MM channel: %v\n", err)
				}
			} else {
				fmt.Printf("[5/5] skipped (--skip-mm)\n")
			}

			fmt.Printf("\nrepo-create complete: %s\n", fullName)
			fmt.Printf("  path: %s\n", repoPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&org, "org", "dalsoop", "GitHub organization")
	cmd.Flags().StringVar(&name, "name", "", "Repository name (required)")
	cmd.Flags().StringVar(&lang, "lang", "go", "Language: go, rust, node")
	cmd.Flags().StringVar(&description, "description", "", "Repository description")
	cmd.Flags().IntVar(&teamSize, "team-size", 2, "Number of dev dals to create")
	cmd.Flags().StringVar(&bridgeURL, "bridge-url", envOrDefault("DALCENTER_BRIDGE_URL", daemon.DefaultBridgeURL), "Matterbridge API URL")
	cmd.Flags().BoolVar(&skipRegister, "skip-register", false, "Skip dalcenter register step")
	cmd.Flags().BoolVar(&skipMM, "skip-mm", false, "Skip Mattermost channel creation")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// generateLangFiles creates language-specific initial project files.
func generateLangFiles(repoPath, name, org, lang string) error {
	switch lang {
	case "go":
		modulePath := fmt.Sprintf("github.com/%s/%s", org, name)
		goMod := fmt.Sprintf("module %s\n\ngo 1.25.0\n", modulePath)
		if err := os.WriteFile(filepath.Join(repoPath, "go.mod"), []byte(goMod), 0644); err != nil {
			return fmt.Errorf("write go.mod: %w", err)
		}
		mainGo := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
		if err := os.MkdirAll(filepath.Join(repoPath, "cmd", name), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(repoPath, "cmd", name, "main.go"), []byte(mainGo), 0644); err != nil {
			return fmt.Errorf("write main.go: %w", err)
		}

	case "rust":
		cargoToml := fmt.Sprintf(`[package]
name = %q
version = "0.1.0"
edition = "2024"
`, name)
		if err := os.WriteFile(filepath.Join(repoPath, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
			return fmt.Errorf("write Cargo.toml: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(repoPath, "src"), 0755); err != nil {
			return err
		}
		mainRs := "fn main() {\n    println!(\"hello\");\n}\n"
		if err := os.WriteFile(filepath.Join(repoPath, "src", "main.rs"), []byte(mainRs), 0644); err != nil {
			return fmt.Errorf("write main.rs: %w", err)
		}

	case "node":
		pkgJSON := fmt.Sprintf(`{
  "name": %q,
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "start": "node index.js"
  }
}
`, name)
		if err := os.WriteFile(filepath.Join(repoPath, "package.json"), []byte(pkgJSON), 0644); err != nil {
			return fmt.Errorf("write package.json: %w", err)
		}
		indexJS := "console.log('hello');\n"
		if err := os.WriteFile(filepath.Join(repoPath, "index.js"), []byte(indexJS), 0644); err != nil {
			return fmt.Errorf("write index.js: %w", err)
		}
	}

	// .gitignore
	gitignore := gitignoreForLang(lang)
	if err := os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}

	// README.md
	readme := fmt.Sprintf("# %s\n\n%s project managed by dalcenter.\n", name, lang)
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte(readme), 0644); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}

	return nil
}

func gitignoreForLang(lang string) string {
	switch lang {
	case "go":
		return "# Go\n*.exe\n*.test\n*.out\nvendor/\n"
	case "rust":
		return "# Rust\n/target\nCargo.lock\n"
	case "node":
		return "# Node\nnode_modules/\ndist/\n*.log\n"
	default:
		return ""
	}
}

// scaffoldTeam creates leader, dev dals, roster.md, and team.md within .dal/.
func scaffoldTeam(dalRoot, repoName, lang string, teamSize int) error {
	tplRoot := localdal.ResolveTemplateRoot(dalRoot)

	// Create leader dal (using the standard template from Init)
	leaderDir := filepath.Join(tplRoot, "leader")
	if _, err := os.Stat(leaderDir); err != nil {
		if err := os.MkdirAll(leaderDir, 0755); err != nil {
			return fmt.Errorf("create leader dir: %w", err)
		}
		leaderCue := fmt.Sprintf(leaderCueTemplate, localdal.GenerateUUID())
		if err := os.WriteFile(filepath.Join(leaderDir, "dal.cue"), []byte(leaderCue), 0644); err != nil {
			return fmt.Errorf("write leader dal.cue: %w", err)
		}
		if err := os.WriteFile(filepath.Join(leaderDir, "charter.md"), []byte(leaderCharterTemplate), 0644); err != nil {
			return fmt.Errorf("write leader charter.md: %w", err)
		}
	}

	// Map lang to skills and Docker image
	langSkills := skillsForLang(lang)
	dockerImage := dockerImageForLang(lang)

	// Create dev dals
	for i := 1; i <= teamSize; i++ {
		devName := "dev"
		if teamSize > 1 {
			devName = fmt.Sprintf("dev%d", i)
		}
		devDir := filepath.Join(tplRoot, devName)
		if _, err := os.Stat(devDir); err == nil {
			continue // already exists
		}
		if err := os.MkdirAll(devDir, 0755); err != nil {
			return fmt.Errorf("create %s dir: %w", devName, err)
		}

		devCue := fmt.Sprintf(devCueTemplate, localdal.GenerateUUID(), devName, strings.Join(langSkills, `", "`))
		if err := os.WriteFile(filepath.Join(devDir, "dal.cue"), []byte(devCue), 0644); err != nil {
			return fmt.Errorf("write %s dal.cue: %w", devName, err)
		}
		if err := os.WriteFile(filepath.Join(devDir, "charter.md"), []byte(devCharterTemplate), 0644); err != nil {
			return fmt.Errorf("write %s charter.md: %w", devName, err)
		}
	}

	// Create lang-specific skills if not present
	for _, s := range langSkills {
		skillName := strings.TrimPrefix(s, "skills/")
		skillDir := filepath.Join(tplRoot, "skills", skillName)
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			continue
		}
		os.MkdirAll(skillDir, 0755)
		content := fmt.Sprintf("# %s\n\n%s 프로젝트용 스킬.\n", skillName, lang)
		if err := os.WriteFile(skillFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("write skill %s: %w", skillName, err)
		}
	}

	// roster.md
	var roster strings.Builder
	roster.WriteString("# Roster\n\n")
	roster.WriteString("| dal | role | player | image |\n")
	roster.WriteString("|-----|------|--------|-------|\n")
	roster.WriteString("| leader | leader | claude | claude-base |\n")
	for i := 1; i <= teamSize; i++ {
		devName := "dev"
		if teamSize > 1 {
			devName = fmt.Sprintf("dev%d", i)
		}
		roster.WriteString(fmt.Sprintf("| %s | member | claude | %s |\n", devName, dockerImage))
	}
	roster.WriteString("| dalops | ops | claude | claude-base |\n")
	roster.WriteString("| dal | member | claude | claude-base |\n")

	rosterPath := filepath.Join(tplRoot, "roster.md")
	if err := os.WriteFile(rosterPath, []byte(roster.String()), 0644); err != nil {
		return fmt.Errorf("write roster.md: %w", err)
	}

	// team.md
	team := fmt.Sprintf(`# %s Team

## Language
%s

## Docker Image
%s

## Team Size
%d dev dal(s) + leader + dalops + dal (document manager)

## Managed By
dalcenter
`, repoName, lang, dockerImage, teamSize)

	teamPath := filepath.Join(tplRoot, "team.md")
	if err := os.WriteFile(teamPath, []byte(team), 0644); err != nil {
		return fmt.Errorf("write team.md: %w", err)
	}

	return nil
}

func skillsForLang(lang string) []string {
	base := []string{"skills/git-workflow", "skills/pre-flight", "skills/inbox-protocol"}
	switch lang {
	case "go":
		return append(base, "skills/go-review")
	case "rust":
		return append(base, "skills/rust-review")
	case "node":
		return append(base, "skills/node-review")
	default:
		return base
	}
}

func dockerImageForLang(lang string) string {
	switch lang {
	case "rust":
		return "claude-rust"
	case "node":
		return "claude-node"
	default:
		return "claude-go"
	}
}

// registerRepo reuses register logic: port allocation, systemd, tokens, soft-serve.
func registerRepo(repoPath, repoName, bridgeURL string) error {
	// Port allocation
	port := nextAvailablePort()
	addr := fmt.Sprintf(":%d", port)
	serviceName := systemdServiceName(repoName)

	// systemd service
	if err := installSystemdService(serviceName, repoPath, addr, bridgeURL); err != nil {
		return fmt.Errorf("systemd: %w", err)
	}
	fmt.Printf("  systemd service: %s (port %d)\n", serviceName, port)

	// Token injection
	if err := injectTokensToService(serviceName, repoPath); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: token injection: %v\n", err)
	} else {
		fmt.Println("  tokens injected")
	}

	// Soft-serve subtree
	ssRepoName := repoName + "-localdal"
	if err := daemon.EnsureSoftServeRepo(ssRepoName); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: soft-serve repo: %v\n", err)
	} else if err := daemon.SetupSubtree(repoPath, ssRepoName); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: subtree: %v\n", err)
	} else {
		fmt.Printf("  soft-serve subtree: %s/.dal → %s\n", repoPath, ssRepoName)
	}

	fmt.Printf("  url: http://localhost:%d\n", port)
	return nil
}

// initialCommitAndPush creates the initial commit and pushes to origin.
func initialCommitAndPush(repoPath, repoName string) error {
	cmds := [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", fmt.Sprintf("feat: initialize %s with .dal/ scaffold", repoName)},
		{"git", "push", "-u", "origin", "main"},
	}
	for _, args := range cmds {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = repoPath
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s: %w", args[0], err)
		}
	}
	return nil
}

// createMMChannel creates a Mattermost channel for the new team.
func createMMChannel(repoName string) error {
	mmURL := os.Getenv("DALCENTER_MM_URL")
	mmToken := os.Getenv("DALCENTER_MM_TOKEN")
	if mmURL == "" || mmToken == "" {
		return fmt.Errorf("DALCENTER_MM_URL or DALCENTER_MM_TOKEN not set")
	}

	mmTeam := os.Getenv("DALCENTER_MM_TEAM")
	if mmTeam == "" {
		mmTeam = "dalsoop"
	}

	// Resolve team ID
	teamID, err := mmGetTeamID(mmURL, mmToken, mmTeam)
	if err != nil {
		return fmt.Errorf("resolve team ID: %w", err)
	}

	// Create channel
	channelName := repoName
	payload := fmt.Sprintf(`{"team_id":%q,"name":%q,"display_name":%q,"type":"O"}`,
		teamID, channelName, repoName)

	req, _ := http.NewRequest("POST", strings.TrimRight(mmURL, "/")+"/api/v4/channels", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+mmToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("create channel: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		// Channel might already exist — not fatal
		if resp.StatusCode == 409 {
			fmt.Printf("  channel %q already exists\n", channelName)
		} else {
			return fmt.Errorf("create channel %d: %s", resp.StatusCode, string(body))
		}
	} else {
		fmt.Printf("  channel created: %s\n", channelName)
	}

	return nil
}

// mmGetTeamID resolves a Mattermost team name to its ID.
func mmGetTeamID(mmURL, mmToken, teamName string) (string, error) {
	url := fmt.Sprintf("%s/api/v4/teams/name/%s", strings.TrimRight(mmURL, "/"), teamName)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+mmToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get team %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode team: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("team %q not found", teamName)
	}
	return result.ID, nil
}

// CUE templates for repo-create scaffolding.
// Each has one %s placeholder for UUID.
const leaderCueTemplate = `uuid:    %q
name:    "leader"
version: "1.0.0"
player:  "claude"
role:    "leader"
channel_only: true
skills:  ["skills/leader-protocol", "skills/inbox-protocol", "skills/pre-flight"]
hooks:   []
git: {
	user:         "dal-leader"
	email:        "dal-leader@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
`

const leaderCharterTemplate = `# Leader Dal

## Role
팀 리더. 작업을 분배하고 결과를 검토한다. 직접 코드를 수정하지 않는다.

## Tools
- dalcli-leader ps — 팀 상태 확인
- dalcli-leader wake/sleep <dal> — 멤버 관리
- dalcli-leader assign <dal> <task> — 작업 배정
- dalcli-leader logs <dal> — 멤버 로그 확인

## Workflow
1. 작업 요청 수신
2. 하위 작업으로 분해
3. 멤버에게 배정 (dalcli-leader assign)
4. 진행 상황 모니터링
5. 결과 검토 및 피드백
6. 완료 시 최종 PR 생성

## Rules
- main 직접 커밋 금지
- Write/Edit/commit 금지 — dalcli-leader assign으로 위임
- 리뷰 없이 머지 금지
`

// devCueTemplate has %q for UUID, %q for name, and %s for skills (joined).
const devCueTemplate = `uuid:    %q
name:    %q
version: "1.0.0"
player:  "claude"
role:    "member"
channel_only: true
skills:  ["%s"]
hooks:   []
git: {
	user:         "dal-dev"
	email:        "dal-dev@dalcenter.local"
	github_token: "env:GITHUB_TOKEN"
}
`

const devCharterTemplate = `# Dev Dal

## Role
개발자. 코드를 작성하고 테스트한다.

## Tools
- dalcli status — 내 상태 확인
- dalcli ps — 팀 상태 확인
- dalcli report <message> — 리더에게 보고

## Workflow
1. 배정된 작업 확인
2. 브랜치 생성: git checkout -b feat/<task>
3. 구현 및 테스트
4. 커밋, 푸시, PR 생성
5. dalcli report로 완료 보고

## Rules
- main 직접 커밋 금지
- 테스트 작성 필수
- 작고 명확한 커밋
- 기존 코드 패턴 준수
`
