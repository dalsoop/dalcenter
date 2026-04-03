package daemon

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/dalsoop/dalcenter/internal/paths"
)

// ensureDalcenterURL writes DALCENTER_URL to the team env file and /root/.bashrc
// so that CLI commands (dalcenter wake, task, etc.) can reach the daemon without
// manual export.
func ensureDalcenterURL(addr, serviceRepo string) {
	url := buildURL(addr)
	if url == "" {
		return
	}

	// Set in current process so child containers inherit it
	os.Setenv("DALCENTER_URL", url)

	envFile := envFileForRepo(serviceRepo)
	if envFile != "" {
		if err := ensureEnvLine(envFile, "DALCENTER_URL", url); err != nil {
			log.Printf("[daemon] env file update failed: %v", err)
		} else {
			log.Printf("[daemon] DALCENTER_URL=%s written to %s", url, envFile)
		}
	}

	bashrc := os.Getenv("HOME") + "/.bashrc"
	if err := ensureBashrcExport(bashrc, "DALCENTER_URL", url); err != nil {
		log.Printf("[daemon] bashrc update failed: %v", err)
	} else {
		log.Printf("[daemon] DALCENTER_URL=%s written to %s", url, bashrc)
	}
}

// buildURL constructs http://localhost:<port> from an addr like ":11190" or "0.0.0.0:11190".
func buildURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("[daemon] cannot parse addr %q: %v", addr, err)
		return ""
	}
	return "http://localhost:" + port
}

// envFileForRepo returns the env file path for the given service repo.
// Uses the repo directory basename as the systemd instance name.
func envFileForRepo(serviceRepo string) string {
	if serviceRepo == "" {
		return ""
	}
	name := filepath.Base(serviceRepo)
	return filepath.Join(paths.ConfigDir(), name+".env")
}

// ensureEnvLine ensures KEY=VALUE exists in an env file.
// Updates the value if the key exists with a different value.
// Appends the line if the key is absent.
func ensureEnvLine(path, key, value string) error {
	line := key + "=" + value

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	content := string(data)
	prefix := key + "="

	for _, l := range strings.Split(content, "\n") {
		if strings.HasPrefix(l, prefix) {
			if strings.TrimSpace(l) == line {
				return nil // already set correctly
			}
			// Update existing line
			updated := strings.Replace(content, l, line, 1)
			return os.WriteFile(path, []byte(updated), 0644)
		}
	}

	// Append
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

// ensureBashrcExport ensures export KEY=VALUE exists in bashrc.
// Updates the value if the key exists with a different value.
// Appends the line if the key is absent.
func ensureBashrcExport(path, key, value string) error {
	line := "export " + key + "=" + value

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	content := string(data)
	prefix := "export " + key + "="

	for _, l := range strings.Split(content, "\n") {
		if strings.HasPrefix(l, prefix) {
			if strings.TrimSpace(l) == line {
				return nil // already set correctly
			}
			// Update existing line
			updated := strings.Replace(content, l, line, 1)
			return os.WriteFile(path, []byte(updated), 0644)
		}
	}

	// Append
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}
