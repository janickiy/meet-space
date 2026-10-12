/* Interactive meeting-list prototype. Fixtures and created meetings stay in memory. */
(() => {
  const M = window.MS,
    I = (name) => M.icon(name),
    E = (value) => M.escape(value);
  const tabs = [
    ["upcoming", "Предстоящие"],
    ["active", "Активные"],
    ["past", "Завершённые"],
  ];
  const scopes = {
    all: "Все мои встречи",
    owned: "Я организатор",
    participating: "Я участник",
  };
  const state = () =>
    (M.state.meetings ||= {
      tab: "upcoming",
      query: "",
      scope: "all",
      from: "",
      to: "",
    });
  const fixtures = [
    {
      id: "design",
      title: "Дизайн-синхронизация",
      date: "2026-10-12",
      time: "10:00",
      duration: 45,
      owner: "Анна Кузнецова",
      mine: false,
      people: 5,
      status: "upcoming",
      chat: "team",
    },
    {
      id: "release",
      title: "Обсуждение новой версии",
      date: "2026-10-12",
      time: "13:30",
      duration: 30,
      owner: "Вы",
      mine: true,
      people: 8,
      status: "upcoming",
      chat: "dev",
    },
    {
      id: "weekly",
      title: "Планы на неделю",
      date: "2026-10-13",
      time: "11:00",
      duration: 30,
      owner: "Вы",
      mine: true,
      people: 4,
      status: "upcoming",
      chat: "team",
    },
    {
      id: "room",
      title: "Комната команды",
      date: "",
      time: "",
      duration: 0,
      owner: "Вы",
      mine: true,
      people: 5,
      status: "upcoming",
      chat: "team",
    },
    {
      id: "active",
      title: "Рабочая встреча разработки",
      date: "2026-10-12",
      time: "09:30",
      duration: 60,
      owner: "Максим Петров",
      mine: false,
      people: 3,
      status: "active",
      chat: "dev",
    },
    {
      id: "review",
      title: "Продукт. Следующая глава",
      date: "2026-10-09",
      time: "14:00",
      duration: 45,
      owner: "Вы",
      mine: true,
      people: 6,
      status: "past",
      record: 0,
      chat: "team",
    },
    {
      id: "demo",
      title: "Демо новой версии",
      date: "2026-10-08",
      time: "11:00",
      duration: 60,
      owner: "Анна Кузнецова",
      mine: false,
      people: 8,
      status: "past",
      record: 3,
      chat: "dev",
    },
    {
      id: "cancelled",
      title: "Обсуждение бюджета",
      date: "2026-10-07",
      time: "16:00",
      duration: 30,
      owner: "Вы",
      mine: true,
      people: 3,
      status: "past",
      cancelled: true,
    },
  ];
  const all = () => [...(M.state.meetingItems || []), ...fixtures];
  const find = (id) => all().find((m) => m.id === id);
  const dateLabel = (date) =>
    date
      ? new Date(`${date}T12:00:00`).toLocaleDateString("ru-RU", {
          day: "numeric",
          month: "long",
        })
      : "Без даты";
  const people = (n) =>
    `${n} ${n % 10 === 1 && n % 100 !== 11 ? "участник" : n % 10 >= 2 && n % 10 <= 4 && !(n % 100 >= 12 && n % 100 <= 14) ? "участника" : "участников"}`;
  const statusLabel = (m) =>
    m.cancelled
      ? "Отменена"
      : m.status === "past"
        ? "Завершена"
        : m.status === "active"
          ? "В эфире"
          : m.date
            ? "Запланирована"
            : "Без даты";
  const summary = (m) =>
    m
      ? `${m.date ? `${dateLabel(m.date)} · ${E(m.time)}${m.duration ? ` · ${m.duration} мин` : ""}` : "Без даты · можно начать в удобное время"}<br/>Организатор: ${E(m.owner)}`
      : "Проверьте звук и камеру перед встречей";
  M.meetingSummary = summary;
  const matched = () => {
    const s = state(),
      query = s.query.trim().toLocaleLowerCase("ru");
    return all().filter(
      (m) =>
        m.status === s.tab &&
        m.title.toLocaleLowerCase("ru").includes(query) &&
        (s.scope === "all" || (s.scope === "owned" ? m.mine : !m.mine)) &&
        (!s.from || (Boolean(m.date) && m.date >= s.from)) &&
        (!s.to || (Boolean(m.date) && m.date <= s.to)),
    );
  };
  const filterCount = () =>
    Number(state().scope !== "all") +
    Number(Boolean(state().from || state().to));
  const fields = () =>
    `<label class="field">Моё участие<select name="scope" aria-label="Моё участие">${Object.entries(
      scopes,
    )
      .map(
        ([id, label]) =>
          `<option value="${id}" ${state().scope === id ? "selected" : ""}>${label}</option>`,
      )
      .join(
        "",
      )}</select></label><label class="field">С даты<input name="from" type="date" aria-label="Встречи с даты" value="${E(state().from)}" ${state().to ? `max="${state().to}"` : ""}/></label><label class="field">По дату<input name="to" type="date" aria-label="Встречи по дату" value="${E(state().to)}" ${state().from ? `min="${state().from}"` : ""}/></label>`;
  const entry = (m) =>
    `<article class="cf-entry ${m.status === "active" ? "is-live" : ""}" data-meeting-id="${m.id}"><div class="cf-date-block">${m.date ? `<strong>${E(m.time)}</strong><span>${dateLabel(m.date)}</span>` : `${I("Video")}<span>Без даты</span>`}</div><div class="cf-entry-copy"><span class="cf-status ${m.status === "active" ? "live" : ""} ${m.cancelled ? "cancelled" : ""}">${m.status === "active" ? "<i></i>" : ""}${statusLabel(m)}</span><button class="cf-title" type="button" data-action="${m.status === "past" ? "meetingDetails" : "openConference"}" data-id="${m.id}">${E(m.title)}</button><p>${m.duration ? `${m.duration} мин <span>·</span> ` : ""}${m.mine ? "Вы организатор" : "Вы участник"}</p><div class="cf-attendees">${I("Users")} ${people(m.people)}${m.mine ? "" : `<span>· ${E(m.owner)}</span>`}</div></div><div class="cf-entry-actions ${m.status === "past" ? "" : "details-only"}"><button class="icon-button" data-action="meetingDetails" data-id="${m.id}" aria-label="Подробнее о встрече ${E(m.title)}">${I("Ellipsis")}</button>${m.status === "past" ? `<button class="button secondary" data-action="meetingDetails" data-id="${m.id}">${I("Folder")}<span>Материалы</span></button>` : ""}</div></article>`;
  const empty = () =>
    `<div class="cf-empty"><span>${I("CalendarDays")}</span><h2>${state().query || filterCount() ? "Встречи не найдены" : state().tab === "active" ? "Сейчас нет активных встреч" : state().tab === "past" ? "Здесь будет история встреч" : "Самое время для первой встречи"}</h2><p>${state().query || filterCount() ? "Попробуйте другое название или измените фильтры." : "Создайте встречу и пригласите коллег по ссылке."}</p><button class="button secondary" data-action="${state().query || filterCount() ? "resetMeetings" : "newMeeting"}">${state().query || filterCount() ? "Сбросить фильтры" : "Создать встречу"}</button></div>`;
  const caption = () =>
    ({
      upcoming: "Предстоящие и без даты",
      active: "Встречи в эфире",
      past: "Завершённые и отменённые",
    })[state().tab];
  function updateResults() {
    const results = matched();
    document.getElementById("meetings-results").innerHTML =
      results.map(entry).join("") || empty();
    document.getElementById("meeting-result-count").textContent =
      `${caption()} · ${results.length}`;
    document.getElementById("clear-meeting-search").hidden = !state().query;
  }
  M.screens.meetings = {
    title: "Встречи",
    group: "Рабочее пространство",
    render: () =>
      M.shell(
        `<div class="cf-page"><section class="cf-start" aria-label="Создание встречи"><div class="cf-quick-actions"><button class="cf-quick primary" data-action="newMeeting">${I("Video")}<span><strong>Новая встреча</strong><small>Начать разговор сейчас</small></span>${I("ArrowUpRight")}</button><button class="cf-quick" data-action="schedule">${I("CalendarDays")}<span><strong>Запланировать<span class="cf-desktop-word"> встречу</span></strong><small>Выбрать дату и время</small></span>${I("ArrowUpRight")}</button><button class="cf-join" data-action="join">${I("Link")}<span>Подключиться по ссылке</span>${I("ArrowRight")}</button></div></section><section class="cf-list-section" aria-label="Список встреч"><div class="cf-list-heading"><h2>Мои встречи</h2><span>Москва · UTC+3</span></div><div class="cf-search-row"><label class="search-box cf-search">${I("Search")}<input id="meeting-search" type="search" aria-label="Найти встречу" placeholder="Найти встречу" value="${E(state().query)}"/></label><button id="clear-meeting-search" class="icon-button cf-clear" aria-label="Очистить поиск" ${state().query ? "" : "hidden"}>${I("X")}</button><button class="cf-filter-button" data-action="meetingFilters" aria-label="Фильтры встреч">${I("SlidersHorizontal")}${filterCount() ? `<span>${filterCount()}</span>` : ""}</button></div><div class="cf-tabs" role="tablist" aria-label="Статус встреч">${tabs.map(([id, label]) => `<button role="tab" id="meeting-tab-${id}" aria-controls="meetings-results" aria-selected="${state().tab === id}" tabindex="${state().tab === id ? 0 : -1}" data-meeting-tab="${id}">${label}</button>`).join("")}</div><form class="cf-filters" id="desktop-meeting-filters">${fields()}<button class="icon-button" type="button" data-action="resetMeetings" aria-label="Сбросить фильтры">${I("RotateCcw")}</button></form><div class="cf-mobile-filters"><button data-action="meetingFilters">${I("Users")}${scopes[state().scope]}${I("ChevronDown")}</button><button data-action="meetingFilters" class="${state().from || state().to ? "selected" : ""}">${I("CalendarDays")}${state().from || state().to ? "Период выбран" : "Период"}</button>${filterCount() ? '<button class="cf-reset" data-action="resetMeetings" aria-label="Сбросить фильтры">×</button>' : ""}</div><div class="cf-list-caption" id="meeting-result-count">${caption()} · ${matched().length}</div><div id="meetings-results" class="cf-results" role="tabpanel" tabindex="0" aria-labelledby="meeting-tab-${state().tab}" aria-live="polite">${matched().map(entry).join("") || empty()}</div></section></div>`,
        "meetings",
        "meetings-content",
      ),
    bind: () => {
      document.getElementById("meeting-search").oninput = (e) => {
        state().query = e.target.value;
        updateResults();
      };
      document.getElementById("clear-meeting-search").onclick = () => {
        state().query = "";
        document.getElementById("meeting-search").value = "";
        updateResults();
        document.getElementById("meeting-search").focus();
      };
      const selectTab = (id) => {
        state().tab = id;
        M.render();
        document
          .getElementById(`meeting-tab-${id}`)
          .focus({ preventScroll: true });
      };
      document.querySelectorAll("[data-meeting-tab]").forEach((b) => {
        b.onclick = () => selectTab(b.dataset.meetingTab);
        b.onkeydown = (e) => {
          const current = tabs.findIndex(([id]) => id === b.dataset.meetingTab);
          const index =
            e.key === "ArrowRight"
              ? (current + 1) % 3
              : e.key === "ArrowLeft"
                ? (current + 2) % 3
                : e.key === "Home"
                  ? 0
                  : e.key === "End"
                    ? 2
                    : -1;
          if (index !== -1) {
            e.preventDefault();
            selectTab(tabs[index][0]);
          }
        };
      });
      const form = document.getElementById("desktop-meeting-filters");
      form.onsubmit = (e) => e.preventDefault();
      form.onchange = () => {
        if (!form.reportValidity()) return;
        Object.assign(state(), Object.fromEntries(new FormData(form)));
        M.render();
      };
    },
  };
  Object.assign(M.actions, {
    resetMeetings: () => {
      Object.assign(state(), { query: "", scope: "all", from: "", to: "" });
      M.closeModal();
      M.render();
    },
    meetingFilters: () =>
      M.modal(
        "Фильтры встреч",
        `<form id="meeting-filter-form" class="stack">${fields()}<p class="cf-form-note">Дата включается в период. Встречи без даты доступны без фильтра по периоду.</p><button class="button primary">Применить</button><button type="button" class="button quiet" data-action="resetMeetings">Сбросить</button></form>`,
        () => {
          const form = document.getElementById("meeting-filter-form");
          form.onchange = () => {
            form.elements.from.max = form.elements.to.value;
            form.elements.to.min = form.elements.from.value;
          };
          form.onsubmit = (e) => {
            e.preventDefault();
            if (!form.reportValidity()) return;
            Object.assign(state(), Object.fromEntries(new FormData(form)));
            M.closeModal();
            M.render();
          };
        },
      ),
    openConference: (b) => {
      const m = find(b.dataset.id);
      if (!m || m.status === "past") return;
      M.state.meetingTitle = m.title;
      M.state.meetingMeta = m;
      M.go("prejoin");
    },
    meetingDetails: (b) => {
      const m = find(b.dataset.id);
      if (!m) return;
      M.modal(
        m.status === "past" ? "Материалы встречи" : "О встрече",
        `<div class="stack cf-details"><span class="cf-status ${m.status === "active" ? "live" : ""}">${statusLabel(m)}</span><h3>${E(m.title)}</h3><p>${summary(m)}<br/>${people(m.people)}${m.waitingRoom ? "<br/>Зал ожидания включён" : ""}</p>${m.status === "past" ? `<p>${m.cancelled ? "Встреча была отменена. Записей и материалов нет." : m.record !== undefined ? "Запись и обсуждение доступны после завершения встречи." : "Встреча завершена без записи."}</p>${m.record !== undefined ? `<button class="button primary" data-action="conferenceRecord" data-id="${m.id}">${I("CirclePlay")}Смотреть запись</button>` : ""}` : `<button class="button primary" data-action="openConference" data-id="${m.id}">${I("Video")}${m.status === "active" ? "Подключиться" : "Открыть встречу"}</button><button class="button secondary" data-action="conferenceInvite" data-id="${m.id}">${I("Link")}Пригласить по ссылке</button>`}${m.chat ? `<button class="button secondary" data-action="chat" data-chat="${m.chat}">${I("MessagesSquare")}Открыть чат встречи</button>` : ""}</div>`,
      );
    },
    conferenceRecord: (b) => {
      const m = find(b.dataset.id);
      if (m?.record !== undefined) {
        M.state.record = m.record;
        M.go("recording");
      }
    },
    conferenceInvite: (b) => {
      const m = find(b.dataset.id);
      if (!m) return;
      const link = `https://meet.example.org/room/${m.id}`;
      M.modal(
        "Пригласить на встречу",
        `<div class="stack"><h3>${E(m.title)}</h3><label class="field">Ссылка-приглашение<input id="conference-invite" readonly value="${E(link)}"/></label><button class="button primary" id="copy-conference-invite">${I("Copy")}Скопировать ссылку</button><p class="cf-form-note">Демонстрационная ссылка для макета.</p></div>`,
        () => {
          document.getElementById("copy-conference-invite").onclick =
            async () => {
              try {
                await navigator.clipboard.writeText(link);
                M.toast("Ссылка скопирована");
              } catch {
                document.getElementById("conference-invite").select();
                M.toast("Ссылка выделена — скопируйте её вручную");
              }
            };
        },
      );
    },
    newMeeting: () => meetingForm(false),
    schedule: () => meetingForm(true),
  });
  function meetingForm(planned) {
    M.modal(
      planned ? "Запланировать встречу" : "Новая встреча",
      `<form id="meeting-form" class="stack"><label class="field">Название встречи<input name="title" placeholder="Например, обсуждение проекта" required maxlength="80"/></label><label class="cf-form-switch"><span><strong>Запланировать встречу</strong><small>Выберите дату и время заранее</small></span><input name="planned" class="toggle" type="checkbox" ${planned ? "checked" : ""}/></label><div id="schedule-fields" ${planned ? "" : "hidden"}><div class="two-fields"><label class="field">Дата<input name="date" type="date" value="2026-10-12" ${planned ? "required" : "disabled"}/></label><label class="field">Время<input name="time" type="time" value="17:00" ${planned ? "required" : "disabled"}/></label></div><label class="field cf-duration">Длительность<select name="duration" ${planned ? "" : "disabled"}><option value="30">30 минут</option><option value="45">45 минут</option><option value="60">1 час</option><option value="90">1,5 часа</option></select></label><p class="cf-form-note">Москва · UTC+3</p></div><label class="cf-form-switch"><span><strong>Зал ожидания</strong><small>Для участников без допуска</small></span><input name="waitingRoom" class="toggle" type="checkbox"/></label><p class="cf-form-note">Пригласите коллег по ссылке после создания. Встречу запускает организатор.</p><button class="button primary" id="create-conference-submit">${planned ? "Запланировать" : "Создать встречу"}</button></form>`,
      () => {
        const form = document.getElementById("meeting-form");
        form.elements.planned.onchange = () => {
          const on = form.elements.planned.checked;
          document.getElementById("schedule-fields").hidden = !on;
          ["date", "time", "duration"].forEach((name) => {
            form.elements[name].disabled = !on;
          });
          ["date", "time"].forEach((name) => {
            form.elements[name].required = on;
          });
          document.getElementById("create-conference-submit").textContent = on
            ? "Запланировать"
            : "Создать встречу";
        };
        form.elements.title.oninput = () =>
          form.elements.title.setCustomValidity("");
        form.onsubmit = (e) => {
          e.preventDefault();
          const title = form.elements.title.value.trim();
          form.elements.title.setCustomValidity(
            title ? "" : "Введите название встречи",
          );
          if (!form.reportValidity()) return;
          const scheduled = form.elements.planned.checked;
          const m = {
            id: `created-${(M.state.meetingItems || []).length + 1}`,
            title,
            date: scheduled ? form.elements.date.value : "",
            time: scheduled ? form.elements.time.value : "",
            duration: scheduled ? Number(form.elements.duration.value) : 0,
            waitingRoom: form.elements.waitingRoom.checked,
            owner: "Вы",
            mine: true,
            people: 1,
            status: "upcoming",
          };
          (M.state.meetingItems ||= []).unshift(m);
          if (scheduled)
            M.state.created = {
              ...m,
              end: `${m.duration} мин`,
              meta: "Вы организатор",
              tone: "mint",
            };
          Object.assign(state(), {
            tab: "upcoming",
            query: "",
            scope: "all",
            from: "",
            to: "",
          });
          M.go("meetings");
          M.modal(
            scheduled ? "Встреча запланирована" : "Встреча создана",
            `<div class="stack cf-created"><span class="cf-success">${I("Check")}</span><h3>${E(m.title)}</h3><p>${summary(m)}</p><button class="button primary" data-action="conferenceInvite" data-id="${m.id}">${I("Link")}Пригласить по ссылке</button><button class="button secondary" data-action="openConference" data-id="${m.id}">${I("Video")}Открыть встречу</button><button class="button quiet" data-close>К списку встреч</button></div>`,
          );
        };
      },
    );
  }
})();
