package handlers

import "github.com/pocketbase/pocketbase/core"

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
func NewDocumentView(doc *core.Record, mode Mode) DocumentView {
	isFile := doc.GetString("file") != ""
	openURL := doc.GetString("url")
	if isFile {
		openURL = "/api/files/documents/" + doc.Id + "/" + doc.GetString("file")
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
func NewDocumentViewWithAck(doc *core.Record, mode Mode, ackedIDs map[string]struct{}) DocumentView {
	dv := NewDocumentView(doc, mode)
	_, dv.Acked = ackedIDs[doc.Id]
	return dv
}
