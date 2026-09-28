package league

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationConstructors(t *testing.T) {
	tests := []struct {
		name string
		got  Notification
		want Notification
	}{
		{
			name: "ResultSubmitted",
			got:  NotifResultSubmitted("m1", "Pareja A", "Liga Primavera", "6-2 6-2"),
			want: Notification{Type: "quorum_request", Title: "Resultado enviado", Body: "Pareja A ha enviado 6-2 6-2. Confirma o contrapropón.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ArbitrationClosed",
			got:  NotifArbitrationClosed("m1", "Fecha y hora", "Liga Primavera"),
			want: Notification{Type: "dispute", Title: "Arbitraje cerrado", Body: "El administrador ha cerrado la solicitud de arbitraje: Fecha y hora", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ResultConfirmed",
			got:  NotifResultConfirmed("m1", "Pareja A", "Liga Primavera"),
			want: Notification{Type: "general", Title: "Resultado confirmado", Body: "Pareja A ha confirmado el resultado", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ResultCorrected",
			got:  NotifResultCorrected("m1", "Pareja A", "Liga Primavera"),
			want: Notification{Type: "quorum_request", Title: "Resultado corregido", Body: "Pareja A ha corregido el resultado", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ResultCountered",
			got:  NotifResultCountered("m1", "Pareja A", "Liga Primavera"),
			want: Notification{Type: "quorum_request", Title: "Contrapropuesta recibida", Body: "Pareja A ha propuesto un resultado alternativo", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "NewMessage",
			got:  NotifNewMessage("m1", "Ana", "Hola, ¿jugamos mañana?", "Liga Primavera"),
			want: Notification{Type: "message", Title: "Nuevo mensaje", Body: "Ana escribió: Hola, ¿jugamos mañana?", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "NewMessage_truncates",
			got:  NotifNewMessage("m1", "Ana", "Este es un mensaje muy largo que debería ser truncado porque supera los sesenta caracteres permitidos", "Liga Primavera"),
			want: Notification{Type: "message", Title: "Nuevo mensaje", Body: "Ana escribió: Este es un mensaje muy largo que debería ser truncado porque...", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "Proposal",
			got:  NotifProposal(ProposalParams{MatchID: "m1", AuthorName: "Carlos", Date: "2026-03-15", Time: "18:00", VenueName: "Padel 360", CompName: "Liga Primavera"}),
			want: Notification{Type: "scheduling", Title: "Propuesta de fecha", Body: "Carlos propone jugar el 15/03 a las 18:00 en Padel 360", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ProposalAccepted",
			got: NotifProposalAccepted(ProposalAcceptedParams{
				MatchID: "m1", ResponderName: "María", Date: "2026-03-15", Time: "18:00", CompName: "Liga Primavera",
			}),
			want: Notification{Type: "scheduling", Title: "Propuesta aceptada", Body: "María aceptó tu propuesta para el 15/03 a las 18:00", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ProposalRejected",
			got:  NotifProposalRejected("m1", "María", "No puedo ese día", "Liga Primavera"),
			want: Notification{Type: "scheduling", Title: "Propuesta rechazada", Body: "María ha rechazado tu propuesta: No puedo ese día", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "SchedulingReminder",
			got: NotifSchedulingReminder(SchedulingReminderParams{
				MatchID: "m1", Opponent: "Pareja A", CompName: "Liga Primavera", Level: WarnUrgent,
				Deadline: time.Date(2026, 3, 15, 0, 0, 0, 0, Madrid),
			}),
			want: Notification{Type: "scheduling", Title: "Recordatorio: organiza tu partido", Body: "Tu partido vs Pareja A · Liga Primavera vence el 15/03. Quedan pocos días.", MatchID: "m1"},
		},
		{
			name: "WalkoverApproved",
			got:  NotifWalkoverApproved("m1", "Liga Primavera"),
			want: Notification{Type: "general", Title: "Incomparecencia aprobada", Body: "El administrador ha aprobado la incomparecencia", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "DisputeResolved",
			got:  NotifDisputeResolved("m1", "Liga Primavera"),
			want: Notification{Type: "dispute", Title: "Disputa resuelta", Body: "El administrador ha resuelto la disputa", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ArbitrationRequested",
			got:  NotifArbitrationRequested("m1", "Incomparecencia", "Liga Primavera"),
			want: Notification{Type: "dispute", Title: "Arbitraje solicitado", Body: "Un jugador ha solicitado arbitraje: Incomparecencia", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "AdminSupersedeFailed",
			got:  NotifAdminSupersedeFailed("m1", "Pareja A", "Pareja B", "Liga Primavera"),
			want: Notification{Type: "admin_message", Title: "Propuestas pendientes no actualizadas", Body: "El partido Pareja A vs Pareja B tiene propuestas que no se pudieron marcar como superadas. Revisa el hilo.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "AdminUserJoined",
			got:  NotifAdminUserJoined("Carlos García"),
			want: Notification{Type: "user_joined", Title: "Nuevo jugador registrado", Body: "Carlos García se ha registrado en la liga.", Link: "/admin/players"},
		},
		{
			name: "AdminMatchProgress",
			got:  NotifAdminMatchProgress("m1", "Resultado registrado: 6-3 6-4"),
			want: Notification{Type: "match_progress", Title: "Progreso de partido", Body: "Resultado registrado: 6-3 6-4", MatchID: "m1"},
		},
		{
			name: "MatchUpcoming_FarAhead",
			got:  NotifMatchUpcoming(MatchUpcomingParams{MatchID: "m1", Start: time.Date(2026, 6, 15, 18, 0, 0, 0, Madrid), Until: 26 * time.Hour, Venue: "Padel 360", CompName: "Liga Primavera", Opponent: "Pareja B"}),
			want: Notification{Type: "match_reminder", Title: "Próximo partido", Body: "Tu partido vs Pareja B es el 15/06 a las 18:00 en Padel 360.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "MatchUpcoming_Soon",
			got:  NotifMatchUpcoming(MatchUpcomingParams{MatchID: "m1", Start: time.Date(2026, 6, 15, 18, 0, 0, 0, Madrid), Until: 45 * time.Minute, Venue: "Padel 360", CompName: "Liga Primavera", Opponent: "Pareja B"}),
			want: Notification{Type: "match_reminder", Title: "Tu partido empieza pronto", Body: "Tu partido vs Pareja B empieza en 45 minutos · 18:00 en Padel 360.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "ProposalWithdrawn",
			got:  NotifProposalWithdrawn("m1", "Carlos", "Liga Primavera"),
			want: Notification{Type: "scheduling", Title: "Propuesta retirada", Body: "Carlos ha retirado su propuesta de fecha", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "MatchAssigned",
			got:  NotifMatchAssigned("m1", "Pareja B", "Liga Primavera"),
			want: Notification{Type: "match_assigned", Title: "Nuevo partido asignado", Body: "Tu próximo rival es Pareja B.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "CalendarPublished",
			got:  NotifCalendarPublished("c1", "Liga Primavera"),
			want: Notification{Type: "calendar_published", Title: "Calendario publicado", Body: "El calendario ha sido publicado.", Link: "/competition/c1", CompName: "Liga Primavera"},
		},
		{
			name: "OpponentWithdrawn",
			got:  NotifOpponentWithdrawn("m1", "Pareja B", "Liga Primavera", "6-0 6-0"),
			want: Notification{Type: "general", Title: "Pareja retirada", Body: "La pareja Pareja B se ha retirado. El partido se registra como 6-0 6-0 a tu favor.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "PairWithdrawn",
			got:  NotifPairWithdrawn("Liga Primavera"),
			want: Notification{Type: "general", Title: "Retirada de la competición", Body: "Tu pareja ha sido retirada de Liga Primavera.", CompName: "Liga Primavera"},
		},
		{
			name: "ResultAutoConfirmed",
			got:  NotifResultAutoConfirmed("m1", "Liga Primavera"),
			want: Notification{Type: "general", Title: "Resultado confirmado automáticamente", Body: "El resultado ha sido confirmado por tiempo de espera · Liga Primavera.", MatchID: "m1"},
		},
		{
			name: "ProposalResponsePending",
			got:  NotifProposalResponsePending("m1", "Pareja A", "Liga Primavera", 48),
			want: Notification{Type: "quorum_request", Title: "Resultado pendiente de respuesta", Body: "Pareja A propuso un resultado hace más de 48 horas · Liga Primavera. Acepta o contrapropón.", MatchID: "m1"},
		},
		{
			name: "ResultConfirmationPending",
			got:  NotifResultConfirmationPending("m1", "Pareja A", "Liga Primavera", 48),
			want: Notification{Type: "quorum_request", Title: "Resultado pendiente de confirmar", Body: "Pareja A envió un resultado hace más de 48 horas · Liga Primavera. Confirma o contrapropón.", MatchID: "m1"},
		},
		{
			name: "MatchResumes",
			got:  NotifMatchResumes("m1", "6-4", "Liga Primavera"),
			want: Notification{Type: "scheduling", Title: "Partido por reanudar", Body: "Se reanuda desde 6-4 0-0. Acordad una nueva fecha · Liga Primavera.", MatchID: "m1"},
		},
		{
			name: "PenaltyApplied",
			got:  NotifPenaltyApplied("c1", 3, "Incomparecencia"),
			want: Notification{Type: "penalty", Title: "Penalización aplicada", Body: "3 puntos — Incomparecencia", Link: "/competition/c1"},
		},
		{
			name: "PenaltyVoided",
			got:  NotifPenaltyVoided("c1", 3),
			want: Notification{Type: "penalty", Title: "Penalización anulada", Body: "3 puntos anulados", Link: "/competition/c1"},
		},
		{
			name: "AdminPenaltiesApplied",
			got:  NotifAdminPenaltiesApplied("c1", "Liga Primavera", 4),
			want: Notification{Type: "penalty", Title: "Penalizaciones automáticas aplicadas", Body: "4 penalizaciones aplicadas en Liga Primavera", Link: "/admin/competitions/c1"},
		},
		{
			name: "AdminLeagueClosed",
			got:  NotifAdminLeagueClosed("c1", "Liga Primavera", 2),
			want: Notification{Type: "penalty", Title: "Liga cerrada automáticamente", Body: "Liga Primavera ha terminado su semana extraordinaria: 2 penalizaciones por partidos no disputados. Revísalas y corrige las que correspondan a una sola pareja.", Link: "/admin/competitions/c1"},
		},
		{
			name: "RoleChanged",
			got:  NotifRoleChanged([]string{"player", "admin"}),
			want: Notification{Type: "admin_message", Title: "Cambio de rol", Body: "Tu rol ha sido actualizado a player, admin", Link: "/profile"},
		},
		{
			name: "PasswordResetRequested",
			got:  NotifPasswordResetRequested(),
			want: Notification{Type: "admin_message", Title: "Restablecimiento de contraseña", Body: "Un administrador ha solicitado restablecer tu contraseña"},
		},
		{
			name: "PaymentReminder",
			got:  NotifPaymentReminder("c1", "Liga Primavera"),
			want: Notification{Type: "payment", Title: "Recordatorio de pago", Body: "Recuerda realizar el pago para Liga Primavera", Link: "/competition/c1", CompName: "Liga Primavera"},
		},
		{
			name: "AdminCorrection",
			got:  NotifAdminCorrection("m1", []string{"Resultado: 6-3 6-4", "Fecha: 15/03"}, "Liga Primavera"),
			want: Notification{Type: "general", Title: "Corrección de administrador", Body: "Resultado: 6-3 6-4. Fecha: 15/03", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "DateCancelled",
			got:  NotifDateCancelled(DateCancelledParams{MatchID: "m1", PlayerName: "Ana (Pareja A)", Reason: "lesión", CompName: "Liga Primavera"}),
			want: Notification{Type: "scheduling", Title: "Partido cancelado", Body: "Ana (Pareja A) ha cancelado la fecha: lesión", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "AdminDateCancelled",
			got:  NotifAdminDateCancelled(DateCancelledParams{MatchID: "m1", PlayerName: "Ana (Pareja A)", Reason: "lesión", CompName: "Liga Primavera", Pair1Name: "Pareja A", Pair2Name: "Pareja B", Urgency: " Quedan pocos días."}),
			want: Notification{Type: "dispute", Title: "Cancelación de partido", Body: "Pareja A vs Pareja B: Ana (Pareja A) ha cancelado la fecha. Motivo: lesión Quedan pocos días.", MatchID: "m1", CompName: "Liga Primavera"},
		},
		{
			name: "Announcement",
			got:  NotifAnnouncement("c1", "Liga Primavera", "Aviso", "Cambio de pista"),
			want: Notification{Type: "announcement", Title: "Aviso", Body: "Cambio de pista", CompName: "Liga Primavera", Link: "/competition/c1#avisos"},
		},
		{
			name: "TestPush",
			got:  NotifTestPush(),
			want: Notification{Type: "general", Title: "Notificación de prueba", Body: "Si ves esto, las notificaciones funcionan correctamente.", Link: "/admin/dev-tools"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

// TestNotificationConstructors_CoversEveryConstructor fails when a Notif*
// constructor gains no row in TestNotificationConstructors: the table is the
// one place every notification's exact text is pinned.
func TestNotificationConstructors_CoversEveryConstructor(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	src, err := parser.ParseFile(fset, "notifications.go", nil, 0)
	require.NoError(t, err)
	tst, err := parser.ParseFile(fset, "notifications_test.go", nil, 0)
	require.NoError(t, err)

	var covered []string
	ast.Inspect(tst, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if fn, ok := call.Fun.(*ast.Ident); ok && strings.HasPrefix(fn.Name, "Notif") {
				covered = append(covered, fn.Name)
			}
		}
		return true
	})
	for _, d := range src.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Notif") {
			continue
		}
		assert.Contains(t, covered, fn.Name.Name, "add a TestNotificationConstructors row for %s", fn.Name.Name)
	}
}

// TestNotificationLiteralsLiveInConstructors fails when a package builds a
// Notification literal outside notifications.go, which is how the bare
// "Player" instead of "Player (Pair)" bodies slipped past the sweep.
func TestNotificationLiteralsLiveInConstructors(t *testing.T) {
	t.Parallel()
	var offenders []string
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "e2e" || d.Name() == "frontend") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "league/notifications.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if ok && isNotificationType(lit.Type) {
				offenders = append(offenders, path)
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders, "build notifications through a league.Notif* constructor")
}

func isNotificationType(expr ast.Expr) bool {
	switch tt := expr.(type) {
	case *ast.Ident:
		return tt.Name == "Notification"
	case *ast.SelectorExpr:
		return tt.Sel.Name == "Notification"
	}
	return false
}

func TestFmtNotifDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, date, time, want string
	}{
		{"with time", "2026-03-15", "18:00", "el 15/03 a las 18:00"},
		{"no time", "2026-03-15", "", "el 15/03/2026"},
		{"unparseable falls back to raw", "not-a-date", "18:00", "not-a-date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fmtNotifDate(tt.date, tt.time))
		})
	}
}
