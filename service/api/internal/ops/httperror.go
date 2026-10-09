package ops

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// HTTPErrorHandler is the central Echo HTTPErrorHandler: DomainErrors keep their code and status, Echo's own errors
// (unknown route, wrong method, body limit) map to the nearest DomainError, and anything else is a logged 503
// UNAVAILABLE without internal details.
func HTTPErrorHandler(c *echo.Context, err error) {
	if r, uerr := echo.UnwrapResponse(c.Response()); uerr == nil && r.Committed {
		return
	}
	var de *apperr.DomainError
	var sc echo.HTTPStatusCoder // Echo's own errors (route not found, method not allowed, body limit) carry a status
	switch {
	case errors.As(err, &de):
	case errors.As(err, &sc):
		switch sc.StatusCode() {
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			de = apperr.E(apperr.NotFound, "error.unknownOperation")
		case http.StatusRequestEntityTooLarge:
			de = apperr.E(apperr.Validation, "error.bodyTooLarge")
		case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			de = apperr.E(apperr.Timeout, "error.timeout")
		default:
			de = apperr.From(err)
		}
	default:
		de = apperr.From(err)
	}
	if de.CorrelationID == "" {
		de.CorrelationID = correlationID(c) // every ServiceError carries one, also for unknown routes (D07)
	}
	if de.HTTPStatus() >= http.StatusInternalServerError {
		cause := err
		if de.Cause() != nil {
			cause = de.Cause()
		}
		c.Logger().Error("request failed", "error", cause, "request_id", de.CorrelationID)
	}
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(de.HTTPStatus())
		return
	}
	_ = c.JSON(de.HTTPStatus(), de)
}
