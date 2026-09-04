// Package ipc is the newline-delimited-JSON Unix-domain-socket channel
// between the pomo daemon (server) and the pomo TUI (client).
package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"

	"pomo/internal/db"
)

type Event struct {
	Type      string   `json:"type"`
	SessionID int64    `json:"id,omitempty"`
	RepoPath  string   `json:"repo_path,omitempty"`
	Text      string   `json:"text,omitempty"`
	Level     int      `json:"level,omitempty"`
	Actions   []string `json:"actions,omitempty"`
	Answer    string   `json:"answer,omitempty"`
	Action    string   `json:"action,omitempty"`
}

// SocketPath is the standard daemon socket location.
func SocketPath() string { return filepath.Join(db.Dir(), "daemon.sock") }

type Server struct {
	ln     net.Listener
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	onRecv func(Event)
}

// Serve removes any stale socket at path, then listens on it.
func Serve(path string, onRecv func(Event)) (*Server, error) {
	if _, err := os.Stat(path); err == nil {
		if c, derr := net.Dial("unix", path); derr == nil {
			c.Close()
			return nil, errors.New("ipc: another server is already listening on " + path)
		}
		_ = os.Remove(path)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, conns: map[net.Conn]struct{}{}, onRecv: onRecv}
	go s.acceptLoop()
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		go s.readLoop(conn)
	}
}

func (s *Server) readLoop(conn net.Conn) {
	defer s.drop(conn)
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if s.onRecv != nil {
			s.onRecv(e)
		}
	}
}

func (s *Server) drop(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	conn.Close()
}

// Broadcast sends e to every connected client. Write failures drop that client.
func (s *Server) Broadcast(e Event) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	s.mu.Lock()
	targets := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		if _, err := c.Write(line); err != nil {
			s.drop(c)
		}
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	err := s.ln.Close()
	_ = os.Remove(s.ln.Addr().String())
	return err
}

type Client struct {
	conn net.Conn
	enc  *json.Encoder
	mu   sync.Mutex
}

// Dial connects to the socket at path.
func Dial(path string, onRecv func(Event)) (*Client, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, enc: json.NewEncoder(conn)}
	go func() {
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 0, 4096), 1<<20)
		for sc.Scan() {
			var e Event
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			if onRecv != nil {
				onRecv(e)
			}
		}
	}()
	return c, nil
}

func (c *Client) Send(e Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(e) // Encoder appends '\n'
}

func (c *Client) Close() error { return c.conn.Close() }
