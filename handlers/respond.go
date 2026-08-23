package handlers

import (
	"html"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func alertError(e *core.RequestEvent, msg string) error {
	return e.HTML(http.StatusOK, `<div class="alert alert-error">`+html.EscapeString(msg)+`</div>`)
}

func alertSuccess(e *core.RequestEvent, msg string) error {
	return e.HTML(http.StatusOK, `<div class="alert alert-success">`+html.EscapeString(msg)+`</div>`)
}

func alertWarning(e *core.RequestEvent, msg string) error {
	return e.HTML(http.StatusOK, `<div class="alert alert-warning">`+html.EscapeString(msg)+`</div>`)
}

func redirectHX(e *core.RequestEvent, url string) error {
	e.Response.Header().Set("HX-Redirect", url)
	return e.NoContent(http.StatusNoContent)
}
