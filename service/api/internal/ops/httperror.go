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
func HTTPErrorHandler(ectx *echo.Context, err error) {
	r, uerr := echo.UnwrapResponse(ectx.Response())
	if uerr == nil && r.Committed {
		return
	}

	var domainErr *apperr.DomainError

	var sc echo.HTTPStatusCoder // Echo's own errors (route not found, method not allowed, body limit) carry a status
	switch {
	case errors.As(err, &domainErr):
	case errors.As(err, &sc):
		switch sc.StatusCode() {
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			domainErr = apperr.E(apperr.NotFound, "error.unknownOperation")
		case http.StatusRequestEntityTooLarge:
			domainErr = apperr.E(apperr.Validation, "error.bodyTooLarge")
		case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			domainErr = apperr.E(apperr.Timeout, "error.timeout")
		default:
			domainErr = apperr.From(err)
		}
	default:
		domainErr = apperr.From(err)
	}

	if domainErr.CorrelationID == "" {
		domainErr.CorrelationID = correlationID(
			ectx,
		) // every ServiceError carries one, also for unknown routes (D07)
	}

	if domainErr.HTTPStatus() >= http.StatusInternalServerError {
		cause := err
		if domainErr.Cause() != nil {
			cause = domainErr.Cause()
		}

		ectx.Logger().Error("request failed", "error", cause, "request_id", domainErr.CorrelationID)
	}

	if ectx.Request().Method == http.MethodHead {
		_ = ectx.NoContent(domainErr.HTTPStatus())

		return
	}

	_ = ectx.JSON(domainErr.HTTPStatus(), domainErr)
}
