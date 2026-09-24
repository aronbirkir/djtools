package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aronbirkir/djtools/internal/prune"
)

// Config holds settings persisted between runs.
type Config struct {
	XMLPath       string    `json:"xml"`
	MusicDir      string    `json:"music"`
	Extensions    string    `json:"extensions"`
	MaxOrphanPct  float64   `json:"maxOrphanPct"`
	KeepEmptyDirs bool      `json:"keepEmptyDirs"`
	Theme         ThemeMode `json:"theme"`
}

// appDir is where the app keeps its settings and prune reports.
func appDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "djtools"), nil
}

// reportsDir holds one report per real run. It is ours, unlike the folder
// holding rekordbox.xml, which belongs to rekordbox.
func reportsDir() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reports"), nil
}

func reportPath(dir string, now time.Time) string {
	return filepath.Join(dir, "prune-"+now.Format("20060102-150405")+".txt")
}

func defaultConfig() Config {
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	return Config{
		XMLPath:      defaultXMLPath(runtime.GOOS, home, configDir),
		MusicDir:     filepath.Join(home, "Music", "rekordbox"),
		Extensions:   prune.DefaultExtensions,
		MaxOrphanPct: prune.DefaultMaxOrphanPct,
		Theme:        ThemeSystem,
	}
}

// defaultXMLPath is where rekordbox writes its XML export unless told
// otherwise. configDir is %AppData% on Windows.
func defaultXMLPath(goos, home, configDir string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Pioneer", "rekordbox", "rekordbox.xml")
	case "windows":
		return filepath.Join(configDir, "Pioneer", "rekordbox", "rekordbox.xml")
	}
	return ""
}

// expandHome resolves a leading ~ so typed paths behave as in a shell.
func expandHome(path, home string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func configPath() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func loadConfig() Config {
	defaults := defaultConfig()
	path, err := configPath()
	if err != nil {
		return defaults
	}
	return loadConfigFrom(path, defaults)
}

// loadConfigFrom overlays the saved settings on defaults, so a setting missing
// from an older file keeps its default rather than becoming zero. A corrupt
// file yields the defaults rather than a half-applied mix.
func loadConfigFrom(path string, defaults Config) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	cfg := defaults
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaults
	}
	return cfg
}

func saveConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	return saveConfigTo(path, cfg)
}

func saveConfigTo(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
