package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dm0xa/isola/backend/pkg/config"
)

// newTestPool builds a WarmPool whose provisioner/prober are fakes, so the
// claim/reconcile logic is testable without a Docker daemon.
func newTestPool(t *testing.T, size int) (*WarmPool, *[]string, *[]string) {
	t.Helper()
	cfg := &config.Config{
		WarmPoolEnabled: true,
		WarmPoolSize:    size,
		Resolution:      "1920x1080",
		BrowserTheme:    "dark",
	}
	p := NewWarmPool(nil, cfg)

	var fakeMu sync.Mutex
	created, terminated := &[]string{}, &[]string{}
	seq := 0
	p.provisioner = func(ctx context.Context, params WarmPoolParams) (*SessionInfo, error) {
		fakeMu.Lock()
		defer fakeMu.Unlock()
		seq++
		id := "warm-container-" + string(rune('a'+seq-1))
		*created = append(*created, id)
		return &SessionInfo{
			SessionID:   params.SessionID,
			ContainerID: id,
			Host:        "127.0.0.1",
			Port:        5900,
			CdpPort:     9223,
		}, nil
	}
	p.prober = func(ctx context.Context, info *SessionInfo) error {
		fakeMu.Lock()
		defer fakeMu.Unlock()
		for _, id := range *terminated {
			if id == info.ContainerID {
				return errors.New("container dead")
			}
		}
		return nil
	}
	p.stop = func(ctx context.Context, containerID string) {
		fakeMu.Lock()
		defer fakeMu.Unlock()
		*terminated = append(*terminated, containerID)
	}
	p.navigator = func(ctx context.Context, host string, port int, targetURL string) error {
		return nil
	}
	return p, created, terminated
}

func TestWarmPoolDisabledByDefault(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{WarmPoolEnabled: false, WarmPoolSize: 1, Resolution: "1920x1080", BrowserTheme: "dark"}
	p := NewWarmPool(nil, cfg)
	if got := p.Claim(context.Background(), "1920x1080", "dark", 30); got != nil {
		t.Fatalf("Claim on disabled pool = %+v, want nil", got)
	}
}

func TestWarmPoolClaimMatchesExactParams(t *testing.T) {
	t.Parallel()
	p, created, terminated := newTestPool(t, 1)

	p.fillOne(context.Background())
	if len(*created) != 1 {
		t.Fatalf("provisioner calls = %d, want 1", len(*created))
	}

	// Wrong resolution must not claim and must not kill the container.
	if got := p.Claim(context.Background(), "1280x720", "dark", 30); got != nil {
		t.Fatalf("Claim with mismatched resolution = %+v, want nil", got)
	}
	// Wrong theme ditto.
	if got := p.Claim(context.Background(), "1920x1080", "light", 30); got != nil {
		t.Fatalf("Claim with mismatched theme = %+v, want nil", got)
	}
	// Matching params claim the container.
	if got := p.Claim(context.Background(), "1920x1080", "dark", 30); got == nil {
		t.Fatal("Claim with matching params = nil, want container")
	}
	if len(*terminated) != 0 {
		t.Fatalf("terminated = %v, want none (claimed containers stay alive)", *terminated)
	}

	// The claim must start a replacement immediately: a browser boot takes tens
	// of seconds, so waiting for the next reconcile tick means the next user
	// pays the cold path too.
	waitForPoolSize(t, p, 1, 3*time.Second)
}

// waitForPoolSize polls until the pool holds want containers or the deadline
// passes. Used for the async refill paths.
func waitForPoolSize(t *testing.T, p *WarmPool, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		p.mu.Lock()
		n := len(p.warm)
		p.mu.Unlock()
		if n >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pool size = %d, want %d within %v", n, want, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWarmPoolEmptyPoolClaimMisses(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)
	// Provisioning keeps failing, so the pool never fills and claims must miss
	// (and must not tear anything down).
	p.provisioner = func(ctx context.Context, params WarmPoolParams) (*SessionInfo, error) {
		return nil, errors.New("no capacity")
	}

	p.reconcile(context.Background())

	if got := p.Claim(context.Background(), "1920x1080", "dark", 30); got != nil {
		t.Fatalf("Claim on empty pool = %+v, want nil", got)
	}
	if len(*terminated) != 0 {
		t.Fatalf("terminated = %v, want none", *terminated)
	}
}

