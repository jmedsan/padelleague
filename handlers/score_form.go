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
	if _, err := league.ParseScoreMode(full, league.Complete); err == nil {
		return full, nil
	}
	if _, err := league.ParseScoreMode(full, league.AllowOpenSet); err != nil {
		return "", alertError(e, "Marcador no válido")
	}
	return full, nil
}
