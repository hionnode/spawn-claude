package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Mode string

const (
	ModeLocal  Mode = "local"
	ModeDirect Mode = "direct"
)

type Config struct {
	Mode   Mode         `toml:"mode"`
	Direct DirectConfig `toml:"direct"`
}

type DirectConfig struct {
	Vendor string `toml:"vendor"`
}

func Default() Config {
	return Config{Mode: ModeLocal}
}

func (c Config) Validate() error {
	switch c.Mode {
	case ModeLocal, "":
		return nil
	case ModeDirect:
		if c.Direct.Vendor == "" {
			return errors.New(`mode = "direct" but [direct].vendor is empty; set it to a preset name (e.g. "signoz-cloud")`)
		}
		return nil
	default:
		return fmt.Errorf(`unknown mode %q (want "local" or "direct")`, c.Mode)
	}
}

type Paths struct {
	Home    string
	Dir     string
	File    string
	Secrets string
}

func ResolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "spawn-claude")
	return Paths{
		Home:    home,
		Dir:     dir,
		File:    filepath.Join(dir, "config.toml"),
		Secrets: filepath.Join(dir, "secrets.env"),
	}, nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Mode == "" {
		c.Mode = ModeLocal
	}
	return c, nil
}

func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
