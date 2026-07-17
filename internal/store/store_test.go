package store

import (
	"path/filepath"
	"testing"

	"complykar/internal/notify"
	"complykar/internal/rules"
)

func sampleProfile() rules.Profile {
	return rules.Profile{
		BusinessName: "Sharma Kirana", OwnerName: "Meena Sharma", Phone: "+91-9811122233",
		Category: "kirana", State: "Maharashtra", Employees: 2, TurnoverBand: "20L-40L",
		GSTRegistered: true, HasPremises: true,
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("load empty: %v", err)
	}

	p := sampleProfile()
	obls := rules.Evaluate(p, rules.Anchor())
	msgs := []notify.Message{{ID: "wamid.MOCK-1", To: p.Phone, Channel: "whatsapp", Lang: "en", Kind: "reminder", Body: "hello"}}
	if err := s.SetProfile(p, obls, msgs); err != nil {
		t.Fatalf("set profile: %v", err)
	}
	if err := s.MarkFiled(Filing{ObligationID: "gstr1-qrmp", ObligationName: "GSTR-1 (QRMP, Quarterly)", DueDate: "2026-07-13", FiledAt: "2026-07-17T10:00:00Z"}); err != nil {
		t.Fatalf("mark filed: %v", err)
	}

	// A brand-new store loading the same file must see identical state.
	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	snap := s2.Snapshot()
	if snap.Profile == nil || snap.Profile.BusinessName != "Sharma Kirana" {
		t.Fatalf("profile did not survive reload: %+v", snap.Profile)
	}
	if len(snap.Obligations) != len(obls) {
		t.Fatalf("obligations count %d, want %d", len(snap.Obligations), len(obls))
	}
	if len(snap.Filings) != 1 || snap.Filings[0].DueDate != "2026-07-13" {
		t.Fatalf("filings did not survive reload: %+v", snap.Filings)
	}
	if len(snap.Outbox) != 1 || snap.Outbox[0].ID != "wamid.MOCK-1" {
		t.Fatalf("outbox did not survive reload: %+v", snap.Outbox)
	}
}

func TestMarkFiledRejectsDuplicates(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "store.json"))
	f := Filing{ObligationID: "esi", DueDate: "2026-07-15", FiledAt: "2026-07-17T10:00:00Z"}
	if err := s.MarkFiled(f); err != nil {
		t.Fatalf("first filing should succeed: %v", err)
	}
	if err := s.MarkFiled(f); err == nil {
		t.Fatal("duplicate filing should be rejected")
	}
	// Same obligation, different due date is fine (history keeps both).
	f2 := f
	f2.DueDate = "2026-08-15"
	if err := s.MarkFiled(f2); err != nil {
		t.Fatalf("different due date should succeed: %v", err)
	}
	if got := len(s.Snapshot().Filings); got != 2 {
		t.Fatalf("expected 2 filings in history, got %d", got)
	}
}

func TestSetProfilePreservesFilingHistory(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "store.json"))
	p := sampleProfile()
	if err := s.SetProfile(p, rules.Evaluate(p, rules.Anchor()), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkFiled(Filing{ObligationID: "professional-tax", DueDate: "2026-06-20", FiledAt: "2026-07-17T09:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	// Owner edits the profile: history must be preserved, outbox replaced.
	p.Employees = 15
	if err := s.SetProfile(p, rules.Evaluate(p, rules.Anchor()), nil); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if len(snap.Filings) != 1 {
		t.Fatalf("filing history lost on profile update: %+v", snap.Filings)
	}
	if len(snap.Outbox) != 0 {
		t.Fatalf("outbox should be replaced on profile update, got %d", len(snap.Outbox))
	}
}
