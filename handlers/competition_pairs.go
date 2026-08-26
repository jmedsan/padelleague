package handlers

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

type pairEntry struct {
	PairID   string
	PairName string
	Seed     int
	Paid     bool
}

// AddPair enrolls a pair in a competition, validating player uniqueness.
func (h *CompetitionHandler) AddPair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair")
	seedStr := e.Request.FormValue("seed")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return alertError(e, "Pareja no encontrada")
	}

	existingPairIDs := comp.GetStringSlice("pairs")
	if err := h.validatePlayerUniqueness(existingPairIDs, pair, ""); err != nil {
		slog.Error("player uniqueness validation failed", "pair", pairID, "err", err)
		return alertError(e, "Esta pareja tiene jugadores duplicados en la competición")
	}

	for _, pid := range existingPairIDs {
		if pid == pairID {
			return alertError(e, "Esta pareja ya está en la competición")
		}
	}

	comp.Set("pairs", append(existingPairIDs, pairID))

	if seedStr != "" {
		seed, _ := strconv.Atoi(seedStr)
		if seed > 0 {
			seeding := h.getSeeding(comp)
			seeding[pairID] = seed
			comp.Set("seeding", seeding)
		}
	}

	if err := h.app.Save(comp); err != nil {
		slog.Error("add pair failed", "competition", compID, "err", err)
		return alertError(e, "Error al añadir la pareja")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
}

// RemovePair removes a pair from a competition and deletes its pending matches.
func (h *CompetitionHandler) RemovePair(e *core.RequestEvent) error {
	compID := e.Request.PathValue("id")
	pairID := e.Request.FormValue("pair_id")

	comp, err := h.app.FindRecordById("competitions", compID)
	if err != nil {
		return alertError(e, "Competición no encontrada")
	}

	existingPairIDs := comp.GetStringSlice("pairs")
	var updated []string
	for _, pid := range existingPairIDs {
		if pid != pairID {
			updated = append(updated, pid)
		}
	}
	comp.Set("pairs", updated)

	seeding := h.getSeeding(comp)
	delete(seeding, pairID)
	comp.Set("seeding", seeding)

	paymentStatus := h.getPaymentStatus(comp)
	delete(paymentStatus, pairID)
	comp.Set("payment_status", paymentStatus)

	if err := h.app.Save(comp); err != nil {
		slog.Error("remove pair failed", "competition", compID, "err", err)
		return alertError(e, "Error al eliminar la pareja")
	}

	return redirectHX(e, "/admin/competitions/"+compID)
}

// CopyPairs imports pairs from a source competition into the target.
func (h *CompetitionHandler) CopyPairs(e *core.RequestEvent) error {
	targetID := e.Request.PathValue("id")
	sourceID := e.Request.FormValue("source_competition")

	if sourceID == "" {
		return alertError(e, "Selecciona una competición de origen")
	}

	source, err := h.app.FindRecordById("competitions", sourceID)
	if err != nil {
		return alertError(e, "Competición origen no encontrada")
	}

	target, err := h.app.FindRecordById("competitions", targetID)
	if err != nil {
		return alertError(e, "Competición destino no encontrada")
	}

	sourcePairIDs := source.GetStringSlice("pairs")
	sourceSeeding := h.getSeeding(source)
	existingPairIDs := target.GetStringSlice("pairs")
	targetSeeding := h.getSeeding(target)

	existingSet := make(map[string]bool, len(existingPairIDs))
	for _, pid := range existingPairIDs {
		existingSet[pid] = true
	}

	copied := 0
	skipped := 0
	isPlayoff := target.GetString("type") == "playoff"
	for _, pairID := range sourcePairIDs {
		if !h.canCopyPair(pairID, existingSet, existingPairIDs) {
			skipped++
			continue
		}
		existingPairIDs = append(existingPairIDs, pairID)
		existingSet[pairID] = true
		if isPlayoff {
			if s, ok := sourceSeeding[pairID]; ok {
				targetSeeding[pairID] = s
			}
		}
		copied++
	}

	target.Set("pairs", existingPairIDs)
	target.Set("seeding", targetSeeding)

	if err := h.app.Save(target); err != nil {
		slog.Error("copy pairs failed", "err", err)
		return alertError(e, "Error al copiar parejas")
	}

	return alertSuccess(e, fmt.Sprintf("%d parejas copiadas, %d omitidas", copied, skipped))
}

func (h *CompetitionHandler) canCopyPair(pairID string, existingSet map[string]bool, existingPairIDs []string) bool {
	if existingSet[pairID] {
		return false
	}
	pair, err := h.app.FindRecordById("pairs", pairID)
	if err != nil {
		return false
	}
	return h.validatePlayerUniqueness(existingPairIDs, pair, "") == nil
}

func (h *CompetitionHandler) buildPairEntries(pairIDs []string, seeding map[string]int, paymentStatus map[string]bool) []pairEntry {
	var entries []pairEntry
	for _, pid := range pairIDs {
		pair, err := h.app.FindRecordById("pairs", pid)
		if err != nil {
			continue
		}
		entries = append(entries, pairEntry{
			PairID:   pid,
			PairName: pair.GetString("name"),
			Seed:     seeding[pid],
			Paid:     paymentStatus[pid],
		})
	}
	return entries
}

func (h *CompetitionHandler) availablePairs(enrolledIDs []string) []*core.Record {
	allPairsRaw, _ := h.app.FindAllRecords("pairs")
	enrolled := map[string]bool{}
	for _, pid := range enrolledIDs {
		enrolled[pid] = true
	}
	var available []*core.Record
	for _, p := range allPairsRaw {
		if !enrolled[p.Id] {
			available = append(available, p)
		}
	}
	return available
}

func (h *CompetitionHandler) validatePlayerUniqueness(existingPairIDs []string, pair *core.Record, excludePairID string) error {
	p1 := pair.GetString("player1")
	p2 := pair.GetString("player2")

	for _, pid := range existingPairIDs {
		if excludePairID != "" && pid == excludePairID {
			continue
		}
		otherPair, err := h.app.FindRecordById("pairs", pid)
		if err != nil {
			continue
		}
		op1 := otherPair.GetString("player1")
		op2 := otherPair.GetString("player2")
		if p1 == op1 || p1 == op2 || p2 == op1 || p2 == op2 {
			return fmt.Errorf("un jugador ya participa en otra pareja de esta competición")
		}
	}
	return nil
}
