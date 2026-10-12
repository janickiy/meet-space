/* Local prototype of a personal call. No media devices or network calls are used. */
(() => {
  const M = window.MS, E = (value) => M.escape(value), I = (name) => M.icon(name);
  let answerTimer, clockTimer;
  const current = () => M.state.personalCall || null;
  const contact = () => M.chatContact(current()?.contactId) || M.chatContact("max");
  const elapsed = () => current()?.startedAt ? Math.max(0, Math.floor((Date.now() - current().startedAt) / 1000)) : 0;
  const duration = () => `${String(Math.floor(elapsed() / 60)).padStart(2, "0")}:${String(elapsed() % 60).padStart(2, "0")}`;
  function stopTimers() { clearTimeout(answerTimer); clearInterval(clockTimer); }
  function runTimers() {
    stopTimers();
    const call = current();
    if (!call) return;
    if (call.phase === "calling") answerTimer = setTimeout(() => {
      if (current()?.id !== call.id || current().phase !== "calling") return;
      call.phase = "connected";
      call.startedAt = Date.now();
      M.render();
      runTimers();
    }, Math.max(0, call.calledAt + 4500 - Date.now()));
    clockTimer = setInterval(() => {
      if (!current()) return stopTimers();
      document.querySelectorAll("[data-personal-elapsed]").forEach((node) => { node.textContent = duration(); });
    }, 1000);
  }
  function createCall(person) {
    M.state.personalCall = { id: crypto.randomUUID(), contactId: person.id, phase: "calling", calledAt: Date.now(), startedAt: 0, mic: true, camera: false, sharing: false };
    M.state.chat = person.id;
    runTimers();
  }
  M.personalCallSnapshot = () => current() ? { ...current() } : null;
  M.setPersonalCallSnapshot = (value) => {
    if (value !== null && (!value || typeof value !== "object" || typeof value.id !== "string" || !["calling", "connected"].includes(value.phase) || M.chatContact(value.contactId)?.kind !== "direct" || !Number.isFinite(value.calledAt) || !Number.isFinite(value.startedAt))) return false;
    const next = value === null ? null : { id: value.id, contactId: value.contactId, phase: value.phase, calledAt: value.calledAt, startedAt: value.startedAt, mic: !!value.mic, camera: !!value.camera, sharing: !!value.sharing };
    if (JSON.stringify(next) === JSON.stringify(current())) return false;
    M.state.personalCall = next;
    runTimers();
    return true;
  };
  M.renderPersonalCallBanner = (person) => current()?.contactId === person.id ? `<button class="personal-call-banner" data-action="resumePersonalCall">${I("Phone")}<span><strong>Личный звонок</strong><small>${current().phase === "calling" ? "Вы звоните…" : `<time data-personal-elapsed>${duration()}</time>`}</small></span><b>Вернуться ${I("ArrowUpRight")}</b></button>` : "";
  const control = (label, icon, action, active, extra = "") => `<button class="personal-control ${active ? "is-on" : ""} ${extra}" data-action="${action}" aria-label="${label}" ${typeof active === "boolean" ? `aria-pressed="${active}"` : ""}><span>${I(icon)}</span><small>${label}</small></button>`;

  M.screens["personal-call"] = {
    title: "Личный звонок", group: "Встреча",
    render: () => {
      if (!current()) createCall(M.chatContact(M.state.chat)?.kind === "direct" ? M.chatContact(M.state.chat) : M.chatContact("max"));
      const call = current(), person = contact(), connected = call.phase === "connected";
      return `<main class="personal-call-screen">
        <div class="statusbar"><span>9:41</span><div class="status-indicators">${I("Signal")}${I("Wifi")}<span class="battery" aria-label="Заряд батареи"></span></div></div>
        <header class="personal-call-header"><button class="icon-button" data-action="minimizePersonalCall" aria-label="Свернуть звонок и открыть переписку">${I("ChevronDown")}</button><div><h1>Личный звонок</h1><p>${connected ? `${I("Lock")} Соединение установлено` : "Соединяем вас с собеседником"}</p></div><span class="personal-call-time" ${connected ? "" : "hidden"}>${I("Phone")}<time data-personal-elapsed>${duration()}</time></span></header>
        <section class="personal-call-stage ${connected ? "connected" : "calling"}">
          <div class="personal-self ${call.camera ? "camera-on" : ""}"><span class="personal-self-avatar">${E(M.profileInitials?.() || "АС")}</span><span class="personal-self-label">Вы ${I(call.mic ? "Mic" : "MicOff")}</span><span class="personal-camera-state">${I(call.camera ? "Video" : "VideoOff")}</span></div>
          <div class="personal-remote"><div class="personal-remote-avatar">${E(person.initials)}</div><h2>${E(person.name)}</h2><p class="personal-call-status" role="status">${connected ? "На связи" : "Вы звоните…"}</p>${connected ? `<span class="personal-camera-note">${I("VideoOff")} Камера собеседника выключена</span>` : `<span class="personal-ringing-dots" aria-hidden="true"><i></i><i></i><i></i></span>`}</div>
          ${call.sharing ? `<div class="personal-sharing">${I("MonitorUp")}<span>Вы демонстрируете экран</span><button data-action="personalShare">Остановить</button></div>` : ""}
        </section>
        <footer class="personal-call-footer"><p class="personal-call-hint">${connected ? "Разговор один на один" : "Собеседник услышит вас после ответа"}</p>
          <div class="personal-call-controls">${control(call.mic ? "Микрофон" : "Без звука", call.mic ? "Mic" : "MicOff", "personalMic", call.mic)}${control("Камера", call.camera ? "Video" : "VideoOff", "personalCamera", call.camera)}${control("Экран", "MonitorUp", "personalShare", call.sharing)}${control("Завершить", "PhoneOff", "endPersonalCall", undefined, "end-call")}</div>
          <span class="personal-home-indicator" aria-hidden="true"></span>
        </footer>
      </main>`;
    },
    bind: runTimers,
  };
  M.actions.startPersonalCall = (button) => {
    const person = M.chatContact(button.dataset.contact);
    if (person?.kind !== "direct") return;
    if (current() && current().contactId !== person.id) {
      M.modal("Уже идёт личный звонок", `<div class="stack"><p>Вы разговариваете с ${E(contact().name)}.</p><button class="button primary" data-action="resumePersonalCall">Вернуться к звонку</button><button class="button secondary" data-close>Остаться в переписке</button></div>`);
      return;
    }
    if (!current()) createCall(person);
    M.go("personal-call");
  };
  M.actions.resumePersonalCall = () => { if (current()) { M.state.chat = current().contactId; M.go("personal-call"); } };
  M.actions.minimizePersonalCall = () => { if (current()) { M.state.chat = current().contactId; M.go("thread"); } };
  M.actions.endPersonalCall = () => {
    if (!current()) return;
    const call = current(), time = duration();
    M.state.chat = call.contactId;
    M.state.personalCall = null;
    stopTimers();
    M.go("thread");
    M.toast(call.phase === "connected" ? `Звонок завершён · ${time}` : "Вызов отменён");
    document.querySelector(".personal-call-trigger")?.focus();
  };
  for (const [action, setting] of [["personalMic", "mic"], ["personalCamera", "camera"]]) {
    M.actions[action] = () => {
      if (!current()) return;
      current()[setting] = !current()[setting];
      M.render();
      document.querySelector(`[data-action="${action}"]`)?.focus();
    };
  }
  M.actions.personalShare = () => {
    if (!current()) return;
    if (current().sharing) { current().sharing = false; M.render(); return; }
    M.modal("Демонстрация экрана", `<div class="stack"><p>Собеседник сможет видеть содержимое вашего экрана.</p><button class="button primary" data-action="personalStartShare">${I("MonitorUp")}Начать демонстрацию</button><button class="button secondary" data-close>Отмена</button></div>`);
  };
  M.actions.personalStartShare = () => { if (current()) { current().sharing = true; M.closeModal(); M.render(); } };
})();
