package ops

import (
	"context"

	"github.com/labstack/echo/v5"
)

// internalCallerKey marks a request context whose X-Internal-Token matched the service token.
type internalCallerKey struct{}

// InternalCallerMiddleware marks a request that carries the service token (X-Internal-Token, compared in constant
// time) as an internal caller, the IoT bridge's demo triggers for example (IR768). An empty token marks nothing.
func InternalCallerMiddleware(token string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ectx *echo.Context) error {
			if internalTokenValid(ectx, token) {
				req := ectx.Request()
				ectx.SetRequest(
					req.WithContext(context.WithValue(req.Context(), internalCallerKey{}, true)),
				)
			}

			return next(ectx)
		}
	}
}

// InternalCaller reports whether the request carried the service token.
func InternalCaller(ctx context.Context) bool {
	internal, _ := ctx.Value(internalCallerKey{}).(bool)

	return internal
}
