package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UserPreferences stores user-level settings like preferred editor.
type UserPreferences struct {
	DefaultEditor string `json:"default_editor,omitempty"`
}

func getPreferencesPath() (string, error) {
	grydDir, err := GetGrydDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(grydDir, "preferences.json"), nil
}

// LoadUserPreferences reads ~/.gryd/preferences.json.
func LoadUserPreferences() UserPreferences {
	var prefs UserPreferences
	prefPath, err := getPreferencesPath()
	if err != nil {
		return prefs
	}

	data, err := os.ReadFile(prefPath)
	if err != nil {
		return prefs
	}

	_ = json.Unmarshal(data, &prefs)
	return prefs
}

// SaveUserPreferences writes ~/.gryd/preferences.json.
func SaveUserPreferences(prefs UserPreferences) error {
	prefPath, err := getPreferencesPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(prefPath, data, 0644)
}

// GetDefaultEditor returns the preferred editor if saved, or empty string.
func GetDefaultEditor() string {
	prefs := LoadUserPreferences()
	return prefs.DefaultEditor
}

// SetDefaultEditor updates the default editor in ~/.gryd/preferences.json.
func SetDefaultEditor(editor string) error {
	prefs := LoadUserPreferences()
	prefs.DefaultEditor = editor
	return SaveUserPreferences(prefs)
}