func TestWarmPoolClaimDropsDeadContainer(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)

	p.fillOne(context.Background())
	// The fake prober treats the container as dead once it appears in the
	// terminated list, so kill it manually and observe the claim probe.
	claimID := p.warm[0].info.ContainerID
	*terminated = append(*terminated, claimID)

	if got := p.Claim(context.Background(), "1920x1080", "dark", 30); got != nil {
		t.Fatalf("Claim of dead container = %+v, want nil", got)
	}
	// The claim probe retries once, then tears the container down.
	if len(*terminated) != 2 {
		t.Fatalf("terminated = %v, want the dead container torn down", *terminated)
	}
}

func TestWarmPoolReconcileRefillsAndPrunes(t *testing.T) {
	t.Parallel()
	p, created, terminated := newTestPool(t, 2)

	p.reconcile(context.Background())
	if len(*created) != 2 {
		t.Fatalf("created = %v, want 2 warm containers", *created)
	}

	// Kill one. A single failed probe is only a strike (mirroring the session
	// health checker), so the container must survive the first pass.
	p.mu.Lock()
	victim := p.warm[0].info.ContainerID
	p.mu.Unlock()
	*terminated = append(*terminated, victim)

	p.reconcile(context.Background())
	if len(*created) != 2 {
		t.Fatalf("created = %v, want 2 (one strike must not replace)", *created)
	}
	if !poolTracks(p, victim) {
		t.Fatal("one failed probe pruned a container; a transient probe failure must not cost a warm container")
	}

	// The second consecutive failure prunes the dead container and replaces it.
	p.reconcile(context.Background())
	if len(*created) != 3 {
		t.Fatalf("created = %v, want 3 (one replacement)", *created)
	}
	if poolTracks(p, victim) {
		t.Fatal("dead container still tracked by the pool after two strikes")
	}
	p.mu.Lock()
	tracked := len(p.warm)
	p.mu.Unlock()
	if tracked != 2 {
		t.Fatalf("pool size = %d, want 2", tracked)
	}
}

// poolTracks reports whether the pool still holds a container by id.
func poolTracks(p *WarmPool, containerID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, wc := range p.warm {
		if wc.info.ContainerID == containerID {
			return true
		}
	}
	return false
}

func TestWarmPoolNavigateRequest(t *testing.T) {
	t.Parallel()
	// The streamer /navigate endpoint receives the escaped target URL. This
	// locks the wire format the browser image implements.
	target := "https://example.com/?q=a b"
	req := navigateRequestURL("127.0.0.1", 5900, target)
	want := "http://127.0.0.1:5900/navigate?url=" + "https%3A%2F%2Fexample.com%2F%3Fq%3Da+b"
	if req != want {
		t.Fatalf("navigate URL = %q, want %q", req, want)
	}
}

func TestWarmPoolDisableOnUnsupportedNavigate(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestPool(t, 1)
	p.NotifyNavigateDisabled()
	if !p.Disabled() {
		t.Fatal("pool should be disabled after NotifyNavigateDisabled")
	}
	if got := p.Claim(context.Background(), "1920x1080", "dark", 30); got != nil {
		t.Fatalf("Claim on disabled pool = %+v, want nil", got)
	}
}

func TestWarmPoolFillDisablesOnOldImage(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)
	// Simulate a browser image whose streamer predates /navigate: the
	// capability probe must disable the pool and never publish the container.
	p.navigator = func(ctx context.Context, host string, port int, targetURL string) error {
		return ErrNavigateUnsupported
	}

	p.fillOne(context.Background())

	if !p.Disabled() {
		t.Fatal("pool should disable itself when the image cannot navigate")
	}
	if len(p.warm) != 0 {
		t.Fatalf("pool holds %d containers, want 0 (unusable image)", len(p.warm))
	}
	if len(*terminated) != 1 {
		t.Fatalf("terminated = %v, want the unusable container stopped", *terminated)
	}
}

