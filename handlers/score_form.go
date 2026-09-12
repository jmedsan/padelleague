package handlers

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
)

func readScoreForm(e *core.RequestEvent, carried, field string) (string, error) {
	typed := e.Request.FormValue(field)
	if typed == "" {
		return "", alertError(e, "Debes indicar el marcador")
	}
	if strings.EqualFold(strings.TrimSpace(typed), "WO") {
		return "", alertError(e, `Usa el botón de "partido no jugado" para reportarlo`)
	}
	full := strings.TrimSpace(carried + " " + typed)
	mode := league.Complete
	if e.Request.FormValue("unfinished") == "on" {
		mode = league.AllowOpenSet
	}
	if _, err := league.ParseScoreMode(full, mode); err != nil {
		return "", alertError(e, "Marcador no válido")
	}
	return full, nil
}
