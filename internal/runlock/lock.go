package runlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrAlreadyRunning = errors.New("git-doc is already running")

type Lock struct {
	path string
}

type lockPayload struct {
	PID       int    `json:"pid"`
	CreatedAt string `json:"created_at"`
}

func Acquire(repoRoot string) (*Lock, error) {
	lockPath := filepath.Join(repoRoot, ".git-doc", "run.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	if _, err := os.Stat(lockPath); err == nil {
		pid, parseErr := readPID(lockPath)
		if parseErr == nil && processAlive(pid) {
			return nil, fmt.Errorf("%w (pid=%d)", ErrAlreadyRunning, pid)
		}

		if rmErr := os.Remove(lockPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale lock: %w", rmErr)
		}
	}

	payload := lockPayload{PID: os.Getpid(), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create lock file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(b); err != nil {
		return nil, fmt.Errorf("write lock file: %w", err)
	}

	return &Lock{path: lockPath}, nil
}

func IsAlreadyRunningError(err error) bool {
	return errors.Is(err, ErrAlreadyRunning)
}

func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func readPID(lockPath string) (int, error) {
	b, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, err
	}

	var payload lockPayload
	if err := json.Unmarshal(b, &payload); err == nil && payload.PID > 0 {
		return payload.PID, nil
	}

	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return 0, fmt.Errorf("empty lock file")
	}

	pid, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}
