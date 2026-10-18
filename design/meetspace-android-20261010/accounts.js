/* Прототип дизайна с несколькими аккаунтами.  Никаких запросов к серверу и хранения учетных данных. */
(() => {
  const M = window.MS, I = (name) => M.icon(name), E = (value) => M.escape(value);
  const s = M.state.account;
  const clone = (value) => JSON.parse(JSON.stringify(value));
  const preferenceKeys = ["mic", "speaker", "ringtone", "autoMute", "noise", "camera", "cameraOff", "selfVideo", "hideVideo", "notifications", "notificationDraft", "textSize"];
  const preferences = () => Object.fromEntries(preferenceKeys.map((key) => [key, clone(s[key])]));
  const defaults = preferences();
  let accounts = [
    { id: "personal", name: "Александр Соколов", email: "alex@example.org", server: "https://meet.example.org", preferences: clone(defaults) },
    { id: "studio", name: "Александр · Студия", email: "alex@studio.example.org", server: "https://team.example.org", preferences: { ...clone(defaults), autoMute: true } },
  ];
  let activeId = "personal", addMode = "login";
  let addDraft = { server: "https://meet.example.org", name: "", email: "" };
  const active = () => accounts.find((account) => account.id === activeId);
  const host = (account) => new URL(account.server).host;
  const initials = (name) => name.trim().split(/\s+/).filter((part) => /[\p{L}\p{N}]/u.test(part)).slice(0, 2).map((part) => Array.from(part)[0]).join("").toUpperCase();
  const avatar = (account) => `<span class="account-avatar" aria-hidden="true">${E(initials(account.name))}</span>`;
  const capture = () => {
    const account = active();
    if (!account) return;
    account.name = M.state.profileName || account.name;
    account.email = M.state.profileEmail || account.email;
    account.preferences = preferences();
  };
  const apply = () => {
    const account = active();
    Object.assign(s, clone(defaults), account ? clone(account.preferences) : {}, {
      nameDraft: account?.name || "", profileSaved: false, notificationSaved: false,
      connectionDraft: null, connectionOriginal: null, connectionSaved: false, preview: false,
    });
    M.state.profileName = account?.name || "";
    M.state.profileEmail = account?.email || "";
    M.state.participantName = account?.name || "";
    if (account) M.getOnboardingState().server = account.server;
  };
  const open = (section) => { s.section = section; s.open = true; M.go("settings"); document.getElementById("account-heading")?.focus(); };
  const canChange = () => {
    if (Object.values(M.state.roomRefresh || {}).some(room => ['starting','recording','processing'].includes(room.recording))) {
      M.modal("Сначала завершите запись", `<div class="stack"><p>Остановите запись встречи и дождитесь обработки, чтобы сменить или удалить аккаунт.</p><button class="button primary" data-go="call">Вернуться к встрече</button><button class="button secondary" data-close>Остаться в настройках</button></div>`);return false;
    }
    if (!M.state.personalCall) return true;
    M.modal("Сначала завершите звонок", `<div class="stack"><p>После завершения личного звонка можно сменить аккаунт.</p><button class="button primary" data-action="resumePersonalCall" data-close>Вернуться к звонку</button><button class="button secondary" data-close>Остаться в настройках</button></div>`);
    return false;
  };
  M.updateCurrentAccount = (changes) => { if (active()) Object.assign(active(), changes); };
  M.accountManagerSnapshot = () => { capture(); return { accounts: clone(accounts), activeId, addMode, addDraft: { ...addDraft } }; };
  M.setAccountManagerSnapshot = (value) => {
    if (!value || !value.addDraft || !["server", "name", "email"].every((key) => typeof value.addDraft[key] === "string") || !Array.isArray(value.accounts) || !["login", "register"].includes(value.addMode)) return false;
    if (value.accounts.some((a) => !a || !["id", "name", "email", "server"].every((key) => typeof a[key] === "string") || !a.preferences)) return false;
    try { if (value.accounts.some((a) => new URL(a.server).protocol !== "https:")) return false; } catch { return false; }
    if (value.accounts.length && !value.accounts.some((a) => a.id === value.activeId)) return false;
    if (JSON.stringify(M.accountManagerSnapshot()) === JSON.stringify(value)) return false;
    accounts = value.accounts.map((a) => ({ id: a.id, name: a.name, email: a.email, server: a.server, preferences: Object.fromEntries(preferenceKeys.map((key) => [key, clone(a.preferences[key] ?? defaults[key])])) }));
    activeId = accounts.length ? value.activeId : null; addMode = value.addMode; addDraft = { server: value.addDraft.server, name: value.addDraft.name, email: value.addDraft.email };
    apply(); return true;
  };
  M.completeAccountAuth = ({ name, email, server }, mode = "login") => {
    const normalizedServer = new URL(server).href.replace(/\/$/, "");
    const existing = accounts.find((a) => a.email.toLowerCase() === email.toLowerCase() && a.server === normalizedServer);
    if (existing && mode === "register") return "Аккаунт с этой почтой уже добавлен на этом сервере. Перейдите ко входу.";
    capture();
    const account = existing || { id: crypto.randomUUID(), name: name || email.split("@")[0], email, server: normalizedServer, preferences: clone(defaults) };
    if (!existing) accounts.push(account);
    activeId = account.id; apply();
    return "";
  };
  M.renderAccountDanger = () => active() ? `<section class="account-danger-zone"><span class="account-danger-icon">${I("Trash2")}</span><div><h3>Удаление аккаунта</h3><p>Удалить <strong>${E(active().email)}</strong> с сервера ${E(host(active()))}.</p><button class="account-danger-link" data-action="accountDelete">Удалить аккаунт с сервера ${I("ArrowUpRight")}</button></div></section>` : "";
  M.renderAccounts = () => `<div class="account-panel-heading"><div><span class="accounts-eyebrow">ВАШИ ПРОСТРАНСТВА</span><h2 id="account-heading" tabindex="-1">Аккаунты</h2><p>Выберите аккаунт для встреч и общения.</p></div><span class="accounts-total">${accounts.length}</span></div>
    <div class="accounts-list" aria-label="Добавленные аккаунты">${accounts.map((account) => `<button class="accounts-card ${account.id === activeId ? "is-current" : ""}" data-action="accountSwitch" data-account-id="${E(account.id)}" aria-label="${account.id === activeId ? "Текущий аккаунт" : "Переключиться на аккаунт"}: ${E(account.email)} · ${E(host(account))}" aria-pressed="${account.id === activeId}">${avatar(account)}<span class="accounts-card-copy"><strong>${E(account.name)}</strong><span>${E(account.email)}</span><small>${I("Server")}${E(host(account))}</small></span><span class="accounts-card-state">${account.id === activeId ? `${I("CircleCheck")}<small>Текущий</small>` : `${I("ArrowRight")}<small>Выбрать</small>`}</span></button>`).join("")}</div>
    <button class="button primary accounts-add" data-action="accountAdd">${I("Plus")}Добавить аккаунт</button>
    <p class="account-note">${I("Info")}<span>Можно добавить аккаунты с одного или разных серверов. Настройки сохраняются отдельно для каждого аккаунта.</span></p>
    ${M.renderAccountDanger()}<p class="accounts-demo">Демо · изменения действуют до обновления страницы.</p>`;
  const passwordField = (key, label) => `<label class="account-field" for="account-add-${key}"><span>${label}</span></label><div class="accounts-password"><input id="account-add-${key}" name="${key}" type="password" placeholder="${key === "confirm" ? "Повторите пароль" : addMode === "register" ? "Минимум 8 символов" : "Введите пароль"}" autocomplete="off" required aria-describedby="account-add-error"><button type="button" data-account-password="${key}" aria-label="Показать ${key === "confirm" ? "повтор пароля" : "пароль"}" aria-pressed="false">${I("Eye")}</button></div>`;
  M.renderAddAccount = () => `<div class="account-panel-heading"><div><span class="accounts-eyebrow">ЕЩЁ ОДНО ПРОСТРАНСТВО</span><h2 id="account-heading" tabindex="-1">Добавить аккаунт</h2><p>${addMode === "login" ? "Войдите в существующий аккаунт." : "Создайте аккаунт на выбранном сервере."}</p></div></div>
    <div class="accounts-auth-tabs" role="group" aria-label="Способ добавления аккаунта"><button data-action="accountAddMode" data-mode="login" aria-pressed="${addMode === "login"}">Войти</button><button data-action="accountAddMode" data-mode="register" aria-pressed="${addMode === "register"}">Создать аккаунт</button></div>
    <form id="account-add-form" class="accounts-add-form" novalidate>
      <label class="account-field"><span>Адрес сервера</span><input name="server" type="url" value="${E(addDraft.server)}" placeholder="https://meet.example.org" autocomplete="off" inputmode="url" autocapitalize="none" spellcheck="false" required aria-describedby="account-add-error"></label>
      ${addMode === "register" ? `<label class="account-field"><span>Имя для встреч</span><input name="name" value="${E(addDraft.name)}" placeholder="Как вас зовут" autocomplete="off" maxlength="100" required aria-describedby="account-add-error"></label>` : ""}
      <label class="account-field"><span>Электронная почта</span><input name="email" type="email" value="${E(addDraft.email)}" placeholder="you@company.ru" autocomplete="off" inputmode="email" autocapitalize="none" spellcheck="false" maxlength="254" required aria-describedby="account-add-error"></label>
      <div class="accounts-password-field">${passwordField("password", "Пароль")}${addMode === "register" ? '<small>От 8 до 128 символов.</small>' : ""}</div>
      ${addMode === "register" ? `<div class="accounts-password-field">${passwordField("confirm", "Повторите пароль")}</div>` : ""}
      <p id="account-add-error" class="accounts-form-error" role="alert" hidden></p>
      <button type="submit" class="button primary">${addMode === "login" ? "Войти и добавить" : "Создать и добавить"}${I("ArrowRight")}</button>
      <button type="button" class="button secondary" data-action="accountSection" data-section="accounts">Отмена</button>
    </form><p class="accounts-demo">Демо · данные доступа не отправляются и не сохраняются.</p>`;
  M.bindAccountManager = () => {
    const form = document.getElementById("account-add-form");
    if (!form) return;
    const error = form.querySelector("#account-add-error");
    const fail = (key, text) => { error.textContent = text; error.hidden = false; form.elements[key].setAttribute("aria-invalid", "true"); form.elements[key].focus(); };
    form.addEventListener("input", (event) => { error.hidden = true; event.target.removeAttribute("aria-invalid"); });
    form.querySelectorAll("[data-account-password]").forEach((button) => button.addEventListener("click", () => {
      const input = form.elements[button.dataset.accountPassword], show = input.type === "password";
      input.type = show ? "text" : "password";
      button.setAttribute("aria-pressed", String(show));
      button.setAttribute("aria-label", `${show ? "Скрыть" : "Показать"} ${input.name === "confirm" ? "повтор пароля" : "пароль"}`);
      button.innerHTML = I(show ? "EyeOff" : "Eye");
    }));
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      form.querySelectorAll("[aria-invalid]").forEach((input) => input.removeAttribute("aria-invalid"));
      let server;
      try {
        const raw = form.elements.server.value.trim(), url = new URL(raw);
        if (/[\s,;]/.test(raw) || url.protocol !== "https:" || url.username || url.password || url.search || url.hash) throw Error();
        server = url.href.replace(/\/$/, "");
      } catch { return fail("server", "Введите один адрес HTTPS, например https://meet.example.org"); }
      const name = form.elements.name?.value.trim(), email = form.elements.email.value.trim(), password = form.elements.password.value;
      if (addMode === "register" && (!name || Array.from(name).length > 100)) return fail("name", "Введите имя от 1 до 100 символов.");
      form.elements.email.value = email;
      if (!form.elements.email.validity.valid || new TextEncoder().encode(email).length > 254) return fail("email", "Проверьте адрес электронной почты.");
      if (!password || Array.from(password).length > 128 || (addMode === "register" && Array.from(password).length < 8)) return fail("password", addMode === "register" ? "Пароль должен содержать от 8 до 128 символов." : "Введите пароль длиной до 128 символов.");
      if (addMode === "register" && password !== form.elements.confirm.value) return fail("confirm", "Пароли не совпадают.");
      const errorMessage = M.completeAccountAuth({ name, email, server }, addMode);
      if (errorMessage) return fail("email", errorMessage);
      form.reset(); open("accounts"); M.toast("Демо: аккаунт добавлен и выбран");
    });
  };
  Object.assign(M.actions, {
    accountAdd: () => { if (canChange()) { addMode = "login"; addDraft = { server: active()?.server || M.getOnboardingState().server, name: "", email: "" }; open("add-account"); } },
    accountAddMode: (button) => {
      const form = document.getElementById("account-add-form");
      if (form) {
        addDraft.email = form.elements.email.value; addDraft.name = form.elements.name?.value || addDraft.name;
        try { const url = new URL(form.elements.server.value.trim()); if (url.protocol === "https:" && !url.username && !url.password) addDraft.server = url.href.replace(/\/$/, ""); } catch { /* Сохранять предыдущий действительный адрес. */ }
      }
      addMode = button.dataset.mode === "register" ? "register" : "login"; M.render();
    },
    accountSwitch: (button) => {
      const id = button.dataset.accountId;
      if (id === activeId || !accounts.some((a) => a.id === id) || !canChange()) return;
      capture(); activeId = id; apply(); M.render(); M.toast(`Выбран аккаунт ${active().email}`);
    },
    accountDelete: () => {
      if (!active() || !canChange()) return;
      const target = { ...active() };
      M.modal("Удалить аккаунт с сервера?", `<form id="account-delete-form" class="accounts-delete-form" novalidate><div class="accounts-delete-identity">${avatar(target)}<div><strong>${E(target.name)}</strong><span>${E(target.email)}</span><small>${E(host(target))}</small></div></div><p>Данные входа и личные настройки будут удалены, все сессии завершены. Общая история встреч и переписки останется с именем «Удалённый аккаунт».</p><p class="wr-hint">Перед удалением передайте владение группами, завершите или отмените созданные встречи и выйдите из звонков.</p><label class="account-field"><span>Пароль текущего аккаунта</span><input name="password" type="password" placeholder="В демо — любой текст" autocomplete="off" required maxlength="128"/></label><label class="wr-delete-check"><input name="confirmed" type="checkbox"/>Я понимаю, что удаление нельзя отменить</label><div class="accounts-delete-actions"><button type="button" class="button secondary" data-close>Отмена</button><button type="submit" class="button danger" disabled>${I("Trash2")}Удалить аккаунт</button></div><small class="accounts-demo">В демо удаление имитируется. Аккаунт на сервере не изменится.</small></form>`, () => {
        const form = document.getElementById("account-delete-form"), submit = form.querySelector('[type="submit"]');
        const matches = () => Boolean(form.elements.password.value.trim()) && form.elements.confirmed.checked;
        form.addEventListener("input", () => { submit.disabled = !matches(); });
        form.addEventListener("change", () => { submit.disabled = !matches(); });
        form.addEventListener("submit", (event) => {
          event.preventDefault(); if (!matches() || target.id !== activeId) return;
          accounts = accounts.filter((a) => a.id !== target.id); activeId = accounts[0]?.id || null; apply();
          M.closeModal();
          if (active()) open("accounts"); else M.go("login");
          M.toast("Демо: аккаунт удалён из макета");
        });
      });
    },
    logout: () => {
      if (!active() || !canChange()) return;
      const id = activeId;
      M.modal("Выйти из аккаунта?", `<div class="stack"><p>Аккаунт ${E(active().email)} будет убран с этого устройства. На сервере он сохранится.</p><button class="button primary" id="account-logout-confirm">Выйти</button><button class="button secondary" data-close>Остаться</button></div>`, () => {
        document.getElementById("account-logout-confirm").addEventListener("click", () => { accounts = accounts.filter((a) => a.id !== id); activeId = accounts[0]?.id || null; apply(); M.closeModal(); if (active()) open("accounts"); else M.go("login"); });
      });
    },
  });
  apply();
})();
