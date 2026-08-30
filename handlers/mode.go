package handlers

// Mode selects how a component renders along three independent axes.
type Mode struct {
	Admin    bool
	Full     bool
	Editable bool
}

// Mode presets for the common rendering variants.
var (
	PlayerRow    = Mode{Admin: false, Full: false, Editable: false}
	PlayerFull   = Mode{Admin: false, Full: true, Editable: true}
	AdminSummary = Mode{Admin: true, Full: false, Editable: false}
	AdminFull    = Mode{Admin: true, Full: true, Editable: true}
)
