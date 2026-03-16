package provision

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"dalforge-hub/dalcenter/internal/state"
)

// Spec describes what to provision.
type Spec struct {
	Base         string // e.g. "ubuntu:24.04"
	InstanceName string // used as hostname/CT description
	VMID         string // if empty, auto-assigned by pct
}

// Result of a provision attempt.
type Result struct {
	VMID    string
	Command string // the constructed command (for dry-run)
	Error   error
}

// BuildCommand constructs the pct create command args without executing.
func BuildCommand(spec Spec) []string {
	args := []string{"create"}

	if spec.VMID != "" {
		args = append(args, spec.VMID)
	} else {
		args = append(args, "0") // 0 = next available
	}

	// Map base to ostemplate path
	ostemplate := resolveTemplate(spec.Base)
	args = append(args, "--ostemplate", ostemplate)
	args = append(args, "--hostname", sanitizeHostname(spec.InstanceName))
	args = append(args, "--storage", "local-lvm")
	args = append(args, "--memory", "512")
	args = append(args, "--rootfs", "local-lvm:4")
	args = append(args, "--unprivileged", "1")
	args = append(args, "--start", "0")

	return args
}

// Provision runs pct create and records result in state.json.
func Provision(instanceRoot string, spec Spec, dryRun bool) Result {
	args := BuildCommand(spec)
	cmdStr := "pct " + strings.Join(args, " ")

	if dryRun {
		return Result{Command: cmdStr}
	}

	// Check pct exists
	if _, err := exec.LookPath("pct"); err != nil {
		r := Result{Command: cmdStr, Error: fmt.Errorf("pct not found in PATH")}
		writeProvisionState(instanceRoot, "", "error", r.Error.Error())
		return r
	}

	cmd := exec.Command("pct", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if err != nil {
		errMsg := output
		if errMsg == "" {
			errMsg = err.Error()
		}
		r := Result{Command: cmdStr, Error: fmt.Errorf("pct create failed: %s", errMsg)}
		writeProvisionState(instanceRoot, "", "error", errMsg)
		return r
	}

	// Extract VMID from output (pct create prints nothing on success if VMID given,
	// or prints the assigned VMID if 0 was used)
	vmid := spec.VMID
	if vmid == "" || vmid == "0" {
		vmid = strings.TrimSpace(output)
	}

	writeProvisionState(instanceRoot, vmid, "provisioned", "")
	return Result{VMID: vmid, Command: cmdStr}
}

func writeProvisionState(instanceRoot, vmid, status, errMsg string) {
	hs, _ := state.Read(instanceRoot)
	if hs == nil {
		hs = &state.HealthState{}
	}
	hs.VMID = vmid
	hs.ProvisionStatus = status
	hs.ProvisionedAt = time.Now().UTC().Format(time.RFC3339)
	hs.ProvisionError = errMsg
	state.Write(instanceRoot, *hs)
}

func resolveTemplate(base string) string {
	// Map common short names to Proxmox template paths
	switch {
	case strings.HasPrefix(base, "ubuntu:"):
		ver := strings.TrimPrefix(base, "ubuntu:")
		return fmt.Sprintf("local:vztmpl/ubuntu-%s-standard_amd64.tar.zst", strings.ReplaceAll(ver, ".", ""))
	case strings.HasPrefix(base, "debian:"):
		ver := strings.TrimPrefix(base, "debian:")
		return fmt.Sprintf("local:vztmpl/debian-%s-standard_amd64.tar.zst", ver)
	default:
		return base // pass through as-is (full template path)
	}
}

func sanitizeHostname(name string) string {
	r := strings.NewReplacer("/", "-", "_", "-", ".", "-")
	h := r.Replace(name)
	if len(h) > 63 {
		h = h[:63]
	}
	return h
}
