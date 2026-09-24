package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T) *File {
	t.Helper()
	f, err := Load(filepath.Join("testdata", "models.yml"))
	if err != nil {
		t.Fatalf("Load fixture: %v", err)
	}
	return f
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFixture(t *testing.T) {
	f := loadFixture(t)
	p := f.Providers["alpha_openai_compat"]
	if p.BaseURL != "https://openai.example.test/v1" {
		t.Errorf("baseUrl = %q", p.BaseURL)
	}
	if p.API != "openai-completions" {
		t.Errorf("api = %q", p.API)
	}
	if p.APIKey != "test-key-123" {
		t.Errorf("list-shaped apiKey did not unwrap: %q", p.APIKey)
	}
	if p.Auth != "apiKey" {
		t.Errorf("auth = %q", p.Auth)
	}
	if p.Headers["X-Custom"] != "custom-value" {
		t.Errorf("headers = %v", p.Headers)
	}
	if len(p.Models) != 2 || p.Models[0].ID != "model-a" || p.Models[1].ID != "model-b" {
		t.Errorf("models = %+v", p.Models)
	}
	// Parsed-but-ignored omp flags.
	a := f.Providers["beta_anthropic_compat"]
	if !a.AuthHeader || !a.DisableStrictTools {
		t.Errorf("authHeader=%v disableStrictTools=%v", a.AuthHeader, a.DisableStrictTools)
	}
}

func TestResolveDefaults(t *testing.T) {
	f := loadFixture(t)
	r, err := f.Resolve("", "")
	if err != nil {
		t.Fatalf("Resolve defaults: %v", err)
	}
	if r.Provider != "alpha_openai_compat" {
		t.Errorf("default provider = %q, want alphabetically-first key", r.Provider)
	}
	if r.Model != "model-a" {
		t.Errorf("default model = %q, want first listed", r.Model)
	}
	if r.BaseURL != "https://openai.example.test/v1" || r.APIKey != "test-key-123" {
		t.Errorf("resolved = %+v", r)
	}
	if r.Headers["X-Custom"] != "custom-value" {
		t.Errorf("headers not copied: %v", r.Headers)
	}
}

func TestResolveExplicit(t *testing.T) {
	f := loadFixture(t)
	r, err := f.Resolve("alpha_openai_compat", "model-b")
	if err != nil {
		t.Fatalf("Resolve explicit: %v", err)
	}
	if r.Provider != "alpha_openai_compat" || r.Model != "model-b" {
		t.Errorf("resolved = %+v", r)
	}
}

func TestResolveErrors(t *testing.T) {
	f := loadFixture(t)
	cases := []struct {
		name, provider, model, want string
	}{
		{"unknown provider", "nope", "", `unknown provider "nope" (available: [alpha_openai_compat bearer beta_anthropic_compat nokey nomodels nourl secrets])`},
		{"unknown model", "alpha_openai_compat", "foo", `unknown model "foo" (available: [model-a model-b])`},
		{"unsupported api", "beta_anthropic_compat", "", `api "anthropic-messages" not supported, only "openai-completions"`},
		{"omp secret placeholder", "secrets", "", `omp secret store placeholder "Secrets34" is not supported`},
		{"unsupported auth", "bearer", "", `auth "bearer" not supported, only "apiKey"`},
		{"empty baseUrl", "nourl", "", "baseUrl is empty"},
		{"no models", "nomodels", "", "no models defined"},
		{"empty apiKey", "nokey", "", "apiKey is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.Resolve(tc.provider, tc.model)
			if err == nil {
				t.Fatalf("Resolve(%q,%q): want error", tc.provider, tc.model)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join("testdata", "missing.yml")); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Errorf("missing file: err = %v", err)
	}
	if _, err := Load(writeTemp(t, "providers: {}\n")); err == nil || !strings.Contains(err.Error(), "no providers defined") {
		t.Errorf("empty providers: err = %v", err)
	}
	if _, err := Load(writeTemp(t, "providers:\n  a:\n    baz: 1\n")); err == nil || !strings.Contains(err.Error(), "field baz not found") {
		t.Errorf("unknown field (strict): err = %v", err)
	}
}

func TestEnvExpansion(t *testing.T) {
	t.Setenv("K", "v")
	f, err := Load(writeTemp(t, "providers:\n  p:\n    baseUrl: u\n    api: openai-completions\n    apiKey: ${K}\n    models:\n      - id: m\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.Resolve("p", "m")
	if err != nil {
		t.Fatal(err)
	}
	if r.APIKey != "v" {
		t.Errorf("apiKey = %q, want expanded env value", r.APIKey)
	}
}
