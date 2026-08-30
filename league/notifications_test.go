package league

import "testing"

func TestNotificationConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  Notification
		want Notification
	}{
		{
			name: "ResultSubmitted",
			got:  NotifResultSubmitted("m1"),
			want: Notification{Type: "quorum_request", Title: "Resultado enviado", Body: "Tu rival ha registrado un resultado. Confirma o disputa.", MatchID: "m1"},
		},
		{
			name: "MatchReportedUnplayed",
			got:  NotifMatchReportedUnplayed("m1"),
			want: Notification{Type: "general", Title: "Partido reportado como no jugado", Body: "Tu rival ha reportado este partido como no jugado. Un administrador lo revisará.", MatchID: "m1"},
		},
		{
			name: "ResultConfirmed",
			got:  NotifResultConfirmed("m1"),
			want: Notification{Type: "general", Title: "Resultado confirmado", Body: "Tu rival ha confirmado el resultado del partido.", MatchID: "m1"},
		},
		{
			name: "ResultDisputed",
			got:  NotifResultDisputed("m1"),
			want: Notification{Type: "general", Title: "Resultado disputado", Body: "Tu rival ha disputado el resultado que enviaste.", MatchID: "m1"},
		},
		{
			name: "ResultCorrected",
			got:  NotifResultCorrected("m1"),
			want: Notification{Type: "quorum_request", Title: "Resultado corregido", Body: "El rival ha corregido el resultado. Confirma o disputa.", MatchID: "m1"},
		},
		{
			name: "NewMessage",
			got:  NotifNewMessage("m1", "Ana", "Hola, ¿jugamos mañana?"),
			want: Notification{Type: "general", Title: "Nuevo mensaje", Body: "Ana escribió: Hola, ¿jugamos mañana?", MatchID: "m1"},
		},
		{
			name: "NewMessage_truncates",
			got:  NotifNewMessage("m1", "Ana", "Este es un mensaje muy largo que debería ser truncado porque supera los sesenta caracteres permitidos"),
			want: Notification{Type: "general", Title: "Nuevo mensaje", Body: "Ana escribió: Este es un mensaje muy largo que debería ser truncado porque...", MatchID: "m1"},
		},
		{
			name: "Proposal",
			got:  NotifProposal(ProposalParams{MatchID: "m1", AuthorName: "Carlos", Date: "15/03", Time: "18:00", VenueName: "Padel 360"}),
			want: Notification{Type: "scheduling", Title: "Propuesta de fecha", Body: "Carlos propone jugar el 15/03 a las 18:00 en Padel 360", MatchID: "m1"},
		},
		{
			name: "ProposalAccepted",
			got:  NotifProposalAccepted("m1", "María", "15/03", "18:00"),
			want: Notification{Type: "scheduling", Title: "Propuesta aceptada", Body: "María aceptó tu propuesta para el 15/03 a las 18:00", MatchID: "m1"},
		},
		{
			name: "ProposalRejected",
			got:  NotifProposalRejected("m1", "María", "No puedo ese día"),
			want: Notification{Type: "scheduling", Title: "Propuesta rechazada", Body: "María rechazó tu propuesta: No puedo ese día", MatchID: "m1"},
		},
		{
			name: "DecisionChangedToRejected",
			got:  NotifDecisionChangedToRejected("m1", "María"),
			want: Notification{Type: "scheduling", Title: "Decisión cambiada", Body: "María cambió su decisión: propuesta ahora rechazada", MatchID: "m1"},
		},
		{
			name: "DecisionChangedToAccepted",
			got:  NotifDecisionChangedToAccepted("m1", "María", "15/03", "18:00"),
			want: Notification{Type: "scheduling", Title: "Decisión cambiada", Body: "María cambió su decisión: propuesta ahora aceptada para el 15/03 a las 18:00", MatchID: "m1"},
		},
		{
			name: "SchedulingReminder",
			got:  NotifSchedulingReminder("m1", "pendiente"),
			want: Notification{Type: "scheduling", Title: "Recordatorio: organiza tu partido", Body: "Tu partido está pendiente. Organízalo antes de que venza el plazo.", MatchID: "m1"},
		},
		{
			name: "WalkoverApproved",
			got:  NotifWalkoverApproved("m1"),
			want: Notification{Type: "general", Title: "Walkover aprobado", Body: "Un administrador ha resuelto el partido como walkover.", MatchID: "m1"},
		},
		{
			name: "DisputeResolved",
			got:  NotifDisputeResolved("m1"),
			want: Notification{Type: "dispute", Title: "Disputa resuelta", Body: "Un administrador ha resuelto la disputa de tu partido.", MatchID: "m1"},
		},
		{
			name: "AdminMatchUnplayed",
			got:  NotifAdminMatchUnplayed("m1"),
			want: Notification{Type: "dispute", Title: "Partido no jugado", Body: "Un jugador ha reportado un partido como no jugado.", MatchID: "m1"},
		},
		{
			name: "AdminDisputed",
			got:  NotifAdminDisputed("m1", "Notas del jugador"),
			want: Notification{Type: "dispute", Title: "Partido disputado", Body: "Notas del jugador", MatchID: "m1"},
		},
		{
			name: "AdminSupersedeFailed",
			got:  NotifAdminSupersedeFailed("m1"),
			want: Notification{Type: "admin_message", Title: "Propuestas pendientes no actualizadas", Body: "El partido m1 tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.", MatchID: "m1"},
		},
		{
			name: "AdminPlayoffAdvanceFailed",
			got:  NotifAdminPlayoffAdvanceFailed("m1"),
			want: Notification{Type: "admin_message", Title: "Error en avance de playoff", Body: "El partido finalizó pero el bracket no avanzó automáticamente. Revisa el panel de administración.", MatchID: "m1"},
		},
		{
			name: "AdminMatchProgress",
			got:  NotifAdminMatchProgress("m1", "Resultado registrado: 6-3 6-4"),
			want: Notification{Type: "match_progress", Title: "Progreso de partido", Body: "Resultado registrado: 6-3 6-4", MatchID: "m1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %+v, want %+v", tt.got, tt.want)
			}
		})
	}
}
