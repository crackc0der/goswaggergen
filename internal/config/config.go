package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type AuthConfig struct {
	JWT JWTConfig `yaml:"jwt"`
}

type JWTConfig struct {
	Enabled      bool   `yaml:"enabled"`
	SecurityName string `yaml:"security_name"`
	Header       string `yaml:"header"`
	Prefix       string `yaml:"prefix"`
}

type OutputConfig struct {
	Swaggo          bool   `yaml:"swaggo"`
	OpenAPIVersion  string `yaml:"openapi_version"`
}

type RulesConfig struct {
	NeverModifyExistingSwagger bool `yaml:"never_modify_existing_swagger"`
	GenerateForPrivateHandlers bool `yaml:"generate_for_private_handlers"`
	IncludeInternalErrors      bool `yaml:"include_internal_errors"`
	InferStatusCodes           bool `yaml:"infer_status_codes"`
	InferRequestBody           bool `yaml:"infer_request_body"`
	InferResponseBody          bool `yaml:"infer_response_body"`
	InferQueryParams           bool `yaml:"infer_query_params"`
	InferPathParams            bool `yaml:"infer_path_params"`
	InferAuth                  bool `yaml:"infer_auth"`
}

type Config struct {
	ProjectName     string       `yaml:"project_name"`
	BasePath        string       `yaml:"base_path"`
	DefaultTags     []string     `yaml:"default_tags"`
	DefaultErrorType string      `yaml:"default_error_type"`
	Auth            AuthConfig   `yaml:"auth"`
	Output          OutputConfig `yaml:"output"`
	Rules           RulesConfig  `yaml:"rules"`
}

func DefaultConfig() *Config {
	return &Config{
		ProjectName: "API",
		BasePath:    "/api/v1",
		DefaultTags: []string{"API"},
		DefaultErrorType: "ErrorResponse",
		Auth: AuthConfig{
			JWT: JWTConfig{
				Enabled:      true,
				SecurityName: "BearerAuth",
				Header:       "Authorization",
				Prefix:       "Bearer ",
			},
		},
		Output: OutputConfig{
			Swaggo:         true,
			OpenAPIVersion: "2.0",
		},
		Rules: RulesConfig{
			NeverModifyExistingSwagger: true,
			GenerateForPrivateHandlers: true,
			IncludeInternalErrors:      true,
			InferStatusCodes:           true,
			InferRequestBody:           true,
			InferResponseBody:          true,
			InferQueryParams:           true,
			InferPathParams:            true,
			InferAuth:                  true,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}
	return cfg, nil
}
