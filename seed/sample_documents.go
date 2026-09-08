package seed

import (
	"fmt"
	"io/fs"

	"padelleague/league"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

func createReglamentoPDFDoc(txApp core.App, col *core.Collection, staticFS fs.FS) (string, error) {
	if staticFS == nil {
		return "", nil
	}
	b, err := fs.ReadFile(staticFS, "static/docs/reglamento-liga-amistosa.pdf")
	if err != nil {
		return "", nil
	}
	f, err := filesystem.NewFileFromBytes(b, "reglamento-liga-amistosa.pdf")
	if err != nil {
		return "", nil
	}
	rec := core.NewRecord(col)
	rec.Set("title", "Reglamento de la liga (amistosa)")
	rec.Set("is_mandatory", true)
	rec.Set("is_default", true)
	rec.Set("file", f)
	if err := txApp.Save(rec); err != nil {
		return "", fmt.Errorf("create document reglamento PDF: %w", err)
	}
	return rec.Id, nil
}

func createSampleAnnouncement(txApp core.App, comp *core.Record) error {
	col, err := txApp.FindCollectionByNameOrId("announcements")
	if err != nil {
		return fmt.Errorf("find announcements collection: %w", err)
	}
	authorID := ""
	if admins, err := txApp.FindRecordsByFilter("users", "roles ~ 'admin'", "", 1, 0, nil); err == nil && len(admins) > 0 {
		authorID = admins[0].Id
	} else {
		return nil
	}
	rec := core.NewRecord(col)
	rec.Set("competition", comp.Id)
	rec.Set("title", "Bienvenida")
	rec.Set("body", "Bienvenidos a la competición. Recordad revisar los documentos.")
	rec.Set("created_by", authorID)
	if err := txApp.Save(rec); err != nil {
		return fmt.Errorf("create sample announcement: %w", err)
	}
	return nil
}

func createSampleDocuments(txApp core.App, comp *core.Record, staticFS fs.FS) error {
	col, err := txApp.FindCollectionByNameOrId("documents")
	if err != nil {
		return fmt.Errorf("find documents collection: %w", err)
	}
	var docIDs []string
	pdfID, err := createReglamentoPDFDoc(txApp, col, staticFS)
	if err != nil {
		return err
	}
	if pdfID != "" {
		docIDs = append(docIDs, pdfID)
	}
	type docSpec struct {
		title     string
		url       string
		mandatory bool
		isDefault bool
	}
	linkDocs := []docSpec{
		{title: "Reglamento FEP", url: "https://www.fep.es/noticias/reglamento-de-juego", mandatory: true, isDefault: true},
		{title: "Tutorial Padel", url: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
	}
	for _, d := range linkDocs {
		rec := core.NewRecord(col)
		rec.Set("title", d.title)
		rec.Set("url", d.url)
		rec.Set("is_mandatory", d.mandatory)
		rec.Set("is_default", d.isDefault)
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create document %s: %w", d.title, err)
		}
		docIDs = append(docIDs, rec.Id)
	}
	comp.Set("documents", docIDs)
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("attach documents to competition: %w", err)
	}
	return nil
}

func ackSampleDocuments(txApp core.App, comp *core.Record, pairIDs []string) error {
	allDocs := league.AttachedDocuments(txApp, comp)
	var docIDs []string
	for _, d := range allDocs {
		if d.GetBool("is_mandatory") {
			docIDs = append(docIDs, d.Id)
		}
	}
	if len(docIDs) == 0 {
		return nil
	}
	playerIDs := make(map[string]struct{})
	for _, pid := range pairIDs {
		for _, uid := range league.PlayersForPair(txApp, pid) {
			playerIDs[uid] = struct{}{}
		}
	}
	col, err := txApp.FindCollectionByNameOrId("document_acks")
	if err != nil {
		return fmt.Errorf("find document_acks collection: %w", err)
	}
	for uid := range playerIDs {
		rec := core.NewRecord(col)
		rec.Set("user", uid)
		rec.Set("competition", comp.Id)
		rec.Set("documents", docIDs)
		if err := txApp.Save(rec); err != nil {
			return fmt.Errorf("create doc ack for %s: %w", uid, err)
		}
	}
	return nil
}
