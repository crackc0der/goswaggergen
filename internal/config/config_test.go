package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ProjectName != "API" {
		t.Errorf("default project name = %q, want API", cfg.ProjectName)
	}
	if cfg.Auth.JWT.SecurityName != "BearerAuth" {
		t.Errorf("default security name = %q, want BearerAuth", cfg.Auth.JWT.SecurityName)
	}
	if !cfg.Rules.NeverModifyExistingSwagger {
		t.Error("NeverModifyExistingSwagger should be true by default")
	}
}

func TestLoadCustomConfig(t *testing.T) {
	yamlContent := `
project_name: "Test API"
base_path: "/api/v2"
default_error_type: "dto.ErrorResponse"
auth:
  jwt:
    enabled: true
    security_name: "CustomAuth"
    header: "X-Auth"
    prefix: "Token "
rules:
  infer_request_body: true
  infer_response_body: false
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ProjectName != "Test API" {
		t.Errorf("project name = %q, want Test API", cfg.ProjectName)
	}
	if cfg.BasePath != "/api/v2" {
		t.Errorf("base path = %q, want /api/v2", cfg.BasePath)
	}
	if cfg.DefaultErrorType != "dto.ErrorResponse" {
		t.Errorf("default error type = %q, want dto.ErrorResponse", cfg.DefaultErrorType)
	}
	if cfg.Auth.JWT.SecurityName != "CustomAuth" {
		t.Errorf("security name = %q, want CustomAuth", cfg.Auth.JWT.SecurityName)
	}
	if cfg.Auth.JWT.Header != "X-Auth" {
		t.Errorf("auth header = %q, want X-Auth", cfg.Auth.JWT.Header)
	}
	if !cfg.Rules.InferRequestBody {
		t.Error("InferRequestBody should be true")
	}
	if cfg.Rules.InferResponseBody {
		t.Error("InferResponseBody should be false")
	}
}

func TestLoadEmptyConfig(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.ProjectName != "API" {
		t.Errorf("default project name = %q, want API", cfg.ProjectName)
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}
