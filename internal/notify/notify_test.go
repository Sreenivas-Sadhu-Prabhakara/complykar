package notify

import (
	"strings"
	"testing"

	"complykar/internal/rules"
)

func TestMockSenderIsDeterministic(t *testing.T) {
	s := MockWhatsApp{}
	a, err := s.Send("+91-9845012345", "hello")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Send("+91-9845012345", "hello")
	if a != b {
		t.Fatalf("same input produced different ids: %s vs %s", a, b)
	}
	c, _ := s.Send("+91-9845012345", "different body")
	if a == c {
		t.Fatal("different bodies should produce different ids")
	}
	if !strings.HasPrefix(a, "wamid.MOCK-") {
		t.Fatalf("unexpected id format %q", a)
	}
	if p := MockPhone("Anna's Kitchen"); p != MockPhone("Anna's Kitchen") {
		t.Fatal("MockPhone not deterministic")
	}
	if !strings.HasPrefix(MockPhone("seed"), "+91-98") {
		t.Fatalf("MockPhone should look Indian, got %s", MockPhone("seed"))
	}
}

func TestBuildRemindersWindowAndLanguages(t *testing.T) {
	p := rules.Profile{
		BusinessName: "Anna's Kitchen", OwnerName: "Anjali Rao", Phone: "+91-9845012345",
		Category: "restaurant", State: "Karnataka", Employees: 12, TurnoverBand: "40L-1.5Cr",
		GSTRegistered: true, SellsFood: true, HasPremises: true,
	}
	dls := []rules.Deadline{
		{ObligationID: "gstr3b-qrmp", ObligationName: "GSTR-3B (QRMP, Quarterly)", DueDate: "2026-07-22", DaysLeft: 5, PenaltySummary: "late fee"},
		{ObligationID: "gstr1-qrmp", ObligationName: "GSTR-1 (QRMP, Quarterly)", DueDate: "2026-07-13", DaysLeft: -4, Overdue: true, PenaltySummary: "late fee"},
		{ObligationID: "fire-noc", ObligationName: "Fire Department NOC", DueDate: "2026-09-15", DaysLeft: 60, PenaltySummary: "sealing"},
		{ObligationID: "esi", ObligationName: "ESI", DueDate: "2026-07-15", DaysLeft: -2, Overdue: true, Filed: true, PenaltySummary: "interest"},
	}
	msgs := BuildReminders(p, dls, MockWhatsApp{})

	// Two eligible deadlines (within 14 days or overdue+unfiled) x two languages.
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	var en, hi, overdue int
	for _, m := range msgs {
		switch m.Lang {
		case "en":
			en++
		case "hi":
			hi++
		}
		if m.Kind == "overdue" {
			overdue++
		}
		if m.To != p.Phone || m.Channel != "whatsapp" {
			t.Errorf("bad envelope: %+v", m)
		}
		if !strings.Contains(m.Body, "Anjali Rao") {
			t.Errorf("body should address the owner: %q", m.Body)
		}
	}
	if en != 2 || hi != 2 {
		t.Fatalf("expected 2 English + 2 Hindi, got en=%d hi=%d", en, hi)
	}
	if overdue != 2 {
		t.Fatalf("expected 2 overdue messages (en+hi), got %d", overdue)
	}
	for _, m := range msgs {
		if m.Lang == "hi" && !strings.Contains(m.Body, "नमस्ते") {
			t.Errorf("hindi body should be in Devanagari: %q", m.Body)
		}
		if m.Lang == "en" && !strings.Contains(m.Body, "not legal advice") {
			t.Errorf("english body must carry the disclaimer: %q", m.Body)
		}
	}
}

func TestFiledConfirmationBothLanguages(t *testing.T) {
	p := rules.Profile{BusinessName: "Anna's Kitchen", OwnerName: "Anjali Rao", Phone: "+91-9845012345"}
	msgs := BuildFiledConfirmation(p, "gstr1-qrmp", "GSTR-1 (QRMP, Quarterly)", "2026-07-13", MockWhatsApp{})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 confirmations, got %d", len(msgs))
	}
	if msgs[0].Kind != "confirmation" || msgs[1].Kind != "confirmation" {
		t.Fatal("kind should be confirmation")
	}
	if !strings.Contains(msgs[0].Body, "13 Jul 2026") {
		t.Errorf("confirmation should carry the pretty due date: %q", msgs[0].Body)
	}
}
