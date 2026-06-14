package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// savedState remembers the last context and namespace so the next launch
// resumes where you left off. Neither value is sensitive.
type savedState struct {
	Context   string `json:"context"`
	Namespace string `json:"namespace"`
}

func stateFile() (string, error) {
	dir, err := kliConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func loadState() (savedState, bool) {
	p, err := stateFile()
	if err != nil {
		return savedState{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return savedState{}, false
	}
	var s savedState
	if json.Unmarshal(b, &s) != nil {
		return savedState{}, false
	}
	return s, true
}

func saveState(s savedState) {
	p, err := stateFile()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	if b, err := json.Marshal(s); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}
