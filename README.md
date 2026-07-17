# ComplyKar — MSME Compliance Copilot

> **Educational tool, not legal advice — confirm with your CA.**

ComplyKar tells an Indian mom-and-pop business exactly which compliance
obligations apply to them (GST, FSSAI, Udyam, Shops & Establishments,
professional tax, EPF/ESI, TDS, drug licence, fire NOC and more), when each is
due, and reminds them on WhatsApp in **English and Hindi**.

**Who pays:** kirana stores, salons, restaurants, pharmacies, coaching
institutes, boutiques, gyms and small manufacturers — **₹499/mo** per business.
CA firms can also white-label it for their MSME clients.

The heart of the product is a **data-driven rules engine**: compliance rules
are declarative Go structs (typed condition clauses + due-date specs) and a
small, well-tested evaluator turns a business profile into obligations with
plain-language "why this applies to you" text and internally consistent due
dates (GSTR-1 on the 11th, GSTR-3B on the 20th, QRMP quarterly cadence, and so
on) computed from a fixed demo anchor date of **2026-07-17**.

## Quickstart

```bash
make run          # serves UI + API on http://localhost:8103
make test         # go test ./...
make build        # builds bin/complykar
```

Open http://localhost:8103, click **"Try a sample: Anna's Kitchen, Bengaluru"**
and submit — you get 9 obligations, a 90-day deadline calendar with overdue
highlighting and mark-as-filed, and a bilingual WhatsApp outbox.

Everything is a single Go binary: stdlib only, UI embedded via `go:embed`,
state persisted as a JSON snapshot in `./data/store.json`.

## API summary

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health` | Liveness + provider modes |
| GET | `/api/v1/meta` | Form options (categories, states, turnover bands), anchor date, pricing |
| POST | `/api/v1/profile` | Save business profile; returns evaluated obligations and queues reminders |
| GET | `/api/v1/profile` | Current profile |
| GET | `/api/v1/obligations` | Applicable obligations with why/authority/frequency/next dues/penalty/docs |
| GET | `/api/v1/calendar` | Next-90-day deadlines grouped by month, overdue flags, filing history |
| POST | `/api/v1/obligations/{id}/filed` | Mark a deadline instance filed (`{"dueDate":"2026-07-22"}`, optional) |
| GET | `/api/v1/outbox` | Mock WhatsApp outbox (English + Hindi reminders and confirmations) |

Example:

```bash
curl -s -X POST localhost:8103/api/v1/profile -d '{
  "businessName":"Anna'\''s Kitchen","ownerName":"Anjali Rao","phone":"+91-9845012345",
  "category":"restaurant","state":"Karnataka","employees":12,"turnoverBand":"40L-1.5Cr",
  "gstRegistered":true,"sellsFood":true,"hasPremises":true,"interstate":false}'
```

## Rules covered

GST registration thresholds (₹40 L goods / ₹20 L services, interstate ⇒
mandatory), GSTR-1 + GSTR-3B monthly vs QRMP (turnover ≤ ₹5 Cr), GSTR-9
annual, Udyam registration, Shops & Establishments (state-specific notes for
10 states), FSSAI basic registration vs state licence (food sellers), employer
professional tax (state applicability list), municipal trade licence, EPF
(≥ 20 employees), ESI (≥ 10 employees), TDS heuristic (turnover > ₹1.5 Cr),
retail drug licence (pharmacy), fire NOC heuristic (restaurant/gym with
premises). Dates are internally consistent, not statutory-perfect — this is an
educational tool.

## Upgrade to live (zero keys today)

All external integrations are deterministic mocks seeded from an FNV hash of
user input — the same input always produces the same output. A live WhatsApp
integration would implement the `notify.Sender` interface.

| Env var | Default | What it does |
|---|---|---|
| `PORT` | `8103` | HTTP port for UI + API |
| `WHATSAPP_PROVIDER` | `mock` | `mock` writes to the in-app Outbox with `wamid.MOCK-<fnv>` ids; `live` would send via WhatsApp Cloud API (not built — falls back to mock with a log line) |
| `WHATSAPP_API_TOKEN` | unset | Meta Cloud API token used by a live sender |
| `WHATSAPP_PHONE_ID` | unset | Sender phone-number id used by a live sender |

## Layout

```
cmd/server/       HTTP server (routes, handlers)
internal/rules/   declarative rule catalog + evaluator + due-date math (core, tested)
internal/store/   mutex-guarded in-memory state + JSON snapshot persistence
internal/notify/  bilingual reminder generation + mock WhatsApp sender
internal/money/   formatINR (₹1.2 Cr / ₹36.5 L / ₹12,500)
web/              embedded vanilla HTML/CSS/JS UI
```
