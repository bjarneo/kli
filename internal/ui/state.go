package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// savedState remembers the last context, namespace, and theme so the next
// launch resumes where you left off. None of the values are sensitive.
type savedState struct {
	Context   string `json:"context"`
	Namespace string `json:"namespace"`
	Theme     string `json:"theme"`
}

func stateFile() (string, error) {
	return kuConfigFile("state.json")
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
