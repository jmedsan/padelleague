package handlers

// Mode is the ONE standard rendering mode used by every domain component
// (match, result, date, document, player, pair, competition, standings,
// notification, penalty, venue). Components select their variant from these
// axes only — never from ad-hoc per-template booleans. Three orthogonal axes:
//
//   - Admin    — role: admin view vs player view
//   - Full     — detail: full page vs reduced/summary (a compact card)
//   - Editable — interaction: edit (action controls shown) vs read-only
//   - Row      — reduced: a compact list/table row (implies not Full)
//
// The named vocabulary the app uses maps onto these axes:
//
//	admin / player      → Admin
//	full                → Full
//	reduced / summary   → !Full
//	row                 → Row
//	edit                → Editable
//	read-only           → !Editable
type Mode struct {
	Admin    bool
	Full     bool
	Editable bool
	Row      bool
}

// Standard presets — the complete set of role × detail × interaction
// combinations. Use a preset; do not construct ad-hoc Mode literals in
// handlers or invent per-component flags.
var (
	PlayerRow      = Mode{Admin: false, Full: false, Editable: false, Row: true}
	PlayerSummary  = Mode{Admin: false, Full: false, Editable: false}
	PlayerFull     = Mode{Admin: false, Full: true, Editable: true}
	PlayerReadOnly = Mode{Admin: false, Full: true, Editable: false}
	AdminRow       = Mode{Admin: true, Full: false, Editable: false, Row: true}
	AdminSummary   = Mode{Admin: true, Full: false, Editable: false}
	AdminFull      = Mode{Admin: true, Full: true, Editable: true}
	AdminReadOnly  = Mode{Admin: true, Full: true, Editable: false}
)
