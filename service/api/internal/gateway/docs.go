package gateway

import (
	_ "embed"
	"net/http"

	"github.com/labstack/echo/v5"
)

// swagger is the API description that `make swagger` builds with swag from the annotations of the operation
// handlers (IR220); the gateway serves it with Swagger UI so the contract is readable in a browser.
//
//go:embed swagger/swagger.json
var swagger []byte

// swaggerUI is the Swagger UI page as in its installation guide (unpkg distribution), reading /swagger.json.
const swaggerUI = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AC Project Core API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({ url: "/swagger.json", dom_id: "#swagger-ui", deepLinking: true, persistAuthorization: true, defaultModelsExpandDepth: 0 });
    };
  </script>
</body>
</html>
`

// mountDocs serves the API description: GET /swagger.json (the document) and GET /docs (Swagger UI). The gateway is
// the development and demo entry; production routes /v1/ops/* at the load balancer without it.
func mountDocs(e *echo.Echo) {
	e.GET("/swagger.json", func(c *echo.Context) error { return c.JSONBlob(http.StatusOK, swagger) })
	e.GET("/docs", func(c *echo.Context) error { return c.HTML(http.StatusOK, swaggerUI) })
}
