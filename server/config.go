package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config mirrors config.yaml exactly as documented in docs/API_CONTRACT.md.
type Config struct {
	ListenAddr         string   `yaml:"listenAddr"`
	ServerName         string   `yaml:"serverName"`
	MediaPaths         []string `yaml:"mediaPaths"`
	DataDir            string   `yaml:"dataDir"`
	TokenTtlDays       int      `yaml:"tokenTtlDays"`
	RememberTtlDays    int      `yaml:"rememberTtlDays"`
	SignedUrlTtlHours  int      `yaml:"signedUrlTtlHours"`
	RescanIntervalHour int      `yaml:"rescanIntervalHours"`
	FFmpegPath         string   `yaml:"ffmpegPath"`
	ThumbGridEdge      int      `yaml:"thumbGridEdge"`
	ThumbFullEdge      int      `yaml:"thumbFullEdge"`
}

func defaultConfig() Config {
	return Config{
		ListenAddr:         "0.0.0.0:8080",
		ServerName:         "My NAS",
		MediaPaths:         nil,
		DataDir:            "data",
		TokenTtlDays:       30,
		RememberTtlDays:    180,
		SignedUrlTtlHours:  12,
		RescanIntervalHour: 6,
		FFmpegPath:         "",
		ThumbGridEdge:      480,
		ThumbFullEdge:      1280,
	}
}

// exampleConfigYAML is written by "nas-gallery init" when no config exists yet.
const exampleConfigYAML = `# nas-gallery configuration. See server/README.md for the full reference.
listenAddr: 0.0.0.0:8080
serverName: My NAS
mediaPaths: []            # list of library roots, e.g. ["/volume1/photo", "/volume1/video"]
dataDir: data             # DB, thumb cache and HMAC secret live here
tokenTtlDays: 30          # bearer token lifetime (rememberMe = false)
rememberTtlDays: 180      # bearer token lifetime (rememberMe = true)
signedUrlTtlHours: 12     # media URL signature lifetime
rescanIntervalHours: 6    # periodic library rescan interval
ffmpegPath: ""            # empty = auto-detect ffmpeg in PATH (optional, poster frames)
thumbGridEdge: 480        # grid thumbnail long edge, px
thumbFullEdge: 1280       # full-view thumbnail long edge, px
`

// LoadConfig reads a YAML config file over the defaults. Missing keys keep
// their default values; the file itself must exist.
func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, cfg.validate()
}

// writeExampleConfig writes the example config only if it does not exist yet.
// It reports whether the file was created.
func writeExampleConfig(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absOrSelf(path)), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(exampleConfigYAML), 0o600); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}
	return true, nil
}

func absOrSelf(p string) string {
	if p == "" {
		return "."
	}
	return filepath.Dir(p)
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("listenAddr must not be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return fmt.Errorf("dataDir must not be empty")
	}
	if c.TokenTtlDays <= 0 {
		c.TokenTtlDays = 30
	}
	if c.RememberTtlDays <= 0 {
		c.RememberTtlDays = 180
	}
	if c.SignedUrlTtlHours <= 0 {
		c.SignedUrlTtlHours = 12
	}
	if c.RescanIntervalHour <= 0 {
		c.RescanIntervalHour = 6
	}
	if c.ThumbGridEdge <= 0 {
		c.ThumbGridEdge = 480
	}
	if c.ThumbFullEdge <= 0 {
		c.ThumbFullEdge = 1280
	}
	// Resolve media paths to absolute, clean, deduplicated values.
	seen := make(map[string]bool, len(c.MediaPaths))
	paths := make([]string, 0, len(c.MediaPaths))
	for _, p := range c.MediaPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("resolve media path %q: %w", p, err)
		}
		abs = filepath.Clean(abs)
		if !seen[abs] {
			seen[abs] = true
			paths = append(paths, abs)
		}
	}
	c.MediaPaths = paths
	c.DataDir, _ = filepath.Abs(c.DataDir)
	c.DataDir = filepath.Clean(c.DataDir)
	return nil
}

func (c Config) dbPath() string    { return filepath.Join(c.DataDir, "nas-gallery.db") }
func (c Config) thumbsDir() string { return filepath.Join(c.DataDir, "thumbs") }
func (c Config) tokenTtl(remember bool) int {
	if remember {
		return c.RememberTtlDays
	}
	return c.TokenTtlDays
}
func (c Config) signedTTL() time.Duration {
	return time.Duration(c.SignedUrlTtlHours) * time.Hour
}
