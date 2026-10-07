package api

import _ "embed"

// Spec is the OpenAPI description of the API, served at /api/v1/openapi.yaml.
//
//go:embed openapi.yaml
var Spec []byte
