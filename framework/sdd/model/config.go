package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultGraphDir is the conventional graph directory relative to repo root
	// when initialized with sdd init.
	DefaultGraphDir = ".sdd/graph"

	// SDDDirName is the metadata directory name at the repository root.
	SDDDirName = ".sdd"
)

// Config represents the contents of .sdd/config.yaml.
type Config struct {
	GraphDir string `yaml:"graph_dir"`
}

// ParseConfig unmarshals YAML bytes into a Config struct.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// FormatConfig returns a commented YAML config template with the given graph dir.
func FormatConfig(cfg Config) string {
	graphDir := cfg.GraphDir
	if graphDir == "" {
		graphDir = DefaultGraphDir
	}
	return "# SDD configuration\n" +
		"# See https://github.com/networkteam/sdd for documentation.\n" +
		"\n" +
		"# Graph directory relative to repository root.\n" +
		"graph_dir: " + graphDir + "\n"
}
