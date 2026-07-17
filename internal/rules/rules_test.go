package rules

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func ids(obls []Obligation) []string {
	out := make([]string, 0, len(obls))
	for _, o := range obls {
		out = append(out, o.ID)
	}
	return out
}

func has(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func baseProfile() Profile {
	return Profile{
		BusinessName: "Test Traders", OwnerName: "Ravi Kumar", Phone: "+91-9812345678",
		Category: "kirana", State: "Karnataka", Employees: 0, TurnoverBand: "<20L",
	}
}

// The spec's canonical case: 12-employee Karnataka restaurant at 60 L turnover,
// GST registered, sells food, has premises.
func TestKarnatakaRestaurantCanonicalCase(t *testing.T) {
	p := Profile{
		BusinessName: "Anna's Kitchen", OwnerName: "Anjali Rao", Phone: "+91-9845012345",
		Category: "restaurant", State: "Karnataka", Employees: 12, TurnoverBand: "40L-1.5Cr",
		GSTRegistered: true, SellsFood: true, HasPremises: true, Interstate: false,
	}
	got := ids(Evaluate(p, Anchor()))
	want := []string{
		"gstr1-qrmp", "gstr3b-qrmp", "udyam", "shops-establishments",
		"fssai-state-license", "professional-tax", "trade-license", "esi", "fire-noc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obligations mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestEvaluatorPermutations(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Profile)
		want    []string
		wantNot []string
	}{
		{
			name:    "UP kirana under threshold gets no GST or PT",
			mutate:  func(p *Profile) { p.State = "Uttar Pradesh"; p.HasPremises = true },
			want:    []string{"udyam", "shops-establishments", "trade-license"},
			wantNot: []string{"gst-registration", "professional-tax", "epf", "esi", "fssai-registration"},
		},
		{
			name: "Delhi salon crossing 20L services threshold must register for GST, no PT in Delhi",
			mutate: func(p *Profile) {
				p.Category = "salon"
				p.State = "Delhi"
				p.Employees = 3
				p.TurnoverBand = "20L-40L"
			},
			want:    []string{"gst-registration"},
			wantNot: []string{"professional-tax", "gstr1-qrmp", "gstr3b-qrmp", "gstr1-monthly"},
		},
		{
			name: "Gujarat manufacturer 25 staff 1.5-5Cr: EPF+ESI+TDS+QRMP",
			mutate: func(p *Profile) {
				p.Category = "manufacturer"
				p.State = "Gujarat"
				p.Employees = 25
				p.TurnoverBand = "1.5Cr-5Cr"
				p.GSTRegistered = true
				p.Interstate = true
			},
			want:    []string{"epf", "esi", "tds-quarterly", "gstr1-qrmp", "gstr3b-qrmp", "gstr9-annual", "professional-tax"},
			wantNot: []string{"gst-registration", "gstr1-monthly", "gstr3b-monthly"},
		},
		{
			name: "Maharashtra pharmacy: drug license, PT, no FSSAI when not selling food",
			mutate: func(p *Profile) {
				p.Category = "pharmacy"
				p.State = "Maharashtra"
				p.Employees = 5
				p.TurnoverBand = "40L-1.5Cr"
				p.GSTRegistered = true
				p.HasPremises = true
			},
			want:    []string{"drug-license", "professional-tax", "gstr1-qrmp", "trade-license"},
			wantNot: []string{"fssai-registration", "fssai-state-license", "fire-noc"},
		},
		{
			name: "TN coaching under 20L services: no GST registration needed, PT applies",
			mutate: func(p *Profile) {
				p.Category = "coaching"
				p.State = "Tamil Nadu"
				p.Employees = 8
			},
			want:    []string{"professional-tax", "udyam"},
			wantNot: []string{"gst-registration", "esi", "epf"},
		},
		{
			name: "Telangana restaurant above 5Cr: monthly GSTR not QRMP, GSTR-9, TDS",
			mutate: func(p *Profile) {
				p.Category = "restaurant"
				p.State = "Telangana"
				p.Employees = 30
				p.TurnoverBand = ">5Cr"
				p.GSTRegistered = true
				p.SellsFood = true
				p.HasPremises = true
			},
			want:    []string{"gstr1-monthly", "gstr3b-monthly", "gstr9-annual", "tds-quarterly", "fssai-state-license", "fire-noc", "epf", "esi", "professional-tax"},
			wantNot: []string{"gstr1-qrmp", "gstr3b-qrmp", "gst-registration"},
		},
		{
			name: "Rajasthan boutique with interstate sales: GST mandatory regardless of turnover",
			mutate: func(p *Profile) {
				p.Category = "boutique"
				p.State = "Rajasthan"
				p.Employees = 2
				p.Interstate = true
				p.HasPremises = true
			},
			want:    []string{"gst-registration", "trade-license", "shops-establishments"},
			wantNot: []string{"professional-tax", "gstr1-qrmp"},
		},
		{
			name: "Kerala gym 15 staff 20-40L registered: QRMP, ESI yes, EPF no, fire NOC",
			mutate: func(p *Profile) {
				p.Category = "gym"
				p.State = "Kerala"
				p.Employees = 15
				p.TurnoverBand = "20L-40L"
				p.GSTRegistered = true
				p.HasPremises = true
			},
			want:    []string{"gstr1-qrmp", "gstr3b-qrmp", "esi", "fire-noc", "professional-tax"},
			wantNot: []string{"epf", "gst-registration", "gstr9-annual"},
		},
		{
			name: "kirana food seller under 20L gets basic FSSAI registration not state license",
			mutate: func(p *Profile) {
				p.SellsFood = true
				p.HasPremises = true
			},
			want:    []string{"fssai-registration"},
			wantNot: []string{"fssai-state-license"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := baseProfile()
			tc.mutate(&p)
			if err := p.Validate(); err != nil {
				t.Fatalf("profile invalid: %v", err)
			}
			got := ids(Evaluate(p, Anchor()))
			for _, w := range tc.want {
				if !has(got, w) {
					t.Errorf("expected %s in %v", w, got)
				}
			}
			for _, w := range tc.wantNot {
				if has(got, w) {
					t.Errorf("did not expect %s in %v", w, got)
				}
			}
		})
	}
}

func TestWhyItAppliesReferencesAnswers(t *testing.T) {
	p := baseProfile()
	p.Category = "salon"
	p.State = "Karnataka"
	p.Employees = 4
	p.TurnoverBand = "20L-40L"
	obls := Evaluate(p, Anchor())
	for _, ob := range obls {
		if strings.TrimSpace(ob.WhyItApplies) == "" {
			t.Errorf("%s has empty whyItApplies", ob.ID)
		}
	}
	var gst, pt, se string
	for _, ob := range obls {
		switch ob.ID {
		case "gst-registration":
			gst = ob.WhyItApplies
		case "professional-tax":
			pt = ob.WhyItApplies
		case "shops-establishments":
			se = ob.WhyItApplies
		}
	}
	if !strings.Contains(gst, "₹20 L") || !strings.Contains(gst, "service") {
		t.Errorf("gst why should cite the 20L services threshold, got %q", gst)
	}
	if !strings.Contains(pt, "Karnataka") || !strings.Contains(pt, "4") {
		t.Errorf("pt why should cite state and employee count, got %q", pt)
	}
	if se != "" {
		t.Errorf("shops-establishments should not apply without premises, got %q", se)
	}
}

func TestDueDateMath(t *testing.T) {
	anchor := Anchor()
	fmtDates := func(ts []time.Time) []string {
		var out []string
		for _, tm := range ts {
			out = append(out, tm.Format("2006-01-02"))
		}
		return out
	}
	cases := []struct {
		name string
		spec DueSpec
		want []string
	}{
		{"monthly GSTR-1 day 11 skips passed July date", DueSpec{Kind: "monthly", Day: 11},
			[]string{"2026-08-11", "2026-09-11", "2026-10-11"}},
		{"monthly GSTR-3B day 20 includes July", DueSpec{Kind: "monthly", Day: 20},
			[]string{"2026-07-20", "2026-08-20", "2026-09-20"}},
		{"quarterly QRMP GSTR-1 day 13 next is October", DueSpec{Kind: "quarterly", Day: 13},
			[]string{"2026-10-13", "2027-01-13", "2027-04-13"}},
		{"quarterly QRMP GSTR-3B day 22 includes July", DueSpec{Kind: "quarterly", Day: 22},
			[]string{"2026-07-22", "2026-10-22", "2027-01-22"}},
		{"quarterly TDS day 31 clamps to month length", DueSpec{Kind: "quarterly", Day: 31},
			[]string{"2026-07-31", "2026-10-31", "2027-01-31"}},
		{"annual GSTR-9 Dec 31", DueSpec{Kind: "annual", Month: 12, Day: 31},
			[]string{"2026-12-31", "2027-12-31", "2028-12-31"}},
		{"annual trade license Mar 31 rolls to next year", DueSpec{Kind: "annual", Month: 3, Day: 31},
			[]string{"2027-03-31", "2028-03-31", "2029-03-31"}},
		{"one-time 30-day grace", DueSpec{Kind: "one-time", GraceDays: 30},
			[]string{"2026-08-16"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fmtDates(NextOccurrences(tc.spec, anchor, 3))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestLastBefore(t *testing.T) {
	anchor := Anchor()
	cases := []struct {
		name   string
		spec   DueSpec
		wantOK bool
		want   string
	}{
		{"monthly day 11 most recent is July 11", DueSpec{Kind: "monthly", Day: 11}, true, "2026-07-11"},
		{"monthly day 20 most recent is June 20", DueSpec{Kind: "monthly", Day: 20}, true, "2026-06-20"},
		{"quarterly day 13 most recent is July 13", DueSpec{Kind: "quarterly", Day: 13}, true, "2026-07-13"},
		{"quarterly day 22 April is outside 60-day lookback", DueSpec{Kind: "quarterly", Day: 22}, false, ""},
		{"annual outside lookback", DueSpec{Kind: "annual", Month: 12, Day: 31}, false, ""},
		{"one-time has no past occurrence", DueSpec{Kind: "one-time", GraceDays: 30}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := LastBefore(tc.spec, anchor, 60)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if ok && got.Format("2006-01-02") != tc.want {
				t.Errorf("got %s want %s", got.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestObligationNextDueDatesUseAnchor(t *testing.T) {
	p := baseProfile()
	p.GSTRegistered = true
	p.TurnoverBand = "40L-1.5Cr"
	obls := Evaluate(p, Anchor())
	for _, ob := range obls {
		if ob.ID == "gstr3b-qrmp" {
			want := []string{"2026-07-22", "2026-10-22", "2027-01-22"}
			if !reflect.DeepEqual(ob.NextDueDates, want) {
				t.Fatalf("gstr3b-qrmp dates %v want %v", ob.NextDueDates, want)
			}
			return
		}
	}
	t.Fatal("gstr3b-qrmp not found")
}

func TestBuildDeadlinesOverdueAndFiledTransitions(t *testing.T) {
	p := baseProfile()
	p.State = "Karnataka"
	p.Employees = 12
	p.Category = "restaurant"
	p.TurnoverBand = "40L-1.5Cr"
	p.GSTRegistered = true
	p.SellsFood = true
	p.HasPremises = true

	obls := Evaluate(p, Anchor())
	noneFiled := func(string, string) (string, bool) { return "", false }
	dls := BuildDeadlines(obls, Anchor(), 90, 60, noneFiled)

	find := func(dl []Deadline, id, date string) *Deadline {
		for i := range dl {
			if dl[i].ObligationID == id && dl[i].DueDate == date {
				return &dl[i]
			}
		}
		return nil
	}

	// QRMP GSTR-1 for Apr-Jun quarter was due 2026-07-13: 4 days overdue.
	d := find(dls, "gstr1-qrmp", "2026-07-13")
	if d == nil {
		t.Fatal("expected gstr1-qrmp 2026-07-13 deadline")
	}
	if !d.Overdue || d.Filed || d.DaysLeft != -4 {
		t.Fatalf("unexpected overdue deadline state: %+v", d)
	}

	// Mark it filed: overdue must clear, filed metadata must appear.
	filedAt := "2026-07-17T10:00:00Z"
	filedLookup := func(id, date string) (string, bool) {
		if id == "gstr1-qrmp" && date == "2026-07-13" {
			return filedAt, true
		}
		return "", false
	}
	dls2 := BuildDeadlines(obls, Anchor(), 90, 60, filedLookup)
	d2 := find(dls2, "gstr1-qrmp", "2026-07-13")
	if d2 == nil || !d2.Filed || d2.Overdue || d2.FiledAt != filedAt {
		t.Fatalf("filed transition failed: %+v", d2)
	}

	// All deadlines stay inside lookback/horizon and are sorted by date.
	lo, hi := "2026-05-18", "2026-10-15"
	for i, dl := range dls {
		if dl.DueDate < lo || dl.DueDate > hi {
			t.Errorf("deadline %s %s outside window [%s, %s]", dl.ObligationID, dl.DueDate, lo, hi)
		}
		if i > 0 && dls[i-1].DueDate > dl.DueDate {
			t.Errorf("deadlines not sorted at index %d", i)
		}
	}
}

func TestEvaluateIsDeterministic(t *testing.T) {
	p := baseProfile()
	p.GSTRegistered = true
	p.SellsFood = true
	p.HasPremises = true
	p.TurnoverBand = "1.5Cr-5Cr"
	p.Employees = 22
	a := Evaluate(p, Anchor())
	b := Evaluate(p, Anchor())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Evaluate is not deterministic for identical input")
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	p := baseProfile()
	p.Category = "spaceship"
	if err := p.Validate(); err == nil {
		t.Error("expected error for unknown category")
	}
	p = baseProfile()
	p.State = "Goa"
	if err := p.Validate(); err == nil {
		t.Error("expected error for unsupported state")
	}
	p = baseProfile()
	p.TurnoverBand = "9Cr"
	if err := p.Validate(); err == nil {
		t.Error("expected error for unknown turnover band")
	}
	p = baseProfile()
	p.Employees = -1
	if err := p.Validate(); err == nil {
		t.Error("expected error for negative employees")
	}
}
