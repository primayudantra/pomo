package ipc_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"pomo/internal/ipc"
)

// sockPath returns a short socket path — macOS caps AF_UNIX paths near 104
// bytes, and t.TempDir() paths are too long.
func sockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pi")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir + "/s"
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

func TestBroadcastReachesAllClients(t *testing.T) {
	path := sockPath(t)
	srv, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	var mu sync.Mutex
	got := map[string]int{}
	recv := func(name string) func(ipc.Event) {
		return func(e ipc.Event) {
			mu.Lock()
			got[name+":"+e.Type]++
			mu.Unlock()
		}
	}
	c1, err := ipc.Dial(path, recv("c1"))
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := ipc.Dial(path, recv("c2"))
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	time.Sleep(100 * time.Millisecond)
	srv.Broadcast(ipc.Event{Type: "nudge", Text: "hi"})

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return got["c1:nudge"] == 1 && got["c2:nudge"] == 1
	})
}

func TestClientSendReachesServerHandler(t *testing.T) {
	path := sockPath(t)
	var mu sync.Mutex
	var last ipc.Event
	srv, err := ipc.Serve(path, func(e ipc.Event) {
		mu.Lock()
		last = e
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c, err := ipc.Dial(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Send(ipc.Event{Type: "checkpoint-answer", Answer: "y"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return last.Type == "checkpoint-answer" && last.Answer == "y"
	})
}

func TestServeCleansStaleSocket(t *testing.T) {
	path := sockPath(t)
	srv1, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv1.Close()

	srv2, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatalf("Serve over stale socket: %v", err)
	}
	srv2.Close()
}

func TestOneClientDisconnectDoesNotBreakOthers(t *testing.T) {
	path := sockPath(t)
	srv, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c1, _ := ipc.Dial(path, nil)
	var mu sync.Mutex
	n := 0
	c2, _ := ipc.Dial(path, func(ipc.Event) { mu.Lock(); n++; mu.Unlock() })
	defer c2.Close()

	time.Sleep(100 * time.Millisecond)
	c1.Close()
	time.Sleep(50 * time.Millisecond)
	srv.Broadcast(ipc.Event{Type: "watching"})

	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return n == 1 })
}
