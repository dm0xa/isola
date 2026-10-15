package orchestrator

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSessionContainerSecurityArgs_baseline(t *testing.T) {
	t.Parallel()
	args := sessionContainerSecurityArgs(SessionContainerSecurityParams{
		Resolution:     "1920x1080",
		GPUPassthrough: false,
	})
	joined := strings.Join(args, " ")

	if !slices.Contains(args, "--cap-drop") || !slices.Contains(args, "ALL") {
		t.Fatal("expected cap-drop ALL")
	}
	// SYS_ADMIN re-opens the container-to-host escape surface (mount/umount,
	// global sysctl writes, /proc/kcore) that the read-only rootfs and masked
	// paths otherwise close, and the entrypoint does not need it.
	if strings.Contains(joined, "SYS_ADMIN") {
		t.Fatal("must not grant SYS_ADMIN to session containers")
	}
	if !slices.Contains(args, "127.0.0.1::5900") || !slices.Contains(args, "127.0.0.1::9223") {
		t.Fatal("expected loopback-only port publish")
	}
	if strings.Contains(joined, "seccomp=unconfined") {
		t.Fatal("must not disable seccomp")
	}
	if strings.Contains(joined, "/dev/dri") {
		t.Fatal("GPU device must not be passed when GPUPassthrough is false")
	}
	if !strings.Contains(joined, "--read-only") {
		t.Fatal("expected read-only rootfs")
	}
}

func TestSessionContainerSecurityArgs_exactCapSet(t *testing.T) {
	t.Parallel()
	args := sessionContainerSecurityArgs(SessionContainerSecurityParams{Resolution: "1280x720"})
	joined := strings.Join(args, " ")

	// Pin the exact capability set so a future edit cannot silently widen it.
	want := []string{
		"SETUID", "SETGID", "CHOWN", "DAC_OVERRIDE",
		"IPC_LOCK", "KILL", "NET_BIND_SERVICE", "NET_ADMIN",
	}
	for _, c := range want {
		if !strings.Contains(joined, "--cap-add "+c) {
			t.Errorf("expected --cap-add %s", c)
		}
	}
	for _, a := range args {
		if a == "SYS_ADMIN" || a == "SYS_RAWIO" || a == "SYS_PTRACE" || a == "SYS_MODULE" || a == "SETPCAP" {
			t.Errorf("capability %s must not be granted", a)
		}
	}
}

func TestSessionContainerSecurityArgs_gpuOptIn(t *testing.T) {
	t.Parallel()
	args := sessionContainerSecurityArgs(SessionContainerSecurityParams{
		Resolution:     "1280x720",
		GPUPassthrough: true,
	})
	if !slices.Contains(args, "--device") {
		t.Fatal("expected --device when GPUPassthrough is true")
	}
	idx := slices.Index(args, "--device")
	if idx < 0 || args[idx+1] != "/dev/dri:/dev/dri" {
		t.Fatalf("expected /dev/dri passthrough, got %v", args)
	}
}

func TestSessionContainerSecurityArgs_shmBudget(t *testing.T) {
	t.Parallel()
	args := sessionContainerSecurityArgs(SessionContainerSecurityParams{Resolution: "1920x1080"})
	var shm string
	for _, a := range args {
		if strings.HasPrefix(a, "/dev/shm:") {
			shm = a
			break
		}
	}
	if shm == "" {
		t.Fatal("missing /dev/shm tmpfs")
	}
	want := fmt.Sprintf("/dev/shm:rw,noexec,nosuid,nodev,size=%dm", shmBudgetMB("1920x1080"))
	if shm != want {
		t.Fatalf("shm tmpfs %q, want %q", shm, want)
	}
}

// Not parallel: t.Setenv mutates process env.
func TestSessionContainerSecurityArgs_resourceLimits(t *testing.T) {
	t.Setenv("ISOLA_SESSION_MEMORY_MB", "4096")
	t.Setenv("ISOLA_SESSION_CPUS", "1.5")
	t.Setenv("ISOLA_SESSION_PIDS", "2048")

	args := sessionContainerSecurityArgs(SessionContainerSecurityParams{Resolution: "1280x720"})
	joined := strings.Join(args, " ")
	for _, want := range []string{"--memory 4096m", "--memory-swap 4096m", "--cpus 1.5", "--pids-limit 2048"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

// A bad value must not remove a limit — it falls back to the default.
func TestSessionContainerSecurityArgs_invalidLimitsFallBack(t *testing.T) {
	t.Setenv("ISOLA_SESSION_MEMORY_MB", "nope")
	t.Setenv("ISOLA_SESSION_CPUS", "-1")
	t.Setenv("ISOLA_SESSION_PIDS", "0")

	wantCPUs := defaultSessionCPUsForHost()
	mem, cpus, pids := sessionResourceLimits()
	if mem != defaultSessionMemoryMB || cpus != wantCPUs || pids != defaultSessionPids {
		t.Fatalf("got (%d, %v, %d), want defaults (%d, %v, %d)",
			mem, cpus, pids, defaultSessionMemoryMB, wantCPUs, defaultSessionPids)
	}
}

// The CPU default must never exceed the host's core count: the Docker daemon
// rejects `--cpus` above it, so an oversized default fails every session on a
// small host with an opaque exit 125.
func TestDefaultSessionCPUsFitsHost(t *testing.T) {
	t.Parallel()
	got := defaultSessionCPUsForHost()
	if cores := float64(runtime.NumCPU()); got > cores {
		t.Fatalf("default cpus %.1f exceeds host cores %v", got, cores)
	}
	if got > defaultSessionCPUs {
		t.Fatalf("default cpus %.1f exceeds the hardened default %v", got, defaultSessionCPUs)
	}
	if got <= 0 {
		t.Fatalf("default cpus must be positive, got %.1f", got)
	}
}
