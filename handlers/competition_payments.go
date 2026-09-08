package handlers

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"padelleague/league"
	"padelleague/notify"
)

// CompetitionPaymentsHandler handles payment status for competition pairs.
type CompetitionPaymentsHandler struct {
	app      core.App
	notifier *notify.Notifier
}

// NewCompetitionPaymentsHandler creates a CompetitionPaymentsHandler.
func NewCompetitionPaymentsHandler(app core.App, notifier *notify.Notifier) *CompetitionPaymentsHandler {
	return &CompetitionPaymentsHandler{app: app, notifier: notifier}
}

// TogglePayment marks a single pair's payment status as paid or unpaid.
func (h *CompetitionPaymentsHandler) TogglePayment(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	paymentStatus := getPaymentStatus(comp)
	nowPaid := !paymentStatus[pairID]
	paymentStatus[pairID] = nowPaid
	comp.Set("payment_status", paymentStatus)
	if nowPaid {
		setPaymentRecordedBy(comp, pairID, e.Auth.Id)
	}

	if err := h.app.Save(comp); err != nil {
		slog.Error("toggle payment failed", "err", err)
		return alertError(e, "Error al cambiar el estado de pago")
	}

	flash(e, "Pago registrado")
	return redirectHX(e, "/admin/competitions/"+compID)
}

// TogglePaymentAll sets all pairs in a competition to paid or unpaid.
func (h *CompetitionPaymentsHandler) TogglePaymentAll(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	pairIDs := comp.GetStringSlice("pairs")
	status := map[string]bool{}
	for _, pid := range pairIDs {
		status[pid] = true
		setPaymentRecordedBy(comp, pid, e.Auth.Id)
	}

	comp.Set("payment_status", status)
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al guardar")
	}

	flash(e, "Todos marcados como pagados")
	return redirectHX(e, "/admin/competitions/"+id)
}

// SendPaymentReminder notifies (bell + push + email) every player from an
// unpaid pair in the competition.
func (h *CompetitionPaymentsHandler) SendPaymentReminder(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	comp, err := h.app.FindRecordById("competitions", id)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	paymentStatus := getPaymentStatus(comp)
	seen := make(map[string]struct{})
	var players []string
	for _, pid := range comp.GetStringSlice("pairs") {
		if paymentStatus[pid] {
			continue
		}
		for _, uid := range league.PlayersForPair(h.app, pid) {
			if _, ok := seen[uid]; !ok {
				seen[uid] = struct{}{}
				players = append(players, uid)
			}
		}
	}
	if len(players) == 0 {
		return alertWarning(e, "Todas las parejas están al día")
	}

	h.notifier.NotifyPlayers(players, league.Notification{
		Type: "payment", Title: "Recordatorio de pago",
		Body: "Recuerda realizar el pago para " + comp.GetString("name"),
		Link: "/competition/" + comp.Id, CompName: comp.GetString("name"),
	})
	return alertSuccess(e, "Recordatorio enviado a "+strconv.Itoa(len(players))+" jugadores")
}

func getPaymentStatus(comp *core.Record) map[string]bool {
	status := make(map[string]bool)
	if err := comp.UnmarshalJSONField("payment_status", &status); err != nil {
		slog.Warn("unmarshal payment_status", "err", err)
	}
	return status
}

// setPaymentRecordedBy stamps the current time and actor for pairID into the
// competition's payment_paid_at/payment_paid_by maps, ready for Save.
func setPaymentRecordedBy(comp *core.Record, pairID, actorID string) {
	paidAt := getPaymentDates(comp)
	paidAt[pairID] = time.Now().Format(time.RFC3339)
	comp.Set("payment_paid_at", paidAt)

	paidBy := getPaymentActors(comp)
	paidBy[pairID] = actorID
	comp.Set("payment_paid_by", paidBy)
}

func getPaymentDates(comp *core.Record) map[string]string {
	dates := make(map[string]string)
	if err := comp.UnmarshalJSONField("payment_paid_at", &dates); err != nil {
		slog.Warn("unmarshal payment_paid_at", "err", err)
	}
	return dates
}

func getPaymentActors(comp *core.Record) map[string]string {
	actors := make(map[string]string)
	if err := comp.UnmarshalJSONField("payment_paid_by", &actors); err != nil {
		slog.Warn("unmarshal payment_paid_by", "err", err)
	}
	return actors
}
