// Package config loads the agent_soup model registry from a YAML file using
// the same format as omp's ~/.omp/agent/models.yml, restricted to the subset
// agent_soup supports: OpenAI-compatible (chat completions) providers.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// File is the top-level document of models.yml.
type File struct {
	Providers map[string]Provider `yaml:"providers"`
}

// Provider describes one OpenAI-compatible endpoint and its models.
type Provider struct {
	BaseURL string `yaml:"baseUrl"`
	API     string `yaml:"api"`
	APIKey  Secret `yaml:"apiKey"`
	Auth    string `yaml:"auth"`
	// AuthHeader is parsed for omp compatibility but ignored: it only
	// affects anthropic-messages providers, which agent_soup does not support.
	AuthHeader bool `yaml:"authHeader"`
	// DisableStrictTools is parsed for omp compatibility but ignored: it only
	// affects anthropic-messages providers, which agent_soup does not support.
	DisableStrictTools bool              `yaml:"disableStrictTools"`
	Headers            map[string]string `yaml:"headers"`
	Models             []ModelDef        `yaml:"models"`
}

// ModelDef is one model entry under a provider.
type ModelDef struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// Resolved is a fully resolved provider+model pair, ready for use by
// internal/openaicompat.
type Resolved struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
	Headers  map[string]string
}

// Secret is an API key: a plain scalar, or a one-element flow sequence as
// written by omp ([Secrets33]). It marshals back as a plain scalar.
type Secret string

// UnmarshalYAML accepts a scalar or a one-element flow sequence (omp writes
// apiKey: [Secrets33]); the element is unwrapped.
func (s *Secret) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = Secret(value.Value)
		return nil
	case yaml.SequenceNode:
		if len(value.Content) != 1 {
			return fmt.Errorf("apiKey: expected a scalar or a single-element list, got %d elements", len(value.Content))
		}
		el := value.Content[0]
		if el.Kind != yaml.ScalarNode {
			return fmt.Errorf("apiKey: list element must be a scalar")
		}
		*s = Secret(el.Value)
		return nil
	default:
		return fmt.Errorf("apiKey: expected a scalar or a single-element list, got %v", value.Tag)
	}
}

// MarshalYAML writes the secret back as a plain scalar.
func (s Secret) MarshalYAML() (any, error) {
	return string(s), nil
}

var ompSecretRe = regexp.MustCompile(`^\[?Secrets[0-9]+\]?$`)

// resolve expands ${ENV_VAR} references; omp secret-store placeholders such
// as [Secrets33] are rejected because agent_soup has no access to omp's
// secret store.
func (s Secret) resolve() (string, error) {
	v := string(s)
	if ompSecretRe.MatchString(v) {
		return "", fmt.Errorf("omp secret store placeholder %q is not supported; use a literal key or ${ENV_VAR}", v)
	}
	return os.ExpandEnv(v), nil
}

// Load reads and strictly parses the models file at path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if len(f.Providers) == 0 {
		return nil, fmt.Errorf("config %s: no providers defined", path)
	}
	return &f, nil
}

// Resolve picks a provider and model. Empty providerName selects the
// alphabetically-first provider key; empty modelName selects the provider's
// first listed model. Everything else must match explicitly.
func (f *File) Resolve(providerName, modelName string) (Resolved, error) {
	if len(f.Providers) == 0 {
		return Resolved{}, errors.New("no providers defined")
	}
	keys := make([]string, 0, len(f.Providers))
	for k := range f.Providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if providerName == "" {
		providerName = keys[0]
	}
	p, ok := f.Providers[providerName]
	if !ok {
		return Resolved{}, fmt.Errorf("unknown provider %q (available: %v)", providerName, keys)
	}
	if p.API != "" && p.API != "openai-completions" {
		return Resolved{}, fmt.Errorf("provider %s: api %q not supported, only \"openai-completions\"", providerName, p.API)
	}
	if p.Auth != "" && p.Auth != "apiKey" {
		return Resolved{}, fmt.Errorf("provider %s: auth %q not supported, only \"apiKey\"", providerName, p.Auth)
	}
	if p.BaseURL == "" {
		return Resolved{}, fmt.Errorf("provider %s: baseUrl is empty", providerName)
	}
	if len(p.Models) == 0 {
		return Resolved{}, fmt.Errorf("provider %s: no models defined", providerName)
	}
	if modelName == "" {
		modelName = p.Models[0].ID
	}
	found := false
	for _, m := range p.Models {
		if m.ID == modelName {
			found = true
			break
		}
	}
	if !found {
		ids := make([]string, 0, len(p.Models))
		for _, m := range p.Models {
			ids = append(ids, m.ID)
		}
		return Resolved{}, fmt.Errorf("provider %s: unknown model %q (available: %v)", providerName, modelName, ids)
	}
	apiKey, err := p.APIKey.resolve()
	if err != nil {
		return Resolved{}, fmt.Errorf("provider %s: %w", providerName, err)
	}
	if apiKey == "" {
		return Resolved{}, fmt.Errorf("provider %s: apiKey is empty", providerName)
	}
	return Resolved{
		Provider: providerName,
		Model:    modelName,
		BaseURL:  p.BaseURL,
		APIKey:   apiKey,
		Headers:  p.Headers,
	}, nil
}
