package alloy

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	Label   = "com.grafana.alloy"
	UIBase  = "http://127.0.0.1:12345"
	ReadyURL = UIBase + "/-/ready"
	ReloadURL = UIBase + "/-/reload"
)

type Paths struct {
	Home       string
	BinDir     string
	BinPath    string
	ConfigDir  string
	ConfigFile string
	DataDir    string
	LogDir     string
	StderrLog  string
	StdoutLog  string
	AgentDir   string
	AgentPath  string
}

func ResolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	configDir := filepath.Join(home, ".config", "alloy")
	logDir := filepath.Join(home, "Library", "Logs", "alloy")
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	return Paths{
		Home:       home,
		BinDir:     filepath.Join(home, ".local", "bin"),
		BinPath:    filepath.Join(home, ".local", "bin", "alloy"),
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, "config.alloy"),
		DataDir:    filepath.Join(configDir, "data"),
		LogDir:     logDir,
		StderrLog:  filepath.Join(logDir, "stderr.log"),
		StdoutLog:  filepath.Join(logDir, "stdout.log"),
		AgentDir:   agentDir,
		AgentPath:  filepath.Join(agentDir, Label+".plist"),
	}, nil
}

func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.BinDir, p.DataDir, p.LogDir, p.AgentDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}
