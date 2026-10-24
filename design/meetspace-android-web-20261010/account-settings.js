/* Прототип настроек аккаунта.  Настройки и черновики остаются в памяти этого предварительного просмотра. */
(() => {
  const M = window.MS, I = (name) => M.icon(name), E = (text) => M.escape(text);
  const sections = [
    ["profile", "UserRound", "Профиль", "Имя и данные аккаунта"],
    ["audio", "Mic", "Аудио", "Микрофон, динамик и звук"],
    ["video", "Video", "Видео", "Камера и участники"],
    ["notifications", "Bell", "Уведомления", "События и каналы доставки"],
    ["appearance", "Palette", "Оформление", "Тема и размер текста"],
  ];
  const validSection = (id) => id === "connection" || sections.some(([key]) => key === id);
  const name = () => M.state.profileName || "Александр Соколов";
  const email = () => M.state.profileEmail || "alex@example.org";
  const notifications = { invitations: true, reminders: true, recording: true, summary: true, sound: true, email: false };
  const s = M.state.account = {
    section: "profile", open: false, nameDraft: name(), profileSaved: false,
    mic: "default", speaker: "default", ringtone: "soft", autoMute: false, noise: true,
    camera: "front", cameraOff: true, selfVideo: true, hideVideo: false, preview: false,
    notifications: { ...notifications }, notificationDraft: { ...notifications }, notificationSaved: false,
    textSize: 100, connectionDraft: null, connectionOriginal: null, connectionSaved: false,
  };
  M.profileInitials = () => name().trim().split(/\s+/).slice(0, 2).map((part) => Array.from(part)[0]).join("").toUpperCase();
  M.accountSectionRoute = () => M.state.screen === "settings" && s.open ? s.section : null;
  M.setAccountSectionRoute = (section) => {
    const open = validSection(section), next = open ? section : "profile";
    const changed = s.open !== open || s.section !== next;
    s.open = open; s.section = next;
    return changed;
  };
  M.setAccountSectionRoute(new URLSearchParams(location.search).get("section"));
  M.applyJoinPreferences = () => { M.state.mic = !s.autoMute; M.state.camera = !s.cameraOff; };
  const portrait = (large = false) => `<span class="account-avatar ${large ? "large" : ""}" aria-hidden="true">${E(M.profileInitials())}</span>`;
  const heading = (title, subtitle, avatar = false) => `<div class="account-panel-heading"><div><h2 id="account-heading" tabindex="-1">${title}</h2><p>${subtitle}</p></div>${avatar ? portrait(true) : ""}</div>`;
  const note = (text) => `<p class="account-note">${I("Info")}<span>${text}</span></p>`;
  const switchRow = (key, title, hint = "", source = s, disabled = false) => `<label class="account-switch-row ${disabled ? "is-disabled" : ""}"><span><strong>${title}</strong>${hint ? `<small>${hint}</small>` : ""}</span><input class="toggle" type="checkbox" role="switch" data-pref="${key}" aria-label="${title}" ${source[key] ? "checked" : ""} ${disabled ? "disabled" : ""}/></label>`;
  const select = (key, title, choices, hint = "") => `<label class="account-field"><span>${title}</span><select data-pref="${key}" aria-label="${title}">${choices.map(([value, label]) => `<option value="${value}" ${String(s[key]) === value ? "selected" : ""}>${label}</option>`).join("")}</select>${hint ? `<small>${hint}</small>` : ""}</label>`;
  const test = (kind, label) => `<div class="account-device-test"><div class="account-meter" aria-hidden="true">${Array.from({length: 20}, (_, i) => `<i style="--bar:${i % 7}"></i>`).join("")}</div><button class="account-text-button" data-action="accountTest" data-test="${kind}" aria-pressed="false">${I(kind === "mic" ? "Mic" : "Volume2")}<span>${label}</span></button><span class="account-test-status" role="status"></span></div>`;
  const notificationDirty = () => Object.keys(s.notifications).some((key) => s.notifications[key] !== s.notificationDraft[key]);
  const profile = () => `${heading("Ваш профиль", "Как вас видят участники встречи.", true)}
    <form id="account-profile-form" class="account-form" novalidate>
      <label class="account-field"><span>Имя для встреч</span><input id="account-name" name="name" value="${E(s.nameDraft)}" minlength="1" maxlength="100" required autocomplete="name" aria-describedby="account-name-hint account-name-error"/><small id="account-name-hint">От 1 до 100 символов.</small><small id="account-name-error" class="account-error" hidden>Введите имя от 1 до 100 символов.</small></label>
      <label class="account-field"><span>Email</span><input type="email" value="${E(email())}" readonly aria-describedby="account-email-hint"/><small id="account-email-hint">Адрес используется для входа. Изменение email пока недоступно.</small></label>
      <div class="account-save-row"><button id="account-save-name" class="button primary" ${!s.nameDraft.trim() || s.nameDraft.trim() === name() ? "disabled" : ""}>Сохранить имя</button><span class="account-saved" role="status">${s.profileSaved ? `${I("Check")}Имя сохранено` : ""}</span></div>
    </form>
    <div class="account-registration">${I("CalendarDays")}<span>Регистрация: 3 окт. 2026 г., 19:17</span></div>`;
  const audio = () => `${heading("Аудио", "Подготовьте звук к следующей встрече.")}
    <section class="account-section">${select("mic", "Микрофон", [["default", "По умолчанию"], ["device", "Микрофон устройства"], ["headset", "Микрофон гарнитуры"]])}${test("mic", "Проверить микрофон")}${switchRow("autoMute", "Подключаться с выключенным микрофоном")}${switchRow("noise", "Шумоподавление", "Меньше фонового шума во время разговора")}</section>
    <section class="account-section">${select("speaker", "Динамик", [["default", "По умолчанию"], ["device", "Динамик устройства"], ["headset", "Наушники"]])}${test("speaker", "Проверить динамик")}</section>
    <section class="account-section">${select("ringtone", "Звук приглашения", [["soft", "Мягкий сигнал"], ["classic", "Классический"], ["short", "Короткий"]])}${test("ringtone", "Проверить звук приглашения")}</section>
    ${note("Выбор сохраняется на время просмотра. Проверка звука демонстрационная — устройства не подключаются.")}`;
  const video = () => `${heading("Видео", "Предпросмотр доступен только вам.")}
    <div class="account-camera ${s.preview ? "preview-on" : ""}"><span class="account-camera-label">${I("LockKeyhole")} Только вы</span>${portrait(true)}<strong>${s.preview ? "Предпросмотр камеры" : "Камера выключена"}</strong><span>Демонстрация · без доступа к камере</span><button class="button secondary" data-action="accountCamera">${I(s.preview ? "VideoOff" : "Video")}${s.preview ? "Выключить предпросмотр" : "Показать предпросмотр"}</button></div>
    <section class="account-section">${select("camera", "Камера", [["front", "Фронтальная камера"], ["back", "Основная камера"]])}</section>
    <section class="account-section">${switchRow("cameraOff", "Подключаться с выключенной камерой")}${switchRow("selfVideo", "Видеть себя на встрече", "Ваше видео или аватар среди участников")}${switchRow("hideVideo", "Скрыть видео участников", "Снижает нагрузку на сеть. Звук остаётся включённым.")}</section>
    ${note("Настройки сохраняются на время просмотра макета.")}`;
  const eventRow = (key, label) => `<label class="account-check-row"><input type="checkbox" data-notification="${key}" ${s.notificationDraft[key] ? "checked" : ""}/><span>${label}</span></label>`;
  const notificationPanel = () => `${heading("Уведомления", "Выберите события и каналы доставки.")}
    <form id="account-notification-form">
      <section class="account-section"><h3>События</h3>${eventRow("invitations", "Приглашения и изменения встреч")}${eventRow("reminders", "Напоминания о встречах")}${eventRow("recording", "Готовность записи")}${eventRow("summary", "Расшифровка и итоги встречи")}</section>
      <section class="account-section"><h3>Звук уведомлений</h3>${eventRow("sound", "Звуковой сигнал")}<button type="button" class="account-text-button" data-action="accountTest" data-test="notification" aria-pressed="false">${I("Volume2")}<span>Проверить звук</span></button><span class="account-test-status" role="status"></span></section>
      <section class="account-section"><h3>Каналы доставки</h3>${switchRow("email", "Email", "Отправка почты настроена на сервере", s.notificationDraft)}${switchRow("push", "Push-уведомления", "Не настроены на этом сервере", {}, true)}</section>
      <div class="account-save-row"><button id="account-save-notifications" class="button primary" ${notificationDirty() ? "" : "disabled"}>Сохранить настройки</button><span class="account-saved" role="status">${s.notificationSaved ? `${I("Check")}Настройки сохранены` : ""}</span></div>
    </form>${note("В демо уведомления не отправляются. Настройки действуют до обновления страницы.")}`;
  const appearance = () => `${heading("Оформление", "Настройте MeetSpace под себя.")}
    <fieldset class="account-theme-fieldset"><legend>Тема</legend><div class="account-theme-grid">${[["light", "Светлая", "Sun"], ["dark", "Тёмная", "Moon"]].map(([key, label, icon]) => `<label class="account-theme-choice ${M.state.theme === key ? "selected" : ""}"><input type="radio" name="account-theme" value="${key}" aria-label="${label} тема" ${M.state.theme === key ? "checked" : ""}/><span class="account-theme-preview ${key}" aria-hidden="true"><i></i><span><b></b><em></em><em></em></span></span><span class="account-theme-caption">${I(icon)}<strong>${label}</strong><span class="account-radio-dot"></span></span></label>`).join("")}</div></fieldset>
    <section class="account-section"><h3>Внешний вид</h3>${select("textSize", "Размер текста", [75, 90, 100, 110, 125, 150, 200].map((n) => [String(n), `${n}%${n === 100 ? " · по умолчанию" : ""}`]))}<div class="account-type-sample"><span class="account-sample-eyebrow">ПРЕДПРОСМОТР</span><h3>Удобный размер для важных разговоров</h3><p>Все ваши встречи и сообщения — в одном месте.</p></div></section>
    ${note("Тема и размер текста в настройках применяются сразу и сохраняются на время просмотра.")}`;
  const serverAddress = () => M.state.onboarding?.server || "https://meet.example.org";
  const connectionError = (value) => {
    const clean = value.trim();
    if (!clean) return "Укажите адрес сервера.";
    if (/[\s,;]/.test(clean) || (clean.match(/https?:\/\//gi) || []).length > 1)
      return "Укажите только один адрес сервера.";
    try {
      const url = new URL(clean);
      if (url.protocol !== "https:" || !url.hostname || url.username || url.password || url.search || url.hash)
        return "Введите один адрес HTTPS без логина, пароля и параметров.";
    } catch { return "Введите адрес в формате https://meet.example.org"; }
    return "";
  };
  const normalizeConnection = (value) => new URL(value.trim()).href.replace(/\/$/, "");
  const connection = () => {
    if (s.connectionDraft === null || s.connectionOriginal !== serverAddress()) {
      s.connectionDraft = serverAddress(); s.connectionOriginal = serverAddress(); s.connectionSaved = false;
    }
    const error = connectionError(s.connectionDraft);
    return `${heading("Подключение", "Адрес сервера вашего MeetSpace.")}
      <form id="account-connection-form" class="account-form" novalidate>
        <label class="account-field"><span>Адрес сервера</span><input id="account-server-address" name="server" type="url" inputmode="url" autocomplete="url" autocapitalize="none" spellcheck="false" required maxlength="2048" placeholder="https://meet.example.org" value="${E(s.connectionDraft)}" aria-describedby="account-connection-error" aria-invalid="${Boolean(error)}"/><small id="account-connection-error" class="account-error" ${error ? "" : "hidden"}>${E(error)}</small></label>
        <div class="account-save-row"><button id="account-save-connection" class="button primary" ${error || normalizeConnection(s.connectionDraft) === normalizeConnection(serverAddress()) ? "disabled" : ""}>Сохранить адрес</button><span class="account-saved" role="status">${s.connectionSaved ? `${I("Check")}Адрес сохранён` : ""}</span></div>
      </form>${note("В макете сохраняется только адрес. Подключение к серверу не выполняется.")}`;
  };
  const panels = { profile, audio, video, notifications: notificationPanel, appearance, connection };
  const menu = () => `<aside class="account-sidebar" aria-label="Разделы настроек"><div class="account-identity">${portrait()}<div><strong>${E(name())}</strong><span>${E(email())}</span></div></div><p class="account-menu-label">НАСТРОЙКИ АККАУНТА</p><nav class="account-menu" aria-label="Настройки аккаунта">${sections.map(([id, icon, label, hint]) => `<button class="account-menu-item ${s.section === id ? "active" : ""}" data-action="accountSection" data-section="${id}" aria-label="${label}" ${s.section === id ? 'aria-current="page"' : ""}><span class="account-menu-icon">${I(icon)}</span><span><strong>${label}</strong><small>${hint}</small></span>${I("ChevronRight")}</button>`).join("")}</nav><div class="account-sidebar-footer"><button class="account-server ${s.section === "connection" ? "active" : ""}" data-action="accountSection" data-section="connection" aria-label="Подключение" ${s.section === "connection" ? 'aria-current="page"' : ""}>${I("Server")}<span><strong>Подключение</strong><small>${E(new URL(M.state.onboarding?.server || "https://meet.example.org").host)}</small></span>${I("ChevronRight")}</button><button class="account-logout" data-action="logout">${I("LogOut")}Выйти из аккаунта</button><p>MeetSpace · Демо</p></div></aside>`;
  M.screens.settings = {
    title: "Настройки аккаунта", group: "Рабочее пространство",
    render: () => M.shell(`<div class="account-layout ${s.open ? "detail-open" : ""}" style="--account-scale:${s.textSize / 100}">${menu()}<section class="account-panel" aria-labelledby="account-heading"><button class="account-back" data-action="accountBack">${I("ArrowLeft")}Все настройки</button>${panels[s.section]()}</section></div>`, "settings", "account-content"),
    bind: () => {
      const input = document.getElementById("account-name"), form = document.getElementById("account-profile-form");
      const validateName = () => {
        const clean = s.nameDraft.trim(), valid = clean.length > 0 && clean.length <= 100 && !/[\u0000-\u001f\u007f]/.test(clean);
        document.getElementById("account-save-name").disabled = !valid || clean === name();
        input.setAttribute("aria-invalid", String(!valid));
        document.getElementById("account-name-error").hidden = valid;
        return valid;
      };
      input?.addEventListener("input", () => {
        s.nameDraft = input.value; s.profileSaved = false;
        form.querySelector(".account-saved").textContent = "";
        validateName();
      });
      form?.addEventListener("submit", (e) => {
        e.preventDefault(); if (!validateName()) return;
        M.state.profileName = s.nameDraft.trim(); s.nameDraft = name(); s.profileSaved = true;
        M.state.participantName = name();
        M.render(); document.getElementById("account-name")?.focus();
      });
      const connectionForm = document.getElementById("account-connection-form");
      if (connectionForm) {
        const address = document.getElementById("account-server-address");
        const validateConnection = () => {
          const error = connectionError(s.connectionDraft);
          const hint = document.getElementById("account-connection-error");
          hint.textContent = error; hint.hidden = !error;
          address.setAttribute("aria-invalid", String(Boolean(error)));
          document.getElementById("account-save-connection").disabled = Boolean(error) || normalizeConnection(s.connectionDraft) === normalizeConnection(serverAddress());
          return !error;
        };
        address.addEventListener("input", () => {
          s.connectionDraft = address.value; s.connectionSaved = false;
          connectionForm.querySelector(".account-saved").textContent = "";
          validateConnection();
        });
        connectionForm.addEventListener("submit", (e) => {
          e.preventDefault(); if (!validateConnection()) return;
          const value = normalizeConnection(s.connectionDraft);
          M.getOnboardingState().server = value;
          s.connectionDraft = value; s.connectionOriginal = value; s.connectionSaved = true;
          M.render(); document.getElementById("account-server-address")?.focus();
        });
      }
      document.querySelectorAll("[data-pref]").forEach((input) => input.addEventListener("change", () => {
        const key = input.dataset.pref, value = input.type === "checkbox" ? input.checked : input.value;
        if (s.section === "notifications") { s.notificationDraft[key] = value; updateNotifications(); }
        else {
          s[key] = key === "textSize" ? Number(value) : value;
          if (key === "textSize") { document.querySelector(".account-layout").style.setProperty("--account-scale", s.textSize / 100); }
        }
      }));
      function updateNotifications() {
        s.notificationSaved = false;
        document.getElementById("account-save-notifications").disabled = !notificationDirty();
        document.querySelector("#account-notification-form .account-saved").textContent = "";
      }
      document.querySelectorAll("[data-notification]").forEach((input) => input.addEventListener("change", () => { s.notificationDraft[input.dataset.notification] = input.checked; updateNotifications(); }));
      document.getElementById("account-notification-form")?.addEventListener("submit", (e) => {
        e.preventDefault(); s.notifications = { ...s.notificationDraft }; s.notificationSaved = true; M.render();
        document.getElementById("account-heading")?.focus();
      });
      document.querySelectorAll('[name="account-theme"]').forEach((input) => input.addEventListener("change", () => {
        M.state.theme = input.value; M.state.themeMode = input.value; M.render();
        document.querySelector(`[name="account-theme"][value="${M.state.theme}"]`)?.focus();
      }));
    },
  };
  const open = (section) => {
    if (!validSection(section)) return;
    s.section = section; s.open = true; M.go("settings");
    document.getElementById("account-heading")?.focus();
  };
  Object.assign(M.actions, {
    servers: () => open("connection"),
    accountSection: (button) => open(button.dataset.section),
    accountBack: () => { s.open = false; M.render(); document.querySelector(`[data-section="${s.section}"]`)?.focus(); },
    editProfile: () => open("profile"), appearance: () => open("appearance"),
    accountCamera: () => { s.preview = !s.preview; M.render(); document.querySelector('[data-action="accountCamera"]')?.focus(); },
    accountTest: (button) => {
      const parent = button.parentElement, status = parent.querySelector(".account-test-status");
      if (button.getAttribute("aria-pressed") === "true") return;
      button.setAttribute("aria-pressed", "true"); parent.classList.add("testing");
      if (status) status.textContent = "Демонстрация проверки…";
      setTimeout(() => { button.setAttribute("aria-pressed", "false"); parent.classList.remove("testing"); if (status) status.textContent = "Демонстрация завершена"; }, 1800);
    },
  });
})();
