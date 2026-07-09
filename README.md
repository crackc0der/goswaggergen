# swagger-auto-doc

CLI tool to automatically generate swaggo-compatible Swagger annotations for Go HTTP handlers on [go-chi/chi](https://github.com/go-chi/chi/v5).

## Usage

```bash
swagger-auto-doc --input <path> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--input` | `string` | — | **(required)** File, directory, or Go package pattern (e.g. `./...`, `./internal/handlers/routes.go`) |
| `--write` | `bool` | `false` | Write generated Swagger comments to files on disk |
| `--dry-run` | `bool` | `false` | Preview changes without modifying files. Mutually exclusive with `--write` |
| `--config` | `string` | `""` | Path to YAML configuration file |
| `--verbose` | `bool` | `false` | Enable detailed analysis logging |
| `--fail-on-existing-swagger-change` | `bool` | `false` | Exit with error if an existing Swagger annotation would be modified |

## Examples

```bash
# Preview what would be generated
swagger-auto-doc --input ./... --dry-run

# Preview with verbose output
swagger-auto-doc --input ./... --dry-run --verbose

# Apply annotations to the whole project
swagger-auto-doc --input ./... --write

# Apply to a single file
swagger-auto-doc --input ./internal/http/routes.go --write

# Use custom config (any YAML file matching the format below)
swagger-auto-doc --input ./... --write --config ./config.yaml

# Fail on any attempt to modify existing Swagger
swagger-auto-doc --input ./... --write --fail-on-existing-swagger-change
```

## Config file (YAML)

```yaml
project_name: "My API"                 # title for generated Swagger docs
base_path: "/api/v1"                   # stripped from paths when inferring tags
default_error_type: "dto.ErrorResponse" # fallback type for @Failure lines
auth:
  jwt:
    enabled: true                      # enable JWT auth detection
    security_name: "BearerAuth"        # value for @Security annotation
    header: "Authorization"            # reading this header → route marked protected
    prefix: "Bearer "                  # token prefix in the header value
rules:
  never_modify_existing_swagger: true  # skip handlers with existing Swagger
  generate_for_private_handlers: true  # include unexported (lowercase) handlers
  infer_request_body: true             # find request body from json.Decode
  infer_response_body: true            # find response body from json.Encode / writeJSON
  infer_query_params: true             # find query params from r.URL.Query().Get()
  infer_path_params: true              # find path param types from chi.URLParam
  infer_status_codes: true             # find status codes from WriteHeader / http.Error
  infer_auth: true                     # detect protected routes via middleware names
```

## Requirements

- Go 1.22+
- Project must import `github.com/go-chi/chi/v5` or `github.com/go-chi/chi`
