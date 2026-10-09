// Package api содержит статику Swagger UI и сгенерированную OpenAPI-спецификацию.
package api

import "embed"

//go:embed swagger-ui.html swagger/bb-core.swagger.json
var FS embed.FS
