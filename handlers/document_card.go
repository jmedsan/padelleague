package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// fileTokenFor generates a short-lived file token for the authenticated user.
// Returns "" if no auth record is present (unauthenticated requests).
func fileTokenFor(e *core.RequestEvent) string {
	if e.Auth == nil {
		return ""
	}
	t, err := e.Auth.NewFileToken()
	if err != nil {
		slog.Error("file token generation failed", "err", err)
		return ""
	}
	return t
}

// DocumentView is a view-model for rendering a document across surfaces.
type DocumentView struct {
	Document      *core.Record
	Title         string
	Description   string
	IsFile        bool
	OpenURL       string
	IsDefault     bool
	IsMandatory   bool
	Acked         bool
	CompetitionID string
	Mode          Mode
}

// NewDocumentView builds a DocumentView from a document record.
// fileToken is appended to file URLs so protected files are accessible;
// pass "" when no authenticated user is available.
func NewDocumentView(doc *core.Record, mode Mode, fileToken string) DocumentView {
	isFile := doc.GetString("file") != ""
	openURL := doc.GetString("url")
	if isFile {
		openURL = "/api/files/documents/" + doc.Id + "/" + doc.GetString("file")
		if fileToken != "" {
			openURL += "?token=" + fileToken
		}
	}
	return DocumentView{
		Document:    doc,
		Title:       doc.GetString("title"),
		Description: doc.GetString("description"),
		IsFile:      isFile,
		OpenURL:     openURL,
		IsDefault:   doc.GetBool("is_default"),
		IsMandatory: doc.GetBool("is_mandatory"),
		Mode:        mode,
	}
}

// NewDocumentViewWithAck builds a DocumentView with Acked set from ackedIDs.
func NewDocumentViewWithAck(doc *core.Record, mode Mode, fileToken string, ackedIDs map[string]struct{}) DocumentView {
	dv := NewDocumentView(doc, mode, fileToken)
	_, dv.Acked = ackedIDs[doc.Id]
	return dv
}
