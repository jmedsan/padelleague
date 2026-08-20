package handlers

import (
	"testing"
)

func TestParseProposalData_ValidJSON(t *testing.T) {
	raw := `{"date":"2026-10-15","time":"19:30","venue_id":"abc123","venue_name":"Padel 360","venue_text":""}`
	pd := ParseProposalData(raw)
	if pd == nil {
		t.Fatal("expected non-nil ProposalData")
	}
	if pd.Date != "2026-10-15" {
		t.Errorf("date = %q, want %q", pd.Date, "2026-10-15")
	}
	if pd.Time != "19:30" {
		t.Errorf("time = %q, want %q", pd.Time, "19:30")
	}
	if pd.VenueID != "abc123" {
		t.Errorf("venue_id = %q, want %q", pd.VenueID, "abc123")
	}
	if pd.VenueName != "Padel 360" {
		t.Errorf("venue_name = %q, want %q", pd.VenueName, "Padel 360")
	}
}

func TestParseProposalData_Map(t *testing.T) {
	raw := map[string]any{
		"date":       "2026-11-01",
		"time":       "20:00",
		"venue_id":   "",
		"venue_name": "Mi casa",
		"venue_text": "Mi casa",
	}
	pd := ParseProposalData(raw)
	if pd == nil {
		t.Fatal("expected non-nil ProposalData")
	}
	if pd.Date != "2026-11-01" {
		t.Errorf("date = %q, want %q", pd.Date, "2026-11-01")
	}
	if pd.VenueName != "Mi casa" {
		t.Errorf("venue_name = %q, want %q", pd.VenueName, "Mi casa")
	}
}

func TestParseProposalData_Nil(t *testing.T) {
	pd := ParseProposalData(nil)
	if pd != nil {
		t.Error("expected nil for nil input")
	}
}

func TestParseProposalData_EmptyString(t *testing.T) {
	pd := ParseProposalData("")
	if pd != nil {
		t.Error("expected nil for empty string")
	}
}

func TestParseProposalData_Malformed(t *testing.T) {
	pd := ParseProposalData("not json")
	if pd != nil {
		t.Error("expected nil for malformed JSON")
	}
}
