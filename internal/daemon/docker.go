package daemon

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dalsoop/dalcenter/internal/localdal"
)

// instructionsFileName returns the target filename based on player.
func instructionsFileName(player string) string {
	switch player {
	case "claude":
		return "CLAUDE.md"
	case "codex":
		return "AGENTS.md"
	case "gemini":
		return "GEMINI.md"
	default:
		return "AGENTS.md"
	}
}

// playerHome returns the config home path inside the container.
func playerHome(player string) string {
	switch player {
	case "claude":
		return "/root/.claude"
	case "codex":
		return "/root/.codex"
	case "gemini":
		return "/root/.gemini"
	default:
		return "/root/.config"
	}
}

// dockerRun creates and starts a Docker container for a dal.
func dockerRun(localdalRoot, serviceRepo string, dal *localdal.DalProfile) (string, error) {
	containerName := fmt.Sprintf("dal-%s", dal.Name)
	image := fmt.Sprintf("dalcenter/%s:latest", dal.Player)

	dalDir := filepath.Join(localdalRoot, dal.FolderName)
	home := playerHome(dal.Player)
	hostHome, _ := os.UserHomeDir()

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--hostname", dal.Name,
		// Environment
		"-e", fmt.Sprintf("DAL_NAME=%s", dal.Name),
		"-e", fmt.Sprintf("DAL_UUID=%s", dal.UUID),
		"-e", fmt.Sprintf("DAL_ROLE=%s", dal.Role),
		"-e", fmt.Sprintf("DAL_PLAYER=%s", dal.Player),
		"-e", fmt.Sprintf("DALCENTER_URL=http://host.docker.internal:11190"),
		// Mount dal directory (read-only)
		"-v", fmt.Sprintf("%s:%s:ro", dalDir, "/dal"),
		// Working directory
		"-w", "/workspace",
	}

	// Mount service repo as /workspace
	if serviceRepo != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/workspace", serviceRepo))
	}

	// Mount credentials (player-specific)
	credPath := filepath.Join(hostHome, ".claude", ".credentials.json")
	if dal.Player == "claude" {
		if _, err := os.Stat(credPath); err == nil {
			args = append(args, "-v", fmt.Sprintf("%s:%s/.credentials.json:ro", credPath, home))
		}
	}

	// Mount skills
	for _, skill := range dal.Skills {
		skillPath := filepath.Join(localdalRoot, skill)
		targetPath := filepath.Join(home, "skills", filepath.Base(skill))
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", skillPath, targetPath))
	}

	// Mount instructions.md as the right filename
	instrSrc := filepath.Join(dalDir, "instructions.md")
	instrDst := filepath.Join(home, instructionsFileName(dal.Player))
	args = append(args, "-v", fmt.Sprintf("%s:%s:ro", instrSrc, instrDst))

	// Git config
	args = append(args, "-e", fmt.Sprintf("GIT_AUTHOR_NAME=dal-%s", dal.Name))
	args = append(args, "-e", fmt.Sprintf("GIT_AUTHOR_EMAIL=dal-%s@dalcenter.local", dal.Name))
	args = append(args, "-e", fmt.Sprintf("GIT_COMMITTER_NAME=dal-%s", dal.Name))
	args = append(args, "-e", fmt.Sprintf("GIT_COMMITTER_EMAIL=dal-%s@dalcenter.local", dal.Name))

	args = append(args, image)

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %s: %w", strings.TrimSpace(string(out)), err)
	}
	containerID := strings.TrimSpace(string(out))

	// Inject dalcli / dalcli-leader based on role
	if err := injectCli(containerID, dal.Role); err != nil {
		log.Printf("[docker] warning: failed to inject dalcli: %v", err)
	}

	return containerID, nil
}

// injectCli copies dalcli or dalcli-leader binary into the container.
func injectCli(containerID, role string) error {
	// Find binaries next to dalcenter binary
	self, err := os.Executable()
	if err != nil {
		return err
	}
	binDir := filepath.Dir(self)

	// Always inject dalcli
	dalcliPath := filepath.Join(binDir, "dalcli")
	if _, err := os.Stat(dalcliPath); err == nil {
		cp := exec.Command("docker", "cp", dalcliPath, containerID+":/usr/local/bin/dalcli")
		if out, err := cp.CombinedOutput(); err != nil {
			return fmt.Errorf("inject dalcli: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	// Inject dalcli-leader for leader role
	if role == "leader" {
		leaderPath := filepath.Join(binDir, "dalcli-leader")
		if _, err := os.Stat(leaderPath); err == nil {
			cp := exec.Command("docker", "cp", leaderPath, containerID+":/usr/local/bin/dalcli-leader")
			if out, err := cp.CombinedOutput(); err != nil {
				return fmt.Errorf("inject dalcli-leader: %s: %w", strings.TrimSpace(string(out)), err)
			}
		}
	}

	return nil
}

// dockerStop stops and removes a Docker container.
func dockerStop(containerID string) error {
	// Stop
	stop := exec.Command("docker", "stop", containerID)
	if out, err := stop.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// Remove
	rm := exec.Command("docker", "rm", containerID)
	if out, err := rm.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// dockerSync verifies a running container matches its dal profile.
// Since instructions and skills are bind-mounted, file changes are automatic.
// Sync handles structural changes (new skills added/removed in dal.cue).
func dockerSync(localdalRoot, containerID string, dal *localdal.DalProfile) error {
	// Bind mounts auto-reflect file changes.
	// If dal.cue changed (e.g., new skill added), container needs restart.
	// For now, log what would change.
	log.Printf("[sync] %s: player=%s, skills=%d — bind mounts are live", dal.Name, dal.Player, len(dal.Skills))
	return nil
}

// dockerLogs returns logs from a Docker container.
func dockerLogs(containerID string) (string, error) {
	cmd := exec.Command("docker", "logs", "--tail", "100", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker logs: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// dockerExec runs a command inside a Docker container (for attach).
func dockerExec(containerID string) *exec.Cmd {
	return exec.Command("docker", "exec", "-it", containerID, "/bin/bash")
}
