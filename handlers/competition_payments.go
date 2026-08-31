package handlers

import (
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// CompetitionPaymentsHandler handles payment status for competition pairs.
type CompetitionPaymentsHandler struct {
	app core.App
}

// NewCompetitionPaymentsHandler creates a CompetitionPaymentsHandler.
func NewCompetitionPaymentsHandler(app core.App) *CompetitionPaymentsHandler {
	return &CompetitionPaymentsHandler{app: app}
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
	paymentStatus[pairID] = !paymentStatus[pairID]
	comp.Set("payment_status", paymentStatus)

	if err := h.app.Save(comp); err != nil {
		slog.Error("toggle payment failed", "err", err)
		return alertError(e, "Error al cambiar el estado de pago")
	}

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
	}

	comp.Set("payment_status", status)
	if err := h.app.Save(comp); err != nil {
		return alertError(e, "Error al guardar")
	}

	return redirectHX(e, "/admin/competitions/"+id)
}

func getPaymentStatus(comp *core.Record) map[string]bool {
	status := make(map[string]bool)
	if err := comp.UnmarshalJSONField("payment_status", &status); err != nil {
		slog.Warn("unmarshal payment_status", "err", err)
	}
	return status
}
