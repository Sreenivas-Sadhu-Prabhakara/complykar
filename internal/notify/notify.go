// Package notify generates WhatsApp-style compliance reminders (English and
// Hindi) and delivers them through a Sender. Only a deterministic mock sender
// is implemented — see README for the env vars that would switch it live.
package notify

import (
	"fmt"
	"hash/fnv"
	"time"

	"complykar/internal/rules"
)

// ReminderWindowDays: deadlines due within this many days get a reminder.
const ReminderWindowDays = 14

// createdAtStamp keeps outbox timestamps deterministic against the demo anchor.
var createdAtStamp = rules.AnchorDate + "T09:00:00+05:30"

// Message is one outbound WhatsApp-style message in the mock outbox.
type Message struct {
	ID             string `json:"id"`
	To             string `json:"to"`
	Channel        string `json:"channel"`
	Lang           string `json:"lang"` // en | hi
	Kind           string `json:"kind"` // reminder | overdue | confirmation
	ObligationID   string `json:"obligationId"`
	ObligationName string `json:"obligationName"`
	DueDate        string `json:"dueDate"`
	Body           string `json:"body"`
	CreatedAt      string `json:"createdAt"`
}

// Sender is where a live WhatsApp Cloud API client would plug in.
type Sender interface {
	Send(to, body string) (id string, err error)
}

// MockWhatsApp is a zero-key deterministic sender: the message id is an FNV
// hash of recipient + body, so the same input always yields the same id.
type MockWhatsApp struct{}

// Send implements Sender.
func (MockWhatsApp) Send(to, body string) (string, error) {
	h := fnv.New64a()
	h.Write([]byte(to + "|" + body))
	return fmt.Sprintf("wamid.MOCK-%016x", h.Sum64()), nil
}

// MockPhone deterministically derives an Indian mobile number from a seed
// (used when the owner leaves the phone field blank).
func MockPhone(seed string) string {
	h := fnv.New32a()
	h.Write([]byte(seed))
	return fmt.Sprintf("+91-98%08d", h.Sum32()%100000000)
}

func prettyDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02 Jan 2006")
}

func reminderBody(lang string, p rules.Profile, dl rules.Deadline) string {
	date := prettyDate(dl.DueDate)
	if dl.Overdue {
		days := -dl.DaysLeft
		if lang == "hi" {
			return fmt.Sprintf("नमस्ते %s जी, %s के लिए कम्प्लाईकर की सूचना: %s की अंतिम तिथि %s थी और यह %d दिन विलंबित है। संभावित परिणाम: %s। कृपया शीघ्र फाइल करें और FILED लिखकर भेजें। यह शैक्षिक जानकारी है, कानूनी सलाह नहीं — कृपया अपने CA से पुष्टि करें।",
				p.OwnerName, p.BusinessName, dl.ObligationName, date, days, dl.PenaltySummary)
		}
		return fmt.Sprintf("Namaste %s ji, this is ComplyKar for %s. Attention: %s was due on %s and is %d day(s) overdue. Possible consequence: %s. Please file at the earliest and reply FILED. Educational reminder, not legal advice — confirm with your CA.",
			p.OwnerName, p.BusinessName, dl.ObligationName, date, days, dl.PenaltySummary)
	}
	if lang == "hi" {
		return fmt.Sprintf("नमस्ते %s जी, %s के लिए कम्प्लाईकर की ओर से याद दिलाना: %s की अंतिम तिथि %s है — %d दिन शेष। चूकने पर: %s। फाइल करने के बाद FILED लिखकर भेजें। यह शैक्षिक जानकारी है, कानूनी सलाह नहीं — कृपया अपने CA से पुष्टि करें।",
			p.OwnerName, p.BusinessName, dl.ObligationName, date, dl.DaysLeft, dl.PenaltySummary)
	}
	return fmt.Sprintf("Namaste %s ji, this is ComplyKar for %s. Reminder: %s is due on %s — %d day(s) left. Missing it can mean: %s. Reply FILED once done. Educational reminder, not legal advice — confirm with your CA.",
		p.OwnerName, p.BusinessName, dl.ObligationName, date, dl.DaysLeft, dl.PenaltySummary)
}

// BuildReminders creates English + Hindi reminders for every unfiled deadline
// due within ReminderWindowDays (overdue deadlines included).
func BuildReminders(p rules.Profile, dls []rules.Deadline, s Sender) []Message {
	var out []Message
	for _, dl := range dls {
		if dl.Filed || dl.DaysLeft > ReminderWindowDays {
			continue
		}
		kind := "reminder"
		if dl.Overdue {
			kind = "overdue"
		}
		for _, lang := range []string{"en", "hi"} {
			body := reminderBody(lang, p, dl)
			id, err := s.Send(p.Phone, body)
			if err != nil {
				continue
			}
			out = append(out, Message{
				ID:             id,
				To:             p.Phone,
				Channel:        "whatsapp",
				Lang:           lang,
				Kind:           kind,
				ObligationID:   dl.ObligationID,
				ObligationName: dl.ObligationName,
				DueDate:        dl.DueDate,
				Body:           body,
				CreatedAt:      createdAtStamp,
			})
		}
	}
	return out
}

// BuildFiledConfirmation creates English + Hindi confirmations after an
// obligation instance is marked as filed.
func BuildFiledConfirmation(p rules.Profile, obligationID, obligationName, dueDate string, s Sender) []Message {
	date := prettyDate(dueDate)
	bodies := map[string]string{
		"en": fmt.Sprintf("Recorded: %s marked as filed for due date %s. Well done staying compliant, %s ji. — ComplyKar (educational tool, not legal advice)",
			obligationName, date, p.OwnerName),
		"hi": fmt.Sprintf("दर्ज किया गया: %s — अंतिम तिथि %s — फाइल के रूप में चिह्नित। बधाई %s जी, आप समय पर हैं। — कम्प्लाईकर (शैक्षिक टूल, कानूनी सलाह नहीं)",
			obligationName, date, p.OwnerName),
	}
	var out []Message
	for _, lang := range []string{"en", "hi"} {
		body := bodies[lang]
		id, err := s.Send(p.Phone, body)
		if err != nil {
			continue
		}
		out = append(out, Message{
			ID:             id,
			To:             p.Phone,
			Channel:        "whatsapp",
			Lang:           lang,
			Kind:           "confirmation",
			ObligationID:   obligationID,
			ObligationName: obligationName,
			DueDate:        dueDate,
			Body:           body,
			CreatedAt:      createdAtStamp,
		})
	}
	return out
}
