package runlock

import "testing"

func TestAcquireRelease(t *testing.T) {
	repo := t.TempDir()

	lock, err := Acquire(repo)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	_, err = Acquire(repo)
	if err == nil {
		t.Fatalf("expected second acquire to fail while lock is active")
	}
	if !IsAlreadyRunningError(err) {
		t.Fatalf("expected already-running error, got: %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	lock2, err := Acquire(repo)
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	if err := lock2.Release(); err != nil {
		t.Fatalf("second release failed: %v", err)
	}
}
