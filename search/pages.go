package search

var staticPages = []Entry{
	NewEntry(Entry{Label: "Inicio", Secondary: "Página principal", Type: "página", URL: "/", Scope: Scope{Public: true}}),
	NewEntry(Entry{Label: "Mi perfil", Secondary: "Perfil del jugador", Type: "página", URL: "/profile", Scope: Scope{Public: true}}),
	NewEntry(Entry{Label: "Notificaciones", Secondary: "Centro de notificaciones", Type: "página", URL: "/profile/notifications", Scope: Scope{Public: true}}),
	NewEntry(Entry{Label: "Panel de administración", Secondary: "Panel principal del admin", Type: "página", URL: "/admin", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Parejas", Secondary: "Gestión de parejas", Type: "página", URL: "/admin/pairs", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Jugadores", Secondary: "Gestión de jugadores", Type: "página", URL: "/admin/players", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Disputas", Secondary: "Resolución de disputas", Type: "página", URL: "/admin/disputes", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Pendientes", Secondary: "Partidos pendientes", Type: "página", URL: "/admin/outstanding", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Invitaciones", Secondary: "Gestión de invitaciones", Type: "página", URL: "/admin/invitations", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Pistas", Secondary: "Gestión de pistas", Type: "página", URL: "/admin/venues", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Documentos", Secondary: "Biblioteca de documentos", Type: "página", URL: "/admin/documents", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Configuración", Secondary: "Ajustes de la aplicación", Type: "página", URL: "/admin/settings", Scope: Scope{Admin: true}}),
	NewEntry(Entry{Label: "Clasificación", Secondary: "Tabla de clasificación", Type: "página", URL: "/", Scope: Scope{Public: true}}),
}
