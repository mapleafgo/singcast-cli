package core

import (
	"runtime"
	"sync"
	"testing"
)

func newPlatform() *PlatformIO {
	return &PlatformIO{}
}

// --- SetTunFd / ResetTunFd ---

func TestSetTunFd_ZeroDoesNotPanic(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(0)
}

func TestResetTunFd_WithoutSetDoesNotPanic(t *testing.T) {
	p := newPlatform()
	p.ResetTunFd()
}

func TestResetTunFd_ClearsState(t *testing.T) {
	p := newPlatform()
	p.SetTunFd(42)
	p.ResetTunFd()
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("OpenTun should fail after ResetTunFd")
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

func TestOpenTun_NoFdReturnsError(t *testing.T) {
	p := newPlatform()
	_, err := p.OpenTun(nil)
	if err == nil {
		t.Fatal("OpenTun without prior SetTunFd should return error")
	}
}

// --- GetInterfaces ---

func TestGetInterfaces_OnDesktopReturnsHostInterfaces(t *testing.T) {
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("test only runs on desktop platforms")
	}
	p := newPlatform()
	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	_ = iter
}

func TestGetInterfaces_OnMobileReturnsEmpty(t *testing.T) {
	if runtime.GOOS != "android" && runtime.GOOS != "ios" {
		t.Skip("test only runs on mobile platforms")
	}
	p := newPlatform()
	iter, err := p.GetInterfaces()
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	if iter.HasNext() {
		t.Fatal("GetInterfaces should return empty iterator on mobile")
	}
}

// --- StartDefaultInterfaceMonitor ---

func TestStartDefaultInterfaceMonitor_OnMobileReturnsNil(t *testing.T) {
	if runtime.GOOS != "android" && runtime.GOOS != "ios" {
		t.Skip("test only runs on mobile platforms")
	}
	p := newPlatform()
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

func TestTunState_ConcurrentAccess(t *testing.T) {
	p := newPlatform()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); p.SetTunFd(int32(i%2) * 10) }()
		go func() { defer wg.Done(); p.UnderNetworkExtension() }()
		go func() { defer wg.Done(); p.ResetTunFd() }()
	}
	wg.Wait()
}
