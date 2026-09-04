package seed

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"padelleague/league"
)

func createSampleSponsor(txApp core.App, comp *core.Record, staticFS fs.FS) error {
	col, err := txApp.FindCollectionByNameOrId("sponsors")
	if err != nil {
		return fmt.Errorf("sponsors collection: %w", err)
	}
	data, err := fs.ReadFile(staticFS, "static/img/sample-sponsors/decathlon.png")
	if err != nil {
		return fmt.Errorf("read sponsor logo: %w", err)
	}
	f, err := filesystem.NewFileFromBytes(data, "decathlon.png")
	if err != nil {
		return fmt.Errorf("sponsor logo file: %w", err)
	}
	sponsor := core.NewRecord(col)
	sponsor.Set("name", "Decathlon")
	sponsor.Set("logo", f)
	sponsor.Set("url", "https://www.decathlon.es")
	if err := txApp.Save(sponsor); err != nil {
		return fmt.Errorf("save sponsor: %w", err)
	}
	comp.Set("sponsors", []string{sponsor.Id})
	if err := txApp.Save(comp); err != nil {
		return fmt.Errorf("attach sponsor to competition: %w", err)
	}
	return nil
}

func loadSampleAvatar(staticFS fs.FS, playerNum int) (*filesystem.File, error) {
	path := fmt.Sprintf("static/img/sample-avatars/player-%d.png", playerNum)
	data, err := fs.ReadFile(staticFS, path)
	if err != nil {
		return nil, err
	}
	return league.CompressAvatarBytes(bytes.NewReader(data), fmt.Sprintf("avatar-player-%d.jpg", playerNum))
}
