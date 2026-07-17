// Package rules implements ComplyKar's data-driven compliance rules engine.
//
// Rules are encoded as declarative Go structs (a catalog of Rule values whose
// conditions are lists of typed Clauses). A small evaluator matches a business
// Profile against the catalog and produces Obligations with plain-language
// "why it applies" text and internally consistent due dates computed from a
// fixed anchor date (2026-07-17).
//
// Everything here is educational, not legal advice.
package rules

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"complykar/internal/money"
)

// AnchorDate is the fixed "today" for all due-date math (demo determinism).
const AnchorDate = "2026-07-17"

// Anchor returns the anchor date at midnight UTC.
func Anchor() time.Time {
	t, _ := time.Parse("2006-01-02", AnchorDate)
	return t
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// Profile captures what a mom-and-pop owner tells us about their business.
type Profile struct {
	BusinessName  string `json:"businessName"`
	OwnerName     string `json:"ownerName"`
	Phone         string `json:"phone"`
	Category      string `json:"category"`
	State         string `json:"state"`
	Employees     int    `json:"employees"`
	TurnoverBand  string `json:"turnoverBand"`
	GSTRegistered bool   `json:"gstRegistered"`
	SellsFood     bool   `json:"sellsFood"`
	HasPremises   bool   `json:"hasPremises"`
	Interstate    bool   `json:"interstate"`
}

// Option is a value/label pair for form dropdowns.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// CategoryOptions lists supported business categories.
var CategoryOptions = []Option{
	{Value: "kirana", Label: "Kirana / General Store"},
	{Value: "salon", Label: "Salon / Beauty Parlour"},
	{Value: "restaurant", Label: "Restaurant / QSR"},
	{Value: "pharmacy", Label: "Pharmacy / Medical Store"},
	{Value: "coaching", Label: "Coaching Institute"},
	{Value: "boutique", Label: "Boutique / Tailor"},
	{Value: "gym", Label: "Gym / Fitness Studio"},
	{Value: "manufacturer", Label: "Small Manufacturer"},
}

// States lists the ten supported major states.
var States = []string{
	"Maharashtra", "Karnataka", "Tamil Nadu", "Delhi", "Uttar Pradesh",
	"Gujarat", "West Bengal", "Rajasthan", "Telangana", "Kerala",
}

// BandOptions lists annual turnover bands, ordered low to high.
var BandOptions = []Option{
	{Value: "<20L", Label: "Under ₹20 L"},
	{Value: "20L-40L", Label: "₹20 L – ₹40 L"},
	{Value: "40L-1.5Cr", Label: "₹40 L – ₹1.5 Cr"},
	{Value: "1.5Cr-5Cr", Label: "₹1.5 Cr – ₹5 Cr"},
	{Value: ">5Cr", Label: "Above ₹5 Cr"},
}

// whyLabels are human phrasings used inside "why it applies" sentences.
var whyLabels = map[string]string{
	"kirana":       "kirana store",
	"salon":        "salon",
	"restaurant":   "restaurant/QSR",
	"pharmacy":     "pharmacy",
	"coaching":     "coaching institute",
	"boutique":     "boutique/tailoring shop",
	"gym":          "gym",
	"manufacturer": "manufacturing unit",
}

var bandPretty = map[string]string{
	"<20L":      "under ₹20 L",
	"20L-40L":   "₹20 L–₹40 L",
	"40L-1.5Cr": "₹40 L–₹1.5 Cr",
	"1.5Cr-5Cr": "₹1.5 Cr–₹5 Cr",
	">5Cr":      "above ₹5 Cr",
}

// goodsCategories primarily supply goods; the rest are treated as services
// for GST threshold purposes (40 L goods / 20 L services).
var goodsCategories = map[string]bool{
	"kirana": true, "pharmacy": true, "boutique": true, "manufacturer": true,
}

func bandIndex(b string) int {
	for i, o := range BandOptions {
		if o.Value == b {
			return i
		}
	}
	return -1
}

func sectorOf(category string) string {
	if goodsCategories[category] {
		return "goods"
	}
	return "services"
}

// Normalize trims free-text fields.
func (p *Profile) Normalize() {
	p.BusinessName = strings.TrimSpace(p.BusinessName)
	p.OwnerName = strings.TrimSpace(p.OwnerName)
	p.Phone = strings.TrimSpace(p.Phone)
	p.Category = strings.TrimSpace(p.Category)
	p.State = strings.TrimSpace(p.State)
	p.TurnoverBand = strings.TrimSpace(p.TurnoverBand)
}

// Validate checks enum membership and sane numeric ranges.
func (p Profile) Validate() error {
	if p.BusinessName == "" {
		return fmt.Errorf("businessName is required")
	}
	valid := false
	for _, o := range CategoryOptions {
		if o.Value == p.Category {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown category %q", p.Category)
	}
	valid = false
	for _, s := range States {
		if s == p.State {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown state %q", p.State)
	}
	if bandIndex(p.TurnoverBand) < 0 {
		return fmt.Errorf("unknown turnoverBand %q", p.TurnoverBand)
	}
	if p.Employees < 0 || p.Employees > 10000 {
		return fmt.Errorf("employees must be between 0 and 10000")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Declarative conditions
// ---------------------------------------------------------------------------

// Clause is one typed predicate over a Profile field (or derived field).
type Clause struct {
	Field string   // category | state | sector | employees | turnoverIdx | gstRegistered | sellsFood | hasPremises | interstate
	Op    string   // in | gte | lte | eq
	Strs  []string // for string fields (Op "in")
	Num   int      // for numeric fields
	Bool  bool     // for boolean fields (Op "eq")
}

func (c Clause) matches(p Profile) bool {
	switch c.Field {
	case "category", "state", "sector":
		var v string
		switch c.Field {
		case "category":
			v = p.Category
		case "state":
			v = p.State
		case "sector":
			v = sectorOf(p.Category)
		}
		for _, s := range c.Strs {
			if s == v {
				return true
			}
		}
		return false
	case "employees", "turnoverIdx":
		var v int
		if c.Field == "employees" {
			v = p.Employees
		} else {
			v = bandIndex(p.TurnoverBand)
		}
		switch c.Op {
		case "gte":
			return v >= c.Num
		case "lte":
			return v <= c.Num
		case "eq":
			return v == c.Num
		}
		return false
	case "gstRegistered", "sellsFood", "hasPremises", "interstate":
		var v bool
		switch c.Field {
		case "gstRegistered":
			v = p.GSTRegistered
		case "sellsFood":
			v = p.SellsFood
		case "hasPremises":
			v = p.HasPremises
		case "interstate":
			v = p.Interstate
		}
		return v == c.Bool
	}
	return false
}

// Cond is an all-of clause group with its own "why it applies" template.
type Cond struct {
	Clauses []Clause
	Why     string // may contain {category} {state} {turnover} {employees}
}

// Clause constructors keep the catalog readable.
func catIn(vals ...string) Clause { return Clause{Field: "category", Op: "in", Strs: vals} }
func stateIn(vals ...string) Clause {
	return Clause{Field: "state", Op: "in", Strs: vals}
}
func sectorIs(v string) Clause  { return Clause{Field: "sector", Op: "in", Strs: []string{v}} }
func empGTE(n int) Clause       { return Clause{Field: "employees", Op: "gte", Num: n} }
func turnGTE(n int) Clause      { return Clause{Field: "turnoverIdx", Op: "gte", Num: n} }
func turnLTE(n int) Clause      { return Clause{Field: "turnoverIdx", Op: "lte", Num: n} }
func turnEq(n int) Clause       { return Clause{Field: "turnoverIdx", Op: "eq", Num: n} }
func is(field string) Clause    { return Clause{Field: field, Op: "eq", Bool: true} }
func isNot(field string) Clause { return Clause{Field: field, Op: "eq", Bool: false} }

func renderWhy(tpl string, p Profile) string {
	r := strings.NewReplacer(
		"{category}", whyLabels[p.Category],
		"{state}", p.State,
		"{turnover}", bandPretty[p.TurnoverBand],
		"{employees}", fmt.Sprintf("%d", p.Employees),
	)
	return r.Replace(tpl)
}

// ---------------------------------------------------------------------------
// Due-date specs
// ---------------------------------------------------------------------------

// DueSpec describes a deadline cadence relative to the anchor date.
type DueSpec struct {
	Kind      string // one-time | monthly | quarterly | annual
	Day       int    // day of month (monthly/quarterly/annual); clamped to month length
	Month     int    // month number for annual deadlines
	GraceDays int    // for one-time: due = anchor + GraceDays
}

// clampDate builds a date, clamping the day to the month's last day
// (e.g. day 31 in April becomes April 30).
func clampDate(y int, m time.Month, d int) time.Time {
	last := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > last {
		d = last
	}
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

var quarterlyDueMonths = map[time.Month]bool{
	time.January: true, time.April: true, time.July: true, time.October: true,
}

// occurrencesBetween lists due dates in [from, to] inclusive.
// anchor is needed for one-time specs (due = anchor + grace).
func occurrencesBetween(spec DueSpec, from, to, anchor time.Time) []time.Time {
	var out []time.Time
	add := func(d time.Time) {
		if !d.Before(from) && !d.After(to) {
			out = append(out, d)
		}
	}
	switch spec.Kind {
	case "one-time":
		add(anchor.AddDate(0, 0, spec.GraceDays))
	case "monthly":
		for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(to); m = m.AddDate(0, 1, 0) {
			add(clampDate(m.Year(), m.Month(), spec.Day))
		}
	case "quarterly":
		for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(to); m = m.AddDate(0, 1, 0) {
			if quarterlyDueMonths[m.Month()] {
				add(clampDate(m.Year(), m.Month(), spec.Day))
			}
		}
	case "annual":
		for y := from.Year(); y <= to.Year(); y++ {
			add(clampDate(y, time.Month(spec.Month), spec.Day))
		}
	}
	return out
}

// NextOccurrences returns the next n due dates on or after anchor.
func NextOccurrences(spec DueSpec, anchor time.Time, n int) []time.Time {
	occ := occurrencesBetween(spec, anchor, anchor.AddDate(3, 0, 0), anchor)
	if len(occ) > n {
		occ = occ[:n]
	}
	return occ
}

// LastBefore returns the most recent due date strictly before anchor,
// looking back at most lookbackDays. One-time specs have no past occurrences.
func LastBefore(spec DueSpec, anchor time.Time, lookbackDays int) (time.Time, bool) {
	occ := occurrencesBetween(spec, anchor.AddDate(0, 0, -lookbackDays), anchor.AddDate(0, 0, -1), anchor)
	if len(occ) == 0 {
		return time.Time{}, false
	}
	return occ[len(occ)-1], true
}

// ---------------------------------------------------------------------------
// Rules catalog
// ---------------------------------------------------------------------------

// Rule is one declarative compliance rule.
type Rule struct {
	ID         string
	Name       string
	Authority  string
	Frequency  string // one-time | monthly | quarterly | annual
	Due        DueSpec
	Penalty    string
	Docs       []string
	AnyOf      []Cond            // rule applies if any Cond's clauses all match
	StateNotes map[string]string // optional per-state notes appended to why
}

func (r Rule) applies(p Profile) (bool, string) {
	for _, c := range r.AnyOf {
		all := true
		for _, cl := range c.Clauses {
			if !cl.matches(p) {
				all = false
				break
			}
		}
		if all {
			return true, renderWhy(c.Why, p)
		}
	}
	return false, ""
}

// Catalog is the full declarative rule set, in display order.
var Catalog = []Rule{
	{
		ID:        "gst-registration",
		Name:      "GST Registration",
		Authority: "CBIC / State GST Department",
		Frequency: "one-time",
		Due:       DueSpec{Kind: "one-time", GraceDays: 30},
		Penalty: "10% of tax due (minimum " + money.FormatINR(10000) +
			"); 100% of tax due for deliberate evasion",
		Docs: []string{
			"PAN of proprietor", "Aadhaar", "Passport-size photograph",
			"Premises proof (rent agreement / electricity bill)",
			"Bank account proof (cancelled cheque)",
		},
		AnyOf: []Cond{
			{
				Clauses: []Clause{is("interstate"), isNot("gstRegistered")},
				Why:     "You sell to customers outside {state}. Interstate supply makes GST registration mandatory from the first rupee — no turnover threshold applies.",
			},
			{
				Clauses: []Clause{sectorIs("goods"), turnGTE(2), isNot("gstRegistered")},
				Why:     "Your {category} primarily supplies goods, and your turnover band ({turnover}) crosses the ₹40 L registration threshold for goods.",
			},
			{
				Clauses: []Clause{sectorIs("services"), turnGTE(1), isNot("gstRegistered")},
				Why:     "Your {category} is a service business, and your turnover band ({turnover}) crosses the ₹20 L registration threshold for services.",
			},
		},
	},
	{
		ID:        "gstr1-monthly",
		Name:      "GSTR-1 (Monthly Outward Supplies)",
		Authority: "GSTN / CBIC",
		Frequency: "monthly",
		Due:       DueSpec{Kind: "monthly", Day: 11},
		Penalty: "Late fee " + money.FormatINR(50) + "/day (" + money.FormatINR(20) +
			"/day for nil returns); delays also block your buyers' input tax credit",
		Docs: []string{"Sales register", "B2B invoice details", "Credit/debit notes", "HSN summary"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("gstRegistered"), turnGTE(4)},
			Why:     "You are GST-registered and your turnover ({turnover}) is above ₹5 Cr, so the QRMP scheme is not available — GSTR-1 must be filed monthly by the 11th.",
		}},
	},
	{
		ID:        "gstr3b-monthly",
		Name:      "GSTR-3B (Monthly Summary Return)",
		Authority: "GSTN / CBIC",
		Frequency: "monthly",
		Due:       DueSpec{Kind: "monthly", Day: 20},
		Penalty: "Late fee " + money.FormatINR(50) +
			"/day plus 18% p.a. interest on unpaid tax",
		Docs: []string{"Output tax summary", "Input tax credit ledger", "Cash/credit ledger balances"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("gstRegistered"), turnGTE(4)},
			Why:     "With turnover ({turnover}) above ₹5 Cr you file the GSTR-3B summary return and pay tax monthly by the 20th.",
		}},
	},
	{
		ID:        "gstr1-qrmp",
		Name:      "GSTR-1 (QRMP, Quarterly)",
		Authority: "GSTN / CBIC",
		Frequency: "quarterly",
		Due:       DueSpec{Kind: "quarterly", Day: 13},
		Penalty: "Late fee " + money.FormatINR(50) + "/day (" + money.FormatINR(20) +
			"/day for nil returns), capped per return",
		Docs: []string{"Quarterly sales register", "IFF uploads (if used)", "Credit/debit notes"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("gstRegistered"), turnLTE(3)},
			Why:     "You are GST-registered with turnover ({turnover}) at or below ₹5 Cr, so you qualify for the QRMP scheme — GSTR-1 is filed quarterly by the 13th of the month after each quarter.",
		}},
	},
	{
		ID:        "gstr3b-qrmp",
		Name:      "GSTR-3B (QRMP, Quarterly)",
		Authority: "GSTN / CBIC",
		Frequency: "quarterly",
		Due:       DueSpec{Kind: "quarterly", Day: 22},
		Penalty: "Late fee " + money.FormatINR(50) +
			"/day plus 18% p.a. interest on unpaid tax",
		Docs: []string{"Quarterly summary", "PMT-06 challans (monthly tax payment)", "ITC ledger"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("gstRegistered"), turnLTE(3)},
			Why:     "Under QRMP (turnover {turnover}, at or below ₹5 Cr) GSTR-3B is filed quarterly by the 22nd of the month after the quarter, with monthly tax paid via PMT-06 challan.",
		}},
	},
	{
		ID:        "gstr9-annual",
		Name:      "GSTR-9 (Annual Return)",
		Authority: "GSTN / CBIC",
		Frequency: "annual",
		Due:       DueSpec{Kind: "annual", Month: 12, Day: 31},
		Penalty: "Late fee " + money.FormatINR(200) +
			"/day (CGST + SGST), capped at 0.5% of turnover",
		Docs: []string{"Annual financials", "GSTR-1/GSTR-3B reconciliation", "ITC ledger", "HSN-wise summary"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("gstRegistered"), turnGTE(3)},
			Why:     "Your turnover band ({turnover}) is around/above the ₹2 Cr mark where the annual return becomes mandatory — GSTR-9 for FY 2025-26 is due 31 December 2026.",
		}},
	},
	{
		ID:        "udyam",
		Name:      "Udyam (MSME) Registration",
		Authority: "Ministry of MSME",
		Frequency: "one-time",
		Due:       DueSpec{Kind: "one-time", GraceDays: 30},
		Penalty: "No direct penalty, but you lose MSME benefits: collateral-free loans, " +
			"subsidies and delayed-payment interest under the MSMED Act",
		Docs: []string{"Aadhaar of proprietor", "PAN", "Bank details", "NIC activity code"},
		AnyOf: []Cond{{
			Clauses: nil,
			Why:     "Every micro or small business should register on the Udyam portal — it is free, takes about 10 minutes, and unlocks priority-sector lending and MSME schemes.",
		}},
	},
	{
		ID:        "shops-establishments",
		Name:      "Shops & Establishments Registration",
		Authority: "State Labour Department",
		Frequency: "one-time",
		Due:       DueSpec{Kind: "one-time", GraceDays: 30},
		Penalty: "Fines typically " + money.FormatINR(1000) + "–" + money.FormatINR(5000) +
			" plus per-day amounts; varies by state",
		Docs: []string{"Address proof of premises", "ID proof of proprietor", "Employee list", "Fee challan"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("hasPremises")},
			Why:     "You operate from a physical premises in {state}, so registration under the {state} Shops & Establishments Act applies.",
		}},
		StateNotes: map[string]string{
			"Maharashtra":   "Maharashtra: shops with fewer than 10 workers file an online intimation; 10 or more need full registration.",
			"Karnataka":     "Karnataka: register within 30 days of starting; renewal every 5 years.",
			"Tamil Nadu":    "Tamil Nadu: register under the TN Shops and Establishments Act within 30 days of opening.",
			"Delhi":         "Delhi: one-time online registration on the Labour Department portal.",
			"Uttar Pradesh": "Uttar Pradesh: register on the UP labour portal; renewal every 5 years.",
			"Gujarat":       "Gujarat: shops with fewer than 10 workers need only a one-time intimation.",
			"West Bengal":   "West Bengal: register within 30 days; renewal typically every 3 years.",
			"Rajasthan":     "Rajasthan: register via the SSO portal; small shops get long-validity certificates.",
			"Telangana":     "Telangana: register on the Labour Department portal; renewable for up to 5 years at a time.",
			"Kerala":        "Kerala: register with the local Labour Officer; renewal every 5 years.",
		},
	},
	{
		ID:        "fssai-registration",
		Name:      "FSSAI Basic Registration",
		Authority: "FSSAI / State Food Safety Department",
		Frequency: "annual",
		Due:       DueSpec{Kind: "annual", Month: 9, Day: 30},
		Penalty: "Selling food without registration: fine up to " + money.FormatINR(500000) +
			" and possible imprisonment",
		Docs: []string{"Form A", "Photo ID of proprietor", "Business address proof", "List of food categories"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("sellsFood"), turnEq(0)},
			Why:     "You sell food and your turnover ({turnover}) is under ₹12 L, so basic FSSAI registration (Form A) is sufficient. Keep it renewed — we track the renewal each year.",
		}},
	},
	{
		ID:        "fssai-state-license",
		Name:      "FSSAI State License",
		Authority: "FSSAI / State Food Safety Department",
		Frequency: "annual",
		Due:       DueSpec{Kind: "annual", Month: 9, Day: 30},
		Penalty: "Operating without a licence: fine up to " + money.FormatINR(500000) +
			" and imprisonment up to 6 months",
		Docs: []string{
			"Form B", "Premises layout plan", "Water test report",
			"List of food categories", "Proprietor ID and photo",
		},
		AnyOf: []Cond{{
			Clauses: []Clause{is("sellsFood"), turnGTE(1)},
			Why:     "You sell food and your turnover ({turnover}) is above ₹12 L, which moves you from basic registration to a State FSSAI licence (₹12 L–₹20 Cr slab, Form B).",
		}},
	},
	{
		ID:        "professional-tax",
		Name:      "Professional Tax (Employer)",
		Authority: "State Commercial Taxes Department",
		Frequency: "monthly",
		Due:       DueSpec{Kind: "monthly", Day: 20},
		Penalty:   "Interest 1.25%–2% per month plus penalty up to 50% of the amount due",
		Docs:      []string{"PT registration certificate (PTRC)", "Salary register", "Monthly challans"},
		AnyOf: []Cond{{
			Clauses: []Clause{
				stateIn("Maharashtra", "Karnataka", "Tamil Nadu", "Gujarat", "West Bengal", "Telangana", "Kerala"),
				empGTE(1),
			},
			Why: "{state} levies professional tax and you have {employees} employee(s) — deduct PT from salaries and remit it monthly.",
		}},
	},
	{
		ID:        "trade-license",
		Name:      "Municipal Trade License",
		Authority: "Municipal Corporation / Local Body",
		Frequency: "annual",
		Due:       DueSpec{Kind: "annual", Month: 3, Day: 31},
		Penalty:   "Late renewal fines up to 50% of the licence fee; risk of sealing for unlicensed trade",
		Docs:      []string{"Previous licence (if any)", "Property tax receipt", "Premises layout", "ID proof"},
		AnyOf: []Cond{{
			Clauses: []Clause{is("hasPremises")},
			Why:     "Running a {category} from a physical premises requires a trade licence from your local municipal body in {state}; renew it before the civic year ends (31 March).",
		}},
	},
	{
		ID:        "epf",
		Name:      "EPF Registration & Monthly ECR",
		Authority: "EPFO",
		Frequency: "monthly",
		Due:       DueSpec{Kind: "monthly", Day: 15},
		Penalty:   "Interest 12% p.a. plus damages of 5%–25% p.a. on delayed contributions",
		Docs:      []string{"EPF establishment code", "Employee UANs", "Wage register", "Monthly ECR file"},
		AnyOf: []Cond{{
			Clauses: []Clause{empGTE(20)},
			Why:     "You have {employees} employees — at 20 or more, EPF registration is mandatory and the monthly ECR with contributions is due by the 15th.",
		}},
	},
	{
		ID:        "esi",
		Name:      "ESI Registration & Monthly Contribution",
		Authority: "ESIC",
		Frequency: "monthly",
		Due:       DueSpec{Kind: "monthly", Day: 15},
		Penalty:   "Interest 12% p.a. plus damages up to 25%; prosecution possible for repeated default",
		Docs:      []string{"ESI code", "Employee declarations", "Wage register", "Monthly challans"},
		AnyOf: []Cond{{
			Clauses: []Clause{empGTE(10)},
			Why:     "You have {employees} employees — at 10 or more, ESI applies for staff earning up to ₹21,000/month; contributions are due by the 15th.",
		}},
	},
	{
		ID:        "tds-quarterly",
		Name:      "TDS Deduction & Quarterly Returns",
		Authority: "Income Tax Department",
		Frequency: "quarterly",
		Due:       DueSpec{Kind: "quarterly", Day: 31},
		Penalty: "Late-filing fee " + money.FormatINR(200) +
			"/day (section 234E) plus interest of 1%–1.5% per month",
		Docs: []string{"TAN", "Deductee PANs", "Challan details (deposit by the 7th monthly)", "Rent/contractor/salary ledgers"},
		AnyOf: []Cond{{
			Clauses: []Clause{turnGTE(3)},
			Why:     "With turnover ({turnover}) above ₹1.5 Cr you are likely to cross tax-audit thresholds, which makes TDS deduction applicable (rent 194-I, contractors 194C, professionals 194J). Deposit monthly by the 7th; file returns quarterly.",
		}},
	},
	{
		ID:        "drug-license",
		Name:      "Retail Drug License (Form 20/21)",
		Authority: "State Drugs Control Department",
		Frequency: "one-time",
		Due:       DueSpec{Kind: "one-time", GraceDays: 30},
		Penalty:   "Sale of medicines without a licence: imprisonment up to 3 years plus fine (Drugs & Cosmetics Act)",
		Docs: []string{
			"Registered pharmacist certificate and consent", "Premises plan (minimum area norms)",
			"Refrigerator purchase invoice", "Proprietor affidavits",
		},
		AnyOf: []Cond{{
			Clauses: []Clause{catIn("pharmacy")},
			Why:     "Running a pharmacy requires a retail drug licence with a registered pharmacist on the rolls; the licence is renewed every 5 years.",
		}},
	},
	{
		ID:        "fire-noc",
		Name:      "Fire Department NOC",
		Authority: "State Fire Services Department",
		Frequency: "annual",
		Due:       DueSpec{Kind: "annual", Month: 9, Day: 15},
		Penalty:   "Sealing risk and municipal fines; insurance claims may be rejected without a valid NOC",
		Docs: []string{
			"Building plan", "Fire-safety equipment invoices",
			"Occupancy certificate", "Previous NOC (for renewal)",
		},
		AnyOf: []Cond{{
			Clauses: []Clause{catIn("restaurant", "gym"), is("hasPremises")},
			Why:     "A {category} with public footfall on a physical premises typically needs a Fire NOC in {state}; treat it as an annual renewal (exact rules depend on seating/floor area).",
		}},
	},
}

