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

// The Swagger UI files, pinned to one release and checked by the browser against their hashes (Subresource Integrity),
// so a changed file at the CDN is refused instead of running on the gateway's origin, where Swagger UI keeps an
// entered token (IR371). The hashes are of the files in the npm tarball of that release, which match unpkg's.
const (
	swaggerUIVersion   = "5.33.1"
	swaggerUIBundle    = "https://unpkg.com/swagger-ui-dist@" + swaggerUIVersion + "/swagger-ui-bundle.js"
	swaggerUIBundleSRI = "sha384-ZPehFMQommnnuaZ4rpxgkgTT2DKFVp4hZC/7pLit+9Lek9T1YGSo23eHFbvNkXkw"
	swaggerUICSS       = "https://unpkg.com/swagger-ui-dist@" + swaggerUIVersion + "/swagger-ui.css"
	swaggerUICSSSRI    = "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW"
)

// swaggerUI is the Swagger UI page as in its installation guide (unpkg distribution), reading /swagger.json. Its own
// script is a file (/docs/init.js), so the page's Content-Security-Policy needs no inline script.
const swaggerUI = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AC Project Core API</title>
  <link rel="stylesheet" href="` + swaggerUICSS + `" integrity="` + swaggerUICSSSRI + `" crossorigin="anonymous">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="` + swaggerUIBundle + `" integrity="` + swaggerUIBundleSRI + `" crossorigin="anonymous"></script>
  <script src="/docs/init.js"></script>
</body>
</html>
`

// swaggerInit starts Swagger UI on the API description.
const swaggerInit = `window.onload = () => {
  window.ui = SwaggerUIBundle({ url: "/swagger.json", dom_id: "#swagger-ui", deepLinking: true, persistAuthorization: true, defaultModelsExpandDepth: 0 });
};
`

// docsCSP lets the page run only the pinned Swagger UI file and its own start script, call only this origin, and be
// framed by no one. Swagger UI sets inline styles, so styles allow 'unsafe-inline' as well as the pinned sheet.
const docsCSP = "default-src 'none'; script-src 'self' " + swaggerUIBundle + "; style-src " + swaggerUICSS + " 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; font-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// mountDocs serves the API description: GET /swagger.json (the document) and GET /docs (Swagger UI). The gateway is
// the development and demo entry; production routes the catalog's paths at the load balancer without it.
func mountDocs(e *echo.Echo) {
	e.GET("/swagger.json", func(c *echo.Context) error { return c.JSONBlob(http.StatusOK, swagger) })
	e.GET("/docs", func(c *echo.Context) error {
		h := c.Response().Header()
		h.Set("Content-Security-Policy", docsCSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		return c.HTML(http.StatusOK, swaggerUI)
	})
	e.GET("/docs/init.js", func(c *echo.Context) error {
		c.Response().Header().Set("X-Content-Type-Options", "nosniff")
		return c.Blob(http.StatusOK, "text/javascript; charset=utf-8", []byte(swaggerInit))
	})
}
