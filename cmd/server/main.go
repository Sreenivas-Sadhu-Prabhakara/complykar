// Command server runs ComplyKar: the JSON API under /api/v1/* plus the
// embedded web UI, on a single port (PORT env, default 8103).
package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	complykar "complykar"
	"complykar/internal/notify"
	"complykar/internal/rules"
	"complykar/internal/store"
)

const (
	defaultPort  = "8103"
	horizonDays  = 90 // calendar window
	lookbackDays = 60 // how far back we surface missed (overdue) deadlines
	disclaimer   = "Educational tool, not legal advice — confirm with your CA."
)

type server struct {
	st     *store.Store
	sender notify.Sender
	waMode string
	anchor time.Time
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	waMode := os.Getenv("WHATSAPP_PROVIDER")
	if waMode == "" {
		waMode = "mock"
	}
	if waMode != "mock" {
		log.Printf("WHATSAPP_PROVIDER=%q requested but only the mock sender is built in; using mock", waMode)
		waMode = "mock"
	}

	st := store.New("./data/store.json")
	if err := st.Load(); err != nil {
		log.Fatalf("load store: %v", err)
	}

	s := &server{st: st, sender: notify.MockWhatsApp{}, waMode: waMode, anchor: rules.Anchor()}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/meta", s.handleMeta)
	mux.HandleFunc("POST /api/v1/profile", s.handleSetProfile)
	mux.HandleFunc("GET /api/v1/profile", s.handleGetProfile)
	mux.HandleFunc("GET /api/v1/obligations", s.handleObligations)
	mux.HandleFunc("GET /api/v1/calendar", s.handleCalendar)
	mux.HandleFunc("POST /api/v1/obligations/{id}/filed", s.handleFiled)
	mux.HandleFunc("GET /api/v1/outbox", s.handleOutbox)

	web, err := fs.Sub(complykar.WebFS, "web")
	if err != nil {
		log.Fatalf("embed web: %v", err)
	}
	mux.Handle("/", http.FileServerFS(web))

	log.Printf("ComplyKar listening on http://localhost:%s (anchor date %s, whatsapp=%s)", port, rules.AnchorDate, waMode)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *server) filedLookup(snap store.State) func(string, string) (string, bool) {
	idx := make(map[string]string, len(snap.Filings))
	for _, f := range snap.Filings {
		idx[f.ObligationID+"|"+f.DueDate] = f.FiledAt
	}
	return func(obID, due string) (string, bool) {
		at, ok := idx[obID+"|"+due]
		return at, ok
	}
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"app":        "complykar",
		"anchorDate": rules.AnchorDate,
		"providers":  map[string]string{"whatsapp": s.waMode},
	})
}

func (s *server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"categories":    rules.CategoryOptions,
		"states":        rules.States,
		"turnoverBands": rules.BandOptions,
		"anchorDate":    rules.AnchorDate,
		"priceMonthly":  "₹499/mo",
		"disclaimer":    disclaimer,
	})
}

func (s *server) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	var p rules.Profile
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read body")
		return
	}
	if err := json.Unmarshal(body, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	p.Normalize()
	if p.OwnerName == "" {
		p.OwnerName = "Owner"
	}
	if p.Phone == "" {
		p.Phone = notify.MockPhone(p.BusinessName + "|" + p.OwnerName)
	}
	if err := p.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	obls := rules.Evaluate(p, s.anchor)

	// Reminders are regenerated against existing filing history.
	snap := s.st.Snapshot()
	dls := rules.BuildDeadlines(obls, s.anchor, horizonDays, lookbackDays, s.filedLookup(snap))
	reminders := notify.BuildReminders(p, dls, s.sender)

	if err := s.st.SetProfile(p, obls, reminders); err != nil {
		writeErr(w, http.StatusInternalServerError, "persist failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profile":     p,
		"obligations": obls,
		"reminders":   len(reminders),
		"disclaimer":  disclaimer,
	})
}

func (s *server) handleGetProfile(w http.ResponseWriter, _ *http.Request) {
	snap := s.st.Snapshot()
	if snap.Profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"profileSet": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profileSet": true, "profile": snap.Profile})
}

