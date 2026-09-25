package handlers

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/render"
)

// competitionUpdateLabels names the display label and value formatter for
// each competitions field the admin activity timeline tracks, in the order
// they're reported. unsetZero is non-empty for fields setSchedulingFields
// autofills with a hardcoded default when the admin leaves them blank (e.g.
// an old competition's first save after this field was introduced) — it's
// that field's zero value as read by before.Get ("" for text, "0" for a
// NumberField never persisted). Those fields are only reported once they
// already held a non-zero value, so the autofill itself doesn't read as a
// spurious change.
var competitionUpdateLabels = []struct {
	field, label string
	format       func(any) string
	unsetZero    string
}{
	{"name", "Nombre", fmtActivityString, ""},
	{"type", "Tipo", fmtActivityCompType, ""},
	{"play_twice", "Ida y vuelta", fmtActivityBool, ""},
	{"gender_type", "Género", fmtActivityGenderType, ""},
	{"quorum_timeout_hours", "Tiempo de espera", fmtActivityString, ""},
	{"start_date", "Fecha inicio", fmtActivityDate, ""},
	{"end_date", "Fecha fin", fmtActivityDate, ""},
	{"arrange_grace_days", "Días de gracia", fmtActivityString, "0"},
	{"walkover_score", "Marcador de incomparecencia", fmtActivityString, ""},
	{"default_penalty", "Penalización por defecto", fmtActivityString, "0"},
	{"recovery_days", "Período extra", fmtActivityString, "0"},
	{"match_reminder_hours", "Recordatorios de partido", fmtActivityString, ""},
}

// competitionUpdateDetail builds an "actualizó la configuración: Field: old
// → new" summary of every tracked field that actually changed between
// before and after, or "" if nothing changed. before must be captured after
// any field defaulting (e.g. setSchedulingFields) has already run, so a
// first-time default doesn't read as a spurious change.
func competitionUpdateDetail(before, after *core.Record) string {
	var changes []string
	for _, f := range competitionUpdateLabels {
		oldRaw, newRaw := before.Get(f.field), after.Get(f.field)
		if fmt.Sprint(oldRaw) == fmt.Sprint(newRaw) {
			continue
		}
		if f.unsetZero != "" && fmt.Sprint(oldRaw) == f.unsetZero {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s → %s", f.label, f.format(oldRaw), f.format(newRaw)))
	}
	if len(changes) == 0 {
		return ""
	}
	return "actualizó la configuración: " + strings.Join(changes, "; ")
}

// fmtActivityString renders a raw field value for the activity log,
// showing "—" for an empty value instead of a blank string.
func fmtActivityString(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return "—"
	}
	return s
}

// fmtActivityBool renders a bool field value in Spanish.
func fmtActivityBool(v any) string {
	b, _ := v.(bool)
	if b {
		return "sí"
	}
	return "no"
}

// fmtActivityGenderType renders a gender_type field value via its Spanish
// label, showing "—" for an empty value instead of a blank string.
func fmtActivityGenderType(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return "—"
	}
	return league.GenderTypeLabel(s)
}

// fmtActivityCompType renders a competition type field value via its
// Spanish label, showing "—" for an empty value instead of a blank string.
func fmtActivityCompType(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return "—"
	}
	return league.CompetitionTypeLabel(s)
}

// fmtActivityDate renders a date field value via render.FmtDate, showing
// "—" for an empty value.
func fmtActivityDate(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return "—"
	}
	return render.FmtDate(s)
}