func TestWarmPoolDrainStopsAll(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 2)
	p.fillOne(context.Background())
	p.fillOne(context.Background())
	p.Drain(context.Background())
	if len(*terminated) != 2 {
		t.Fatalf("terminated = %v, want both warm containers stopped", *terminated)
	}
	if len(p.warm) != 0 {
		t.Fatalf("pool holds %d containers after drain, want 0", len(p.warm))
	}
	// After draining, the pool must stop serving and stop filling.
	if !p.Disabled() {
		t.Fatal("pool should be disabled after Drain")
	}
	if got := p.Claim(context.Background(), "1920x1080", "dark", 30); got != nil {
		t.Fatalf("Claim after Drain = %+v, want nil", got)
	}
	// A second Drain is a no-op, not a double-stop of the same container.
	p.Drain(context.Background())
	if len(*terminated) != 2 {
		t.Fatalf("terminated = %v, want no additional stops", *terminated)
	}
}

// TestWarmPoolDrainStopsInFlightFill pins the mid-fill shutdown case: the
// container exists on the host (the daemon created it, provisionWarm marked it
// pending) but its fill has not published it yet. Drain must still stop it, or
// it lingers until the next start's leftover sweep.
func TestWarmPoolDrainStopsInFlightFill(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)

	// A fill is mid-boot: the container exists but is not in the pool yet.
	p.markPending("warm-inflight")
	if len(p.warm) != 0 {
		t.Fatalf("precondition: pool should be empty, has %d", len(p.warm))
	}

	p.Drain(context.Background())

	if len(*terminated) != 1 || (*terminated)[0] != "warm-inflight" {
		t.Fatalf("terminated = %v, want the in-flight container stopped", *terminated)
	}
}

func TestWarmPoolOrphanCollectorKeepsActiveSessions(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)
	// Two warm-labeled containers survived a backend restart; one is backing
	// a live session (in the DB), the other is a true leftover.
	leftover, live := "warm-leftover", "warm-live-session"
	p.sweepLister = func(ctx context.Context) []string { return []string{leftover, live} }
	p.SetActiveContainers(func(ctx context.Context) (map[string]bool, error) {
		return map[string]bool{live: true}, nil
	})

	p.collectOrphans(context.Background())

	if len(*terminated) != 1 || (*terminated)[0] != leftover {
		t.Fatalf("terminated = %v, want exactly the leftover container", *terminated)
	}
}

func TestWarmPoolOrphanCollectorKeepsPooledContainers(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)

	p.fillOne(context.Background())
	pooled := p.warm[0].info.ContainerID
	p.sweepLister = func(ctx context.Context) []string { return []string{pooled, "warm-orphan"} }

	p.collectOrphans(context.Background())

	if len(*terminated) != 1 || (*terminated)[0] != "warm-orphan" {
		t.Fatalf("terminated = %v, want only the untracked container", *terminated)
	}
}

func TestWarmPoolOrphanCollectorSkipsWhileFilling(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)
	// A fill is in flight: the container it just created is legitimately
	// untracked until CreateSession returns, so nothing may be collected.
	p.filling = 1
	p.sweepLister = func(ctx context.Context) []string { return []string{"warm-mid-boot"} }

	p.collectOrphans(context.Background())

	if len(*terminated) != 0 {
		t.Fatalf("terminated = %v, want none while a fill is in flight", *terminated)
	}
}

func TestWarmPoolOrphanCollectorSkipsOnLookupError(t *testing.T) {
	t.Parallel()
	p, _, terminated := newTestPool(t, 1)
	p.sweepLister = func(ctx context.Context) []string { return []string{"warm-a"} }
	p.SetActiveContainers(func(ctx context.Context) (map[string]bool, error) {
		return nil, errors.New("db down")
	})

	p.collectOrphans(context.Background())

	if len(*terminated) != 0 {
		t.Fatalf("terminated = %v, want none (no DB answer means no safe collection)", *terminated)
	}
}
