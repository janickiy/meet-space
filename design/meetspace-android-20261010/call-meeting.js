/* Interactive video meeting: messages, local attachments and recording controls. */
(() => {
  const M = window.MS;
  const E = (value) => M.escape(value);
  const I = (name) => M.icon(name);
  const sessions = new Map();
  const meetingTitle = () => M.state.meetingTitle || "Дизайн-синхронизация";
  function session() {
    const key = M.state.meetingMeta?.id || (meetingTitle() === "Дизайн-синхронизация" ? "design" : meetingTitle());
    if (!sessions.has(key)) sessions.set(key, {
      recording: "recording", draft: "", files: [], messages: [], scroll: 0,
    });
    return sessions.get(key);
  }
  const fileSize = (size) => size < 1024 ? `${size} Б` : size < 1048576
    ? `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 1 }).format(size / 1024)} КБ`
    : `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 1 }).format(size / 1048576)} МБ`;
  const fileIcon = (file) => file.type?.startsWith("image/") ? "FileImage" : "FileText";
  const fileCard = (file) => `<a class="call-file-card" href="${E(file.url)}" download="${E(file.name)}" aria-label="Скачать ${E(file.name)}">
    <span class="call-file-icon">${I(fileIcon(file))}</span><span><strong>${E(file.name)}</strong><small>${fileSize(file.size)}</small></span>${I("Download")}</a>`;
  const message = (item) => `<div class="call-chat-message ${item.own ? "mine" : ""}"><div class="call-chat-bubble">
    <strong class="call-chat-author">${E(item.name)}</strong>${item.text ? `<p>${E(item.text)}</p>` : ""}
    ${(item.files || []).map(fileCard).join("")}<small class="call-chat-time">${E(item.time)}${item.own ? " · Вы" : ""}</small>
  </div></div>`;
  const initialMessages = () => [
    { name: "Мария Полякова", text: "Хорошо видно, можем начинать!", time: "10:02" },
    { name: "Анна Кузнецова", text: "Прикрепляю повестку. Здесь же можно оставить вопросы к встрече.", time: "10:05", files: [{ name: "Повестка встречи.txt", size: 452, url: "assets/meeting-agenda.txt" }] },
    { name: "Дмитрий Волков", text: "Давайте начнём со сценария первого входа.", time: "10:08" },
  ];
  const pendingFiles = () => session().files.map((file) => `<div class="call-pending-file">
    <span class="call-file-icon">${I(fileIcon(file))}</span><span><strong>${E(file.name)}</strong><small>${fileSize(file.size)}</small></span>
    <button type="button" class="icon-button" data-action="removeCallFile" data-file-id="${file.id}" aria-label="Убрать файл ${E(file.name)}">${I("X")}</button></div>`).join("");

  M.renderMeetingChat = () => `<aside id="meeting-chat" class="call-side meeting-chat" aria-labelledby="meeting-chat-title" ${M.state.callChat ? "" : "hidden"}>
    <header class="meeting-chat-header"><div><h2 id="meeting-chat-title" tabindex="-1">Чат встречи</h2><p>Сообщения и файлы · 5 участников</p></div><button class="icon-button" data-action="callChat" aria-label="Закрыть чат">${I("X")}</button></header>
    <div class="call-chat-feed" tabindex="0" aria-label="Сообщения встречи"><div class="call-chat-date">Сегодня</div><div class="call-chat-log" role="log" aria-live="polite" aria-relevant="additions" aria-label="Чат встречи">${[...initialMessages(), ...session().messages].map(message).join("")}</div></div>
    <form id="call-message-form" class="call-chat-composer">
      <div class="call-pending-files" aria-label="Прикреплённые файлы" ${session().files.length ? "" : "hidden"}>${pendingFiles()}</div>
      <label for="call-message-input">Сообщение участникам</label>
      <div class="call-compose-row"><button class="icon-button call-attach" type="button" data-action="attachCallFile" aria-label="Прикрепить файлы">${I("Paperclip")}</button><textarea id="call-message-input" name="message" rows="2" maxlength="4000" placeholder="Написать сообщение…">${E(session().draft)}</textarea><button class="icon-button call-send" type="submit" aria-label="Отправить сообщение" ${session().draft.trim() || session().files.length ? "" : "disabled"}>${I("ArrowUp")}</button></div>
      <input id="call-file-input" type="file" multiple hidden aria-label="Выбрать файлы для встречи"/>
    </form>
  </aside>`;

  function refreshFiles() {
    const pending = document.querySelector(".call-pending-files");
    if (!pending) return;
    pending.innerHTML = pendingFiles();
    pending.hidden = !session().files.length;
    document.querySelector(".call-send").disabled = !session().files.length && !session().draft.trim();
  }

  M.bindMeetingChat = () => {
    const panel = document.getElementById("meeting-chat");
    if (!panel || panel.hidden) return;
    const current = session();
    const feed = panel.querySelector(".call-chat-feed");
    const form = panel.querySelector("form");
    const input = form.elements.message;
    feed.scrollTop = current.scroll;
    feed.addEventListener("scroll", () => { current.scroll = feed.scrollTop; });
    input.addEventListener("input", () => {
      current.draft = input.value;
      form.querySelector(".call-send").disabled = !input.value.trim() && !current.files.length;
    });
    document.getElementById("call-file-input").addEventListener("change", (event) => {
      for (const file of event.target.files) current.files.push({
        id: crypto.randomUUID(), name: file.name, size: file.size, type: file.type, url: URL.createObjectURL(file),
      });
      event.target.value = "";
      refreshFiles();
    });
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const text = input.value.trim();
      if (!text && !current.files.length) return;
      const item = { name: M.state.participantName || M.state.profileName || "Александр Соколов", own: true, text,
        files: [...current.files], time: new Intl.DateTimeFormat("ru-RU", { hour: "2-digit", minute: "2-digit" }).format(new Date()) };
      current.messages.push(item);
      panel.querySelector(".call-chat-log").insertAdjacentHTML("beforeend", message(item));
      current.draft = "";
      current.files = [];
      input.value = "";
      refreshFiles();
      feed.scrollTop = feed.scrollHeight;
      current.scroll = feed.scrollTop;
      input.focus();
    });
    input.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter" && !event.isComposing) {
        event.preventDefault(); form.requestSubmit();
      }
    });
    panel.addEventListener("keydown", (event) => {
      if (event.key === "Escape") { event.preventDefault(); M.actions.callChat(); }
    });
  };

  const recordingLabels = { recording: "Идёт запись", paused: "На паузе", stopped: "Записать" };
  const recordingIcons = { recording: "CircleDot", paused: "Pause", stopped: "Circle" };
  M.renderRecordingControl = () => `<button class="recording-control ${session().recording}" data-action="manageRecording" aria-label="Управление записью: ${recordingLabels[session().recording]}" aria-haspopup="dialog">${I(recordingIcons[session().recording])}<span>${recordingLabels[session().recording]}</span>${I("ChevronDown")}</button>`;
  M.actions.manageRecording = () => {
    const recording = session().recording;
    M.modal("Запись встречи", `<div class="recording-panel stack">
      <div class="recording-summary ${recording}"><span>${I(recordingIcons[recording])}</span><div><h3>${recording === "stopped" ? "Запись остановлена" : recordingLabels[recording]}</h3><p>${E(meetingTitle())}</p></div></div>
      <p>Видео, звук и демонстрация экрана в одной записи.</p>
      ${recording === "stopped" ? `<button class="button primary" data-action="setRecording" data-recording="recording">${I("Circle")}Начать запись</button>` : `<button class="button primary" data-action="setRecording" data-recording="${recording === "recording" ? "paused" : "recording"}">${I(recording === "recording" ? "Pause" : "Play")}${recording === "recording" ? "Приостановить запись" : "Продолжить запись"}</button><button class="button secondary recording-stop" data-action="setRecording" data-recording="stopped">${I("Square")}Остановить запись</button>`}
    </div>`);
  };
  M.actions.setRecording = (button) => {
    const next = button.dataset.recording;
    if (!["recording", "paused", "stopped"].includes(next)) return;
    const previous = session().recording;
    session().recording = next;
    M.closeModal(); M.render();
    document.querySelector(".recording-control")?.focus();
    M.toast(next === "paused" ? "Запись на паузе" : next === "stopped" ? "Запись остановлена" : previous === "paused" ? "Запись продолжена" : "Запись начата");
  };
  M.actions.callChat = () => {
    M.state.callChat = !M.state.callChat;
    M.render();
    document.querySelector(M.state.callChat ? "#meeting-chat-title" : ".call-toolbar [data-action=callChat]")?.focus();
  };
  M.actions.attachCallFile = () => document.getElementById("call-file-input")?.click();
  M.actions.removeCallFile = (button) => {
    const current = session();
    const file = current.files.find((item) => item.id === button.dataset.fileId);
    if (!file) return;
    URL.revokeObjectURL(file.url);
    current.files = current.files.filter((item) => item !== file);
    refreshFiles();
    document.querySelector(".call-attach")?.focus();
  };
})();
