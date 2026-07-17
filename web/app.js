/* ComplyKar frontend — vanilla JS, no dependencies. */
(() => {
  "use strict";

  const state = {
    meta: null,
    profile: null,
    obligations: [],
    calendar: null,
    outbox: [],
    tab: "obligations",
    selectedBand: null,
  };

  const $ = (sel) => document.querySelector(sel);
  const esc = (s) =>
    String(s ?? "").replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  async function api(path, opts) {
    const res = await fetch(path, opts);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || res.statusText);
    return data;
  }

  function toast(msg) {
    const el = $("#toast");
    el.textContent = msg;
    el.hidden = false;
    clearTimeout(el._t);
    el._t = setTimeout(() => (el.hidden = true), 2600);
  }

  function prettyDate(iso) {
    const d = new Date(iso + "T00:00:00");
    return d.toLocaleDateString("en-IN", { day: "2-digit", month: "short", year: "numeric" });
  }

  // ---------- Form setup ----------
  function populateForm() {
    const cat = $("#categorySelect");
    cat.innerHTML = state.meta.categories
      .map((c) => `<option value="${esc(c.value)}">${esc(c.label)}</option>`)
      .join("");
    const st = $("#stateSelect");
    st.innerHTML = state.meta.states
      .map((s) => `<option value="${esc(s)}">${esc(s)}</option>`)
      .join("");
    const bands = $("#bandChips");
    bands.innerHTML = state.meta.turnoverBands
      .map((b) => `<button type="button" class="chip" data-band="${esc(b.value)}">${esc(b.label)}</button>`)
      .join("");
    bands.addEventListener("click", (e) => {
      const btn = e.target.closest("[data-band]");
      if (!btn) return;
      state.selectedBand = btn.dataset.band;
      bands.querySelectorAll(".chip").forEach((c) => c.classList.toggle("selected", c === btn));
    });
    // Default selection: first band.
    state.selectedBand = state.meta.turnoverBands[0].value;
    bands.querySelector(".chip").classList.add("selected");
  }

  function fillForm(p) {
    const f = $("#profileForm");
    f.businessName.value = p.businessName || "";
    f.ownerName.value = p.ownerName || "";
    f.phone.value = p.phone || "";
    f.category.value = p.category;
    f.state.value = p.state;
    f.employees.value = p.employees;
    f.gstRegistered.checked = !!p.gstRegistered;
    f.sellsFood.checked = !!p.sellsFood;
    f.hasPremises.checked = !!p.hasPremises;
    f.interstate.checked = !!p.interstate;
    state.selectedBand = p.turnoverBand;
    document.querySelectorAll("#bandChips .chip").forEach((c) =>
      c.classList.toggle("selected", c.dataset.band === p.turnoverBand));
  }

  const SAMPLE = {
    businessName: "Anna's Kitchen",
    ownerName: "Anjali Rao",
    phone: "+91-9845012345",
    category: "restaurant",
    state: "Karnataka",
    employees: 12,
    turnoverBand: "40L-1.5Cr",
    gstRegistered: true,
    sellsFood: true,
    hasPremises: true,
    interstate: false,
  };

  async function submitProfile(e) {
    e.preventDefault();
    const f = $("#profileForm");
    const body = {
      businessName: f.businessName.value.trim(),
      ownerName: f.ownerName.value.trim(),
      phone: f.phone.value.trim(),
      category: f.category.value,
      state: f.state.value,
      employees: parseInt(f.employees.value, 10) || 0,
      turnoverBand: state.selectedBand,
      gstRegistered: f.gstRegistered.checked,
      sellsFood: f.sellsFood.checked,
      hasPremises: f.hasPremises.checked,
      interstate: f.interstate.checked,
    };
    const btn = $("#submitBtn");
    btn.disabled = true;
    try {
      const res = await api("/api/v1/profile", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      state.profile = res.profile;
      toast(`Found ${res.obligations.length} obligations · ${res.reminders} reminders queued`);
      await refreshAll();
      showDashboard();
    } catch (err) {
      toast("Error: " + err.message);
    } finally {
      btn.disabled = false;
    }
  }

  // ---------- Data refresh ----------
  async function refreshAll() {
    const [obl, cal, out] = await Promise.all([
      api("/api/v1/obligations"),
      api("/api/v1/calendar"),
      api("/api/v1/outbox"),
    ]);
    state.obligations = obl.obligations || [];
    state.calendar = cal;
    state.outbox = out.messages || [];
    renderAll();
  }

  function showDashboard() {
    $("#onboarding").hidden = true;
    $("#dashboard").hidden = false;
    window.scrollTo({ top: 0 });
  }

  function showForm() {
    $("#dashboard").hidden = true;
    $("#onboarding").hidden = false;
    $("#formTitle").textContent = "Edit your business profile";
    if (state.profile) fillForm(state.profile);
    window.scrollTo({ top: 0 });
  }

  // ---------- Renderers ----------
  function renderAll() {
    renderSummary();
    renderObligations();
    renderCalendar();
    renderOutbox();
    $("#countObligations").textContent = state.obligations.length;
    $("#countCalendar").textContent = state.calendar?.summary?.total ?? 0;
    $("#countOutbox").textContent = state.outbox.length;
  }

  function renderSummary() {
    const p = state.profile;
    if (!p) return;
    const catLabel = state.meta.categories.find((c) => c.value === p.category)?.label || p.category;
    const bandLabel = state.meta.turnoverBands.find((b) => b.value === p.turnoverBand)?.label || p.turnoverBand;
    const flag = (on, label) => `<span class="pill ${on ? "on" : "off"}">${esc(label)}</span>`;
    $("#profileSummary").innerHTML = `
      <div class="summary-left">
        <h2>${esc(p.businessName)}</h2>
        <div class="summary-owner">${esc(p.ownerName)} · ${esc(p.phone)}</div>
        <div class="summary-chips">
          <span class="pill">${esc(catLabel)}</span>
          <span class="pill">${esc(p.state)}</span>
          <span class="pill">${p.employees} employees</span>
          <span class="pill">${esc(bandLabel)}</span>
          ${flag(p.gstRegistered, "GST registered")}
          ${flag(p.sellsFood, "Sells food")}
          ${flag(p.hasPremises, "Premises")}
          ${flag(p.interstate, "Interstate")}
        </div>
      </div>
      <button type="button" class="btn ghost small" id="editProfileBtn">Edit profile</button>`;
    $("#editProfileBtn").addEventListener("click", showForm);
  }

  function renderObligations() {
    const el = $("#panel-obligations");
    if (!state.obligations.length) {
      el.innerHTML = `<div class="empty"><span class="big">·</span>No obligations yet — fill in your profile first.</div>`;
      return;
    }
    el.innerHTML = `<div class="obl-grid">` + state.obligations.map((o) => `
      <article class="card obl-card">
        <div class="obl-head">
          <div>
            <h3>${esc(o.name)}</h3>
            <div class="obl-auth">${esc(o.authority)}</div>
          </div>
          <span class="badge ${esc(o.frequency)}">${esc(o.frequency)}</span>
        </div>
        <p class="obl-why">${esc(o.whyItApplies)}</p>
        <div class="due-chips">
          ${(o.nextDueDates || []).map((d) => `<span class="due-chip">${prettyDate(d)}</span>`).join("")}
        </div>
        <div class="obl-penalty"><strong>If missed:</strong> ${esc(o.penaltySummary)}</div>
        <details class="obl-docs">
          <summary>Documents needed (${(o.docsNeeded || []).length})</summary>
          <ul>${(o.docsNeeded || []).map((d) => `<li>${esc(d)}</li>`).join("")}</ul>
        </details>
      </article>`).join("") + `</div>
      <p class="fine-disclaimer" style="margin-top:1rem">${esc(state.meta.disclaimer)}</p>`;
  }

  function statusPill(d) {
    if (d.filed) return `<span class="status-pill done">Filed</span>`;
    if (d.overdue) return `<span class="status-pill late">${-d.daysLeft} days overdue</span>`;
    if (d.daysLeft === 0) return `<span class="status-pill soon">Due today</span>`;
    if (d.daysLeft <= 14) return `<span class="status-pill soon">${d.daysLeft} days left</span>`;
    return `<span class="status-pill ok">${d.daysLeft} days left</span>`;
  }

  function renderCalendar() {
    const el = $("#panel-calendar");
    const cal = state.calendar;
    if (!cal || !cal.profileSet || !(cal.months || []).length) {
      el.innerHTML = `<div class="empty"><span class="big">·</span>Nothing on the calendar yet.</div>`;
      return;
    }
    const s = cal.summary || {};
    let html = `
      <div class="cal-summary">
        <div class="stat"><b>${s.total || 0}</b>deadlines in 90 days</div>
        <div class="stat ${s.overdue ? "alert" : ""}"><b>${s.overdue || 0}</b>overdue</div>
        <div class="stat"><b>${s.dueSoon || 0}</b>due within 14 days</div>
        <div class="stat good"><b>${s.filed || 0}</b>marked filed</div>
      </div>`;
    for (const m of cal.months) {
      html += `<section class="cal-month"><h3>${esc(m.label)}</h3><div class="cal-list">`;
      for (const d of m.deadlines) {
        const day = d.dueDate.slice(8, 10);
        const wd = new Date(d.dueDate + "T00:00:00").toLocaleDateString("en-IN", { weekday: "short" });
        html += `
          <div class="cal-row ${d.overdue ? "overdue" : ""} ${d.filed ? "filed" : ""}">
            <div class="date-box"><b>${day}</b><span>${esc(wd)}</span></div>
            <div class="cal-main">
              <div class="name">${d.filed ? `<span class="tick">✓</span> ` : ""}${esc(d.obligationName)}</div>
              <div class="sub">${esc(d.frequency)} · due ${prettyDate(d.dueDate)}${d.filed ? ` · filed ${new Date(d.filedAt).toLocaleDateString("en-IN")}` : ""}</div>
            </div>
            ${statusPill(d)}
            ${d.filed ? "" : `<button type="button" class="btn ghost small" data-file-id="${esc(d.obligationId)}" data-file-date="${esc(d.dueDate)}">Mark filed</button>`}
          </div>`;
      }
      html += `</div></section>`;
    }
    const hist = cal.history || [];
    html += `<section class="history"><h3>Filing history</h3>`;
    if (!hist.length) {
      html += `<div class="empty">Nothing filed yet. Mark a deadline as filed and it will be remembered here.</div>`;
    } else {
      html += `<div class="history-list">` + hist.map((h) => `
        <div class="history-row">
          <span class="tick">✓</span>
          <strong>${esc(h.obligationName || h.obligationId)}</strong>
          <span>due ${prettyDate(h.dueDate)}</span>
          <span class="when">filed ${new Date(h.filedAt).toLocaleString("en-IN")}</span>
        </div>`).join("") + `</div>`;
    }
    html += `</section>`;
    el.innerHTML = html;

    el.querySelectorAll("[data-file-id]").forEach((btn) =>
      btn.addEventListener("click", () => markFiled(btn.dataset.fileId, btn.dataset.fileDate, btn)));
  }

  async function markFiled(id, dueDate, btn) {
    btn.disabled = true;
    try {
      await api(`/api/v1/obligations/${encodeURIComponent(id)}/filed`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ dueDate }),
      });
      toast("Marked as filed — confirmation sent to outbox");
      await refreshAll();
    } catch (err) {
      toast("Error: " + err.message);
      btn.disabled = false;
    }
  }

  function renderOutbox() {
    const el = $("#panel-outbox");
    if (!state.outbox.length) {
      el.innerHTML = `<div class="empty"><span class="big">·</span>No reminders yet. Deadlines within 14 days generate WhatsApp reminders in English and Hindi.</div>`;
      return;
    }
    el.innerHTML = `
      <div class="outbox-note">
        Provider: <strong>mock</strong> (deterministic, zero keys). Set <code>WHATSAPP_PROVIDER=live</code>
        with <code>WHATSAPP_API_TOKEN</code> + <code>WHATSAPP_PHONE_ID</code> to send real messages — see README.
      </div>
      <div class="chat">` + state.outbox.map((m) => `
        <div class="bubble ${m.lang === "hi" ? "hi" : ""}">
          <div class="meta">
            <span class="lang-tag">${m.lang === "hi" ? "हिंदी" : "EN"}</span>
            <span class="kind-tag ${esc(m.kind)}">${esc(m.kind)}</span>
            <span class="kind-tag">to ${esc(m.to)}</span>
          </div>
          ${esc(m.body)}
          <span class="stamp">${esc(m.id)}</span>
        </div>`).join("") + `</div>`;
  }

  // ---------- Tabs ----------
  function setupTabs() {
    document.querySelectorAll(".tab").forEach((t) =>
      t.addEventListener("click", () => {
        state.tab = t.dataset.tab;
        document.querySelectorAll(".tab").forEach((x) => x.classList.toggle("active", x === t));
        ["obligations", "calendar", "outbox"].forEach((name) => {
          $(`#panel-${name}`).hidden = name !== state.tab;
        });
      }));
  }

  // ---------- Boot ----------
  async function init() {
    state.meta = await api("/api/v1/meta");
    populateForm();
    setupTabs();
    $("#profileForm").addEventListener("submit", submitProfile);
    $("#sampleBtn").addEventListener("click", () => fillForm(SAMPLE));

    const prof = await api("/api/v1/profile");
    if (prof.profileSet) {
      state.profile = prof.profile;
      fillForm(prof.profile);
      await refreshAll();
      showDashboard();
    }
  }

  init().catch((err) => toast("Failed to load: " + err.message));
})();
