package handlers

import (
	"log/slog"
	"mime/multipart"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// DocumentHandler handles admin document library management.
type DocumentHandler struct {
	app        core.App
	renderPage RenderFunc
}

// NewDocumentHandler creates a DocumentHandler with the given dependencies.
func NewDocumentHandler(app core.App, renderPage RenderFunc) *DocumentHandler {
	return &DocumentHandler{app: app, renderPage: renderPage}
}

// Documents renders the admin documents library page.
func (h *DocumentHandler) Documents(e *core.RequestEvent) error {
	docs, _ := h.app.FindRecordsByFilter("documents",
		"id != ''", "title", 0, 0, nil)

	docViews := make([]DocumentView, len(docs))
	for i, d := range docs {
		docViews[i] = NewDocumentView(d, AdminFull)
	}
	return h.renderPage(e, "admin/documents.html", map[string]any{
		"PageTitle":     "Documentos",
		"Documents":     docs,
		"DocumentViews": docViews,
	})
}

// DocumentsCreate handles POST to add a new document (file or link).
func (h *DocumentHandler) DocumentsCreate(e *core.RequestEvent) error {
	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return alertError(e, "El título es obligatorio")
	}

	urlVal := e.Request.FormValue("url")
	fh := fileHeader(e, "file")
	if (urlVal == "") == (fh == nil) {
		return alertError(e, "Añade un archivo o un enlace (uno de los dos)")
	}

	col, err := h.app.FindCollectionByNameOrId("documents")
	if err != nil {
		return alertError(e, "Error interno")
	}

	record := core.NewRecord(col)
	record.Set("title", title)
	record.Set("description", e.Request.FormValue("description"))
	record.Set("url", urlVal)
	record.Set("is_default", e.Request.FormValue("is_default") == "on")
	record.Set("is_mandatory", e.Request.FormValue("is_mandatory") == "on")
	record.Set("created_by", e.Auth.Id)

	if fh != nil {
		f, err := filesystem.NewFileFromMultipart(fh)
		if err != nil {
			return alertError(e, "Archivo no válido")
		}
		record.Set("file", f)
	}

	if err := h.app.Save(record); err != nil {
		slog.Error("save document failed", "err", err)
		return alertError(e, "Error al guardar el documento")
	}

	flash(e, "Documento creado")
	return redirectHX(e, "/admin/documents")
}

// DocumentsUpdate handles POST to modify an existing document.
func (h *DocumentHandler) DocumentsUpdate(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("documents", id)
	if err != nil {
		return alertError(e, "Documento no encontrado")
	}

	title := strings.TrimSpace(e.Request.FormValue("title"))
	if title == "" {
		return alertError(e, "El título es obligatorio")
	}

	record.Set("title", title)
	record.Set("description", e.Request.FormValue("description"))
	record.Set("url", e.Request.FormValue("url"))
	record.Set("is_default", e.Request.FormValue("is_default") == "on")
	record.Set("is_mandatory", e.Request.FormValue("is_mandatory") == "on")

	fh := fileHeader(e, "file")
	if fh != nil {
		f, err := filesystem.NewFileFromMultipart(fh)
		if err != nil {
			return alertError(e, "Archivo no válido")
		}
		record.Set("file", f)
	}

	if err := h.app.Save(record); err != nil {
		slog.Error("update document failed", "err", err)
		return alertError(e, "Error al guardar el documento")
	}

	flash(e, "Documento actualizado")
	return redirectHX(e, "/admin/documents")
}

// DocumentsDelete handles POST to remove a document from the library.
func (h *DocumentHandler) DocumentsDelete(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	record, err := h.app.FindRecordById("documents", id)
	if err != nil {
		return alertError(e, "Documento no encontrado")
	}

	if err := h.app.Delete(record); err != nil {
		slog.Error("delete document failed", "err", err)
		return alertError(e, "Error al eliminar el documento")
	}

	flash(e, "Documento eliminado")
	return redirectHX(e, "/admin/documents")
}

func fileHeader(e *core.RequestEvent, field string) *multipart.FileHeader {
	if e.Request.MultipartForm == nil {
		if err := e.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil
		}
	}
	files := e.Request.MultipartForm.File[field]
	if len(files) == 0 {
		return nil
	}
	return files[0]
}
