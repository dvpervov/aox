package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type State string

const (
	Working State = "WORKING"
	Waiting State = "WAITING"
	Idle    State = "IDLE"
	Unknown State = "UNKNOWN"
)

const (
	Claude = "claude"
	Codex  = "codex"
)

type Session struct {
	ID      string    `json:"id"`
	Agent   string    `json:"agent,omitempty"`
	Pane    string    `json:"pane"`
	State   State     `json:"state"`
	Prompt  string    `json:"prompt,omitempty"`
	Updated time.Time `json:"updated"`
}

func (s *Session) AgentName() string {
	if s.Agent == "" {
		return Claude
	}
	return s.Agent
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "state", "aox", "sessions")
	return dir, os.MkdirAll(dir, 0o700)
}

func path(id string) (string, error) {
	if id == "" || strings.HasPrefix(id, ".") || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func Load(id string) (*Session, error) {
	p, err := path(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Session
	return &s, json.Unmarshal(data, &s)
}

func List() ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var sessions []Session
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		s, err := Load(id)
		if err != nil || s == nil {
			continue
		}
		sessions = append(sessions, *s)
	}
	return sessions, nil
}

func Save(s *Session) error {
	p, err := path(s.ID)
	if err != nil {
		return err
	}
	s.Updated = time.Now()
	var data bytes.Buffer
	enc := json.NewEncoder(&data)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return err
	}
	return WriteFile(p, data.Bytes(), 0o600)
}

func Remove(id string) error {
	p, err := path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func Touch() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	now := time.Now()
	return os.Chtimes(dir, now, now)
}

func WriteFile(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
