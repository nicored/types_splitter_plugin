package types_splitter_plugin

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// PluginsCfg is a configuration for the plugin.
type PluginsCfg struct {
	Splitter *SplitterConfig `yaml:"types_splitter"`
}

// SplitterConfig is a configuration for splitting queries, mutations, subscriptions and types into multiple files.
type SplitterConfig struct {
	QueryConfig QuerySplitConfigs `yaml:"queries"`
	TypeConfig  TypeSplitConfigs  `yaml:"types"`
}

// QuerySplitConfig is a configuration for splitting queries and mutations into multiple files.
type QuerySplitConfig struct {
	// ResolverPrefix is the prefix that will be added to the resolver file name eg. racing => racing.queries.go.
	ResolverPrefix string `yaml:"prefix"`
	// Matches is a list of string regexes that will be used to match against the query name. They must be ordered by priority.
	Matches []string `yaml:"matches"`
	// matches is a list of compiled regexes that will be used to match against the query name.
	matches []*regexp.Regexp
}

type QuerySplitConfigs []QuerySplitConfig

// TypeSplitConfig is a configuration for splitting types into multiple files.
type TypeSplitConfig struct {
	// Name is the name of the type that will be used to match against the type name. eg. RacingRace
	Name string `yaml:"name"`
	// ResolverPrefix is the prefix that will be added to the resolver file name eg. racing_race => racing_race.resolvers.go.
	ResolverPrefix string `yaml:"prefix"`
}

type TypeSplitConfigs []TypeSplitConfig

func loadConfig(cfgFilePath string) (*SplitterConfig, error) {
	cfgFilePath, err := findCfg(cfgFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to find config: %w", err)
	}

	b, err := os.ReadFile(cfgFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read config: %w", err)
	}

	return readConfig(bytes.NewReader(b))
}

func readConfig(cfgFile io.Reader) (*SplitterConfig, error) {
	cfg := &PluginsCfg{}

	dec := yaml.NewDecoder(cfgFile)
	dec.KnownFields(false)

	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	if cfg.Splitter == nil {
		return nil, fmt.Errorf("no types_splitter config defined")
	}

	if len(cfg.Splitter.TypeConfig) == 0 && len(cfg.Splitter.QueryConfig) == 0 {
		return nil, fmt.Errorf("no type or query configs defined")
	}

	if err := cfg.Splitter.compileMatches(); err != nil {
		return nil, err
	}

	return cfg.Splitter, nil
}

// findCfg searches for the config file in this directory and all parents up the tree
// looking for the closest match.
// Copied from 99designs/gqlgen.
func findCfg(cfgName string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("unable to get working dir to findCfg: %w", err)
	}

	cfgPath := filepath.Join(dir, cfgName)
	if _, err = os.Stat(cfgPath); err == nil {
		return cfgPath, nil
	}

	for cfgPath == "" && dir != filepath.Dir(dir) {
		dir = filepath.Dir(dir)

		cfgPath = filepath.Join(dir, cfgName)
		if _, err = os.Stat(cfgPath); err == nil {
			return cfgPath, nil
		}
	}

	if cfgPath == "" {
		return "", os.ErrNotExist
	}

	return cfgPath, nil
}

func (c *SplitterConfig) compileMatches() error {
	for qi, queryCfg := range c.QueryConfig {
		if len(queryCfg.Matches) == 0 {
			return fmt.Errorf("no matches defined for query config %s", queryCfg.ResolverPrefix)
		}

		for _, match := range queryCfg.Matches {
			if strings.TrimSpace(match) == "" {
				return fmt.Errorf(`empty match regex "%s" for query config %s`, match, queryCfg.ResolverPrefix)
			}

			cmp, err := regexp.Compile(fmt.Sprintf("(?i)%s", match))
			if err != nil {
				return fmt.Errorf("invalid match regex %s for query config %s: %w", match, queryCfg.ResolverPrefix, err)
			}

			c.QueryConfig[qi].matches = append(c.QueryConfig[qi].matches, cmp)
		}
	}

	return nil
}

// FindResolverPrefix returns the resolver prefix for the given query name.
func (qs QuerySplitConfigs) FindResolverPrefix(queryName string) (string, bool) {
	for _, q := range qs {
		for _, m := range q.matches {
			if m.MatchString(queryName) {
				return q.ResolverPrefix, true
			}
		}
	}
	return "", false
}

// FindResolverPrefix returns the resolver prefix for the given type name.
func (ts TypeSplitConfigs) FindResolverPrefix(typeName string) (string, bool) {
	for _, t := range ts {
		if t.Name == typeName {
			return t.ResolverPrefix, true
		}
	}
	return "", false
}
