package search

var staticPages = []Entry{
	NewEntry("Inicio", "Página principal", "página", "/", Scope{Public: true}),
	NewEntry("Mi perfil", "Perfil del jugador", "página", "/profile", Scope{Public: true}),
	NewEntry("Notificaciones", "Centro de notificaciones", "página", "/profile/notifications", Scope{Public: true}),
	NewEntry("Panel de administración", "Panel principal del admin", "página", "/admin", Scope{Admin: true}),
	NewEntry("Parejas", "Gestión de parejas", "página", "/admin/pairs", Scope{Admin: true}),
	NewEntry("Jugadores", "Gestión de jugadores", "página", "/admin/players", Scope{Admin: true}),
	NewEntry("Disputas", "Resolución de disputas", "página", "/admin/disputes", Scope{Admin: true}),
	NewEntry("Pendientes", "Partidos pendientes", "página", "/admin/outstanding", Scope{Admin: true}),
	NewEntry("Invitaciones", "Gestión de invitaciones", "página", "/admin/invitations", Scope{Admin: true}),
	NewEntry("Pistas", "Gestión de pistas", "página", "/admin/venues", Scope{Admin: true}),
	NewEntry("Documentos", "Biblioteca de documentos", "página", "/admin/documents", Scope{Admin: true}),
	NewEntry("Configuración", "Ajustes de la aplicación", "página", "/admin/settings", Scope{Admin: true}),
	NewEntry("Clasificación", "Tabla de clasificación", "página", "/", Scope{Public: true}),
}
