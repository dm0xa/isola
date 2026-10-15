package orchestrator

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// SessionContainerSecurityParams configures docker run hardening for a browser session.
type SessionContainerSecurityParams struct {
	Resolution     string
	GPUPassthrough bool // host has renderD128 and operator set ISOLA_GPU=1
}

// Default per-session container limits — overridable per deployment with the
// ISOLA_SESSION_* environment variables (see sessionResourceLimits).
const (
	defaultSessionMemoryMB = 2048 // 2 GB
	defaultSessionCPUs     = 3
	defaultSessionPids     = 512
)

// defaultSessionCPUsForHost is the per-session CPU default for THIS host. The
// Docker daemon refuses `--cpus` above the machine's logical core count
// ("range of CPUs is from 0.01 to N.00, as there are only N CPUs available"),
// so a fixed default makes every session fail on a host with fewer cores — the
// failure surfaces as an opaque Docker exit 125, not as a config error. Small
// hosts therefore get their own core count; larger ones keep the default.
func defaultSessionCPUsForHost() float64 {
	if cores := float64(runtime.NumCPU()); cores < defaultSessionCPUs {
		return cores
	}
	return defaultSessionCPUs
}

// sessionResourceLimits resolves the per-session container limits from the
// environment, falling back to the hardened defaults. Non-positive or
// unparsable values are ignored, so a typo cannot silently drop the limits:
//
//	ISOLA_SESSION_MEMORY_MB   RAM in MB  (default 2048)
//	ISOLA_SESSION_CPUS        CPU count  (default min(3, host cores))
//	ISOLA_SESSION_PIDS        PID limit  (default 512)
//
// An explicit ISOLA_SESSION_CPUS is used as given, so it must not exceed the
// host's core count — the daemon rejects the container otherwise.
func sessionResourceLimits() (memoryMB int, cpus float64, pids int) {
	return envPositiveInt("ISOLA_SESSION_MEMORY_MB", defaultSessionMemoryMB),
		envPositiveFloat("ISOLA_SESSION_CPUS", defaultSessionCPUsForHost()),
		envPositiveInt("ISOLA_SESSION_PIDS", defaultSessionPids)
}

func envPositiveInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && v > 0 {
		return v
	}
	return def
}

func envPositiveFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64); err == nil && v > 0 {
		return v
	}
	return def
}

// sessionContainerSecurityArgs returns docker run flags for capability dropping,
// read-only rootfs, tmpfs mounts, loopback-only port publish, and optional GPU.
// The memory, CPU and PID limits come from the ISOLA_SESSION_* environment
// variables (defaults: 2 GB, 3 CPUs, 512 PIDs).
func sessionContainerSecurityArgs(p SessionContainerSecurityParams) []string {
	memoryMB, cpus, pids := sessionResourceLimits()
	args := []string{
		"-p", "127.0.0.1::5900",
		"-p", "127.0.0.1::9223",
		// Capability hardening (VULN-10). Caps stay on PID 1 (entrypoint): it
		// installs iptables, then session processes run as isolauser via gosu
		// (setuid clears effective caps on those processes).
		//
		// SYS_ADMIN is deliberately NOT granted. The sandbox mounts nothing,
		// and SYS_ADMIN is precisely what would let a compromised root process
		// undo Docker's read-only /proc masks (umount /proc/sys, /proc/kcore),
		// write a global sysctl (kernel.core_pattern), or map /proc/kcore — the
		// container-to-host escape surface that the read-only rootfs and masked
		// paths otherwise close. It is not needed: the entrypoint applies
		// iptables with NET_ADMIN, drops to isolauser with SETUID/SETGID/CHOWN,
		// and runs the browser pipeline without it. Omitting it means no process
		// in the container can mount, unmount, or write a sysctl at all.
		"--cap-drop", "ALL",
		"--cap-add", "SETUID",
		"--cap-add", "SETGID",
		"--cap-add", "CHOWN",
		"--cap-add", "DAC_OVERRIDE",
		"--cap-add", "IPC_LOCK",
		"--cap-add", "KILL",
		"--cap-add", "NET_BIND_SERVICE",
		"--cap-add", "NET_ADMIN",
		"--security-opt", "no-new-privileges",
		"--memory", fmt.Sprintf("%dm", memoryMB),
		"--memory-swap", fmt.Sprintf("%dm", memoryMB),
		"--pids-limit", strconv.Itoa(pids),
		"--cpus", strconv.FormatFloat(cpus, 'f', -1, 64),
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=256m",
		"--tmpfs", "/var/run:rw,noexec,nosuid,nodev,size=64m",
		"--tmpfs", "/run:rw,noexec,nosuid,nodev,size=64m",
		"--tmpfs", "/home/isolauser:rw,noexec,nosuid,nodev,size=512m",
		"--tmpfs", fmt.Sprintf("/dev/shm:rw,noexec,nosuid,nodev,size=%dm", shmBudgetMB(p.Resolution)),
	}
	if p.GPUPassthrough {
		args = append(args, "--device", "/dev/dri:/dev/dri")
	}
	return args
}
