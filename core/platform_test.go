package core

import (
	"runtime"
	"sync"
	"testing"
)

func newPlatform() *PlatformIO {
	return &PlatformIO{}
}

// --- externalTun lifecycle ---

func TestExternalTun_InitiallyFalse(t *testing.T) {
	p := newPlatform()
	if p.externalTunActive() {
		t.Fatal("externalTun should be false initially")
	}
}

func TestSetTunFd_SetsExternalTun(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	if !p.externalTunActive() {
		t.Fatal("externalTun should be true after SetTunFd with non-zero fd")
	}
}

func TestSetTunFd_ZeroDoesNotSetExternalTun(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(0)
	if p.externalTunActive() {
		t.Fatal("externalTun should remain false when fd is 0")
	}
}

func TestResetTunFd_ClearsExternalTun(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	if !p.externalTunActive() {
		t.Fatal("setup: externalTun should be true")
	}
	p.ResetTunFd()
	if p.externalTunActive() {
		t.Fatal("externalTun should be false after ResetTunFd")
	}
}

// --- OpenTun consumes fd ---

func TestOpenTun_ReturnsExternalFd(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	fd, err := p.OpenTun(nil)
	if err != nil {
		t.Fatalf("OpenTun: %v", err)
	}
	if fd != 42 {
		t.Fatalf("OpenTun returned fd=%d, want 42", fd)
	}
}

func TestOpenTun_ConsumesFd(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	_, _ = p.OpenTun(nil)
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("second OpenTun should fail after fd was consumed")
	}
}

func TestOpenTun_PreservesExternalTun(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	_, _ = p.OpenTun(nil)
	// externalTun must persist so subsequent GetInterfaces/StartDefaultInterfaceMonitor
	// still skip netlink, even though the fd is gone.
	if !p.externalTunActive() {
		t.Fatal("externalTun should persist after OpenTun consumes fd")
	}
}

func TestOpenTun_NoFdReturnsError(t *testing.T) {
	p := newPlatform()
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("OpenTun without prior SetTunFd should return error")
	}
}

// --- GetInterfaces skips netlink when externalTun ---

func TestGetInterfaces_ExternalTunReturnsEmpty(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	if iter.HasNext() {
		t.Fatal("GetInterfaces should return empty iterator when externalTun is true")
	}
}

func TestGetInterfaces_NoExternalTunReturnsHostInterfaces(t *testing.T) {
	p := newPlatform()
	// Without external TUN, GetInterfaces calls net.Interfaces() which
	// should return at least the loopback on any system.
	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	// We don't assert non-empty since a sandboxed CI might have no interfaces,
	// but it should not panic or return an error.
	_ = iter
}

// --- StartDefaultInterfaceMonitor skips netlink when externalTun ---

func TestStartDefaultInterfaceMonitor_ExternalTunReturnsNil(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	// A nil listener is safe here because externalTun prevents the goroutine.
	err := p.StartDefaultInterfaceMonitor(nil)
	if err != nil {
		t.Fatalf("StartDefaultInterfaceMonitor: %v", err)
	}
}

// --- UnderNetworkExtension ---

func TestUnderNetworkExtension_NonIosReturnsFalse(t *testing.T) {
	if runtime.GOOS == "ios" {
		t.Skip("test only runs on non-iOS")
	}
	p := newPlatform()
	p.SetTunFd(42)
	if p.UnderNetworkExtension() {
		t.Fatal("UnderNetworkExtension should return false on non-iOS even with externalTun")
	}
}

// --- concurrency safety ---

func TestExternalTun_ConcurrentAccess(t *testing.T) {
	p := newPlatform()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); p.SetTunFd(int32(i%2) * 10) }()
		go func() { defer wg.Done(); p.externalTunActive() }()
		go func() { defer wg.Done(); p.ResetTunFd() }()
	}
	wg.Wait()
}
