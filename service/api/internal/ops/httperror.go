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

	domainErr := domainErrorOf(err)
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

// domainErrorOf is the DomainError an error answers with: the error itself, the nearest one for Echo's own errors
// (unknown route, wrong method, body limit, timeout), else apperr.From's conversion.
func domainErrorOf(err error) *apperr.DomainError {
	if domainErr, ok := errors.AsType[*apperr.DomainError](err); ok {
		return domainErr
	}

	var sc echo.HTTPStatusCoder // Echo's own errors (route not found, method not allowed, body limit) carry a status
	if !errors.As(err, &sc) {
		return apperr.From(err)
	}

	switch sc.StatusCode() {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return apperr.E(apperr.NotFound, apperr.KeyUnknownOperation)
	case http.StatusRequestEntityTooLarge:
		return apperr.E(apperr.Validation, apperr.KeyBodyTooLarge)
	case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return apperr.E(apperr.Timeout, apperr.KeyTimeout)
	}

	return apperr.From(err)
}