var catalogByID = map[string]Rule{}

func init() {
	for _, r := range Catalog {
		catalogByID[r.ID] = r
	}
}

// ---------------------------------------------------------------------------
// Evaluation output
// ---------------------------------------------------------------------------

// Obligation is one applicable compliance duty for a specific profile.
type Obligation struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	WhyItApplies   string   `json:"whyItApplies"`
	Authority      string   `json:"authority"`
	Frequency      string   `json:"frequency"`
	NextDueDates   []string `json:"nextDueDates"`
	PenaltySummary string   `json:"penaltySummary"`
	DocsNeeded     []string `json:"docsNeeded"`
}

// Evaluate matches a profile against the catalog and returns obligations in
// catalog order with the next three due dates computed from anchor.
func Evaluate(p Profile, anchor time.Time) []Obligation {
	var out []Obligation
	for _, r := range Catalog {
		ok, why := r.applies(p)
		if !ok {
			continue
		}
		if note, has := r.StateNotes[p.State]; has {
			why = why + " " + note
		}
		var dues []string
		for _, d := range NextOccurrences(r.Due, anchor, 3) {
			dues = append(dues, d.Format("2006-01-02"))
		}
		out = append(out, Obligation{
			ID:             r.ID,
			Name:           r.Name,
			WhyItApplies:   why,
			Authority:      r.Authority,
			Frequency:      r.Frequency,
			NextDueDates:   dues,
			PenaltySummary: r.Penalty,
			DocsNeeded:     append([]string(nil), r.Docs...),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Deadlines (calendar instances)
// ---------------------------------------------------------------------------

// Deadline is one dated instance of an obligation, used by the calendar,
// the reminder generator and the filed-state tracking.
type Deadline struct {
	ObligationID   string `json:"obligationId"`
	ObligationName string `json:"obligationName"`
	Frequency      string `json:"frequency"`
	DueDate        string `json:"dueDate"`
	DaysLeft       int    `json:"daysLeft"`
	Overdue        bool   `json:"overdue"`
	Filed          bool   `json:"filed"`
	FiledAt        string `json:"filedAt,omitempty"`
	PenaltySummary string `json:"penaltySummary"`
}

// BuildDeadlines expands obligations into dated instances: the most recent
// missed occurrence within lookbackDays (overdue candidates) plus every
// occurrence within the next horizonDays. filed reports (filedAt, true) when
// a given obligation+date has been marked filed.
func BuildDeadlines(obls []Obligation, anchor time.Time, horizonDays, lookbackDays int, filed func(obligationID, dueDate string) (string, bool)) []Deadline {
	end := anchor.AddDate(0, 0, horizonDays)
	var out []Deadline
	for _, ob := range obls {
		r, ok := catalogByID[ob.ID]
		if !ok {
			continue
		}
		var dates []time.Time
		if lb, ok := LastBefore(r.Due, anchor, lookbackDays); ok {
			dates = append(dates, lb)
		}
		dates = append(dates, occurrencesBetween(r.Due, anchor, end, anchor)...)
		for _, d := range dates {
			ds := d.Format("2006-01-02")
			filedAt, isFiled := filed(ob.ID, ds)
			out = append(out, Deadline{
				ObligationID:   ob.ID,
				ObligationName: ob.Name,
				Frequency:      ob.Frequency,
				DueDate:        ds,
				DaysLeft:       int(d.Sub(anchor).Hours() / 24),
				Overdue:        d.Before(anchor) && !isFiled,
				Filed:          isFiled,
				FiledAt:        filedAt,
				PenaltySummary: ob.PenaltySummary,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DueDate != out[j].DueDate {
			return out[i].DueDate < out[j].DueDate
		}
		return out[i].ObligationName < out[j].ObligationName
	})
	return out
}
