package main

// One Thought Controller host per data folder. A second copy pointed at the same
// folder refuses to start instead of racing the first one's writes.
// Ported verbatim (apart from names) from Arameair/identity
// experiment/identity-local-v1-20261023 @ 2b31e99 lock.go.
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type dataLock struct{ path string }

func acquireLock(dir, port string) (*dataLock, error) {
	p := filepath.Join(dir, "thought-controller.lock")
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d %s\n", os.Getpid(), port)
			f.Close()
			return &dataLock{path: p}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("can't create lock file in %s: %w", dir, err)
		}
		b, _ := os.ReadFile(p)
		fields := strings.Fields(string(b))
		pid := 0
		if len(fields) > 0 {
			pid, _ = strconv.Atoi(fields[0])
		}
		if pid > 0 && pid != os.Getpid() && processAlive(pid) {
			return nil, fmt.Errorf("another Thought Controller host (pid %d) is already using data folder %s — stop it first, or give this one its own data folder", pid, dir)
		}
		// left over from a crash: the process is gone, so take over
		_ = os.Remove(p)
	}
	return nil, fmt.Errorf("couldn't take the lock in %s", dir)
}

func (l *dataLock) release() {
	if l != nil {
		_ = os.Remove(l.path)
	}
}