func (s *server) handleObligations(w http.ResponseWriter, _ *http.Request) {
	snap := s.st.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"profileSet":  snap.Profile != nil,
		"obligations": snap.Obligations,
		"disclaimer":  disclaimer,
	})
}

type calendarMonth struct {
	Month     string           `json:"month"`
	Label     string           `json:"label"`
	Deadlines []rules.Deadline `json:"deadlines"`
}

func (s *server) handleCalendar(w http.ResponseWriter, _ *http.Request) {
	snap := s.st.Snapshot()
	if snap.Profile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"profileSet": false, "months": []calendarMonth{}})
		return
	}
	dls := rules.BuildDeadlines(snap.Obligations, s.anchor, horizonDays, lookbackDays, s.filedLookup(snap))

	byMonth := map[string][]rules.Deadline{}
	var keys []string
	for _, d := range dls {
		k := d.DueDate[:7]
		if _, seen := byMonth[k]; !seen {
			keys = append(keys, k)
		}
		byMonth[k] = append(byMonth[k], d)
	}
	sort.Strings(keys)
	months := make([]calendarMonth, 0, len(keys))
	for _, k := range keys {
		t, _ := time.Parse("2006-01", k)
		months = append(months, calendarMonth{Month: k, Label: t.Format("January 2006"), Deadlines: byMonth[k]})
	}

	summary := map[string]int{"total": len(dls)}
	for _, d := range dls {
		if d.Overdue {
			summary["overdue"]++
		}
		if d.Filed {
			summary["filed"]++
		}
		if !d.Filed && d.DaysLeft >= 0 && d.DaysLeft <= notify.ReminderWindowDays {
			summary["dueSoon"]++
		}
	}

	history := append([]store.Filing(nil), snap.Filings...)
	sort.Slice(history, func(i, j int) bool { return history[i].FiledAt > history[j].FiledAt })

	writeJSON(w, http.StatusOK, map[string]any{
		"profileSet": true,
		"anchorDate": rules.AnchorDate,
		"windowDays": horizonDays,
		"summary":    summary,
		"months":     months,
		"history":    history,
	})
}

func (s *server) handleFiled(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap := s.st.Snapshot()
	if snap.Profile == nil {
		writeErr(w, http.StatusBadRequest, "no profile set yet")
		return
	}
	var ob *rules.Obligation
	for i := range snap.Obligations {
		if snap.Obligations[i].ID == id {
			ob = &snap.Obligations[i]
			break
		}
	}
	if ob == nil {
		writeErr(w, http.StatusNotFound, "unknown obligation id "+id)
		return
	}

	var req struct {
		DueDate string `json:"dueDate"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req)
	}

	dls := rules.BuildDeadlines([]rules.Obligation{*ob}, s.anchor, horizonDays, lookbackDays, s.filedLookup(snap))
	var target *rules.Deadline
	if req.DueDate == "" {
		for i := range dls {
			if !dls[i].Filed {
				target = &dls[i]
				break
			}
		}
		if target == nil {
			writeErr(w, http.StatusBadRequest, "no unfiled deadlines in the current window")
			return
		}
	} else {
		for i := range dls {
			if dls[i].DueDate == req.DueDate {
				target = &dls[i]
				break
			}
		}
		if target == nil {
			writeErr(w, http.StatusBadRequest, "dueDate "+req.DueDate+" is not a deadline for "+id)
			return
		}
		if target.Filed {
			writeErr(w, http.StatusConflict, "already marked filed for "+req.DueDate)
			return
		}
	}

	filing := store.Filing{
		ObligationID:   ob.ID,
		ObligationName: ob.Name,
		DueDate:        target.DueDate,
		FiledAt:        time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.st.MarkFiled(filing); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	conf := notify.BuildFiledConfirmation(*snap.Profile, ob.ID, ob.Name, target.DueDate, s.sender)
	if err := s.st.AppendOutbox(conf...); err != nil {
		writeErr(w, http.StatusInternalServerError, "persist failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"filing": filing})
}

func (s *server) handleOutbox(w http.ResponseWriter, _ *http.Request) {
	snap := s.st.Snapshot()
	msgs := snap.Outbox
	// Newest-ish first: reverse of append order.
	rev := make([]notify.Message, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		rev = append(rev, msgs[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": s.waMode,
		"count":    len(rev),
		"messages": rev,
	})
}
