/* Meeting comments for the interactive concept. Data stays in this preview. */
(() => {
  const M = window.MS;
  const E = (value) => M.escape(value);
  const I = (name) => M.icon(name);
  const threads = new Map();
  const title = () => M.state.meetingTitle || "Дизайн-синхронизация";

  function thread() {
    const key = M.state.meetingMeta?.id || title();
    if (!threads.has(key)) {
      threads.set(key, {
        draft: "",
        scroll: 0,
        comments: title() === "Дизайн-синхронизация" ? [
          { name: "Анна Кузнецова", initials: "АК", tone: "peach", time: "10:04", text: "Давайте соберём здесь замечания к макету и решения по итогам встречи." },
          { name: "Мария Полякова", initials: "МП", tone: "mint", time: "10:08", text: "В первом экране важно оставить быстрый переход к предстоящей встрече. Так будет проще вернуться к обсуждению." },
          { name: "Дмитрий Волков", initials: "ДВ", tone: "lilac", time: "10:11", text: "Согласен. Обновлю сценарий первого входа и покажу команде следующую версию." },
        ] : [],
      });
    }
    return threads.get(key);
  }

  const comment = (item) => `<li class="meeting-comment">
    <span class="comment-avatar ${item.tone}" aria-hidden="true">${E(item.initials)}</span>
    <div class="comment-content"><div class="comment-byline"><strong>${E(item.name)}${item.own ? ' <span>· Вы</span>' : ""}</strong><time>${E(item.time)}</time></div>
    <p>${E(item.text)}</p></div>
  </li>`;

  M.callCommentsControl = () => `<button class="call-control comments-control ${M.state.callComments ? "selected" : ""}" data-action="callComments" aria-label="Комментарии" aria-expanded="${!!M.state.callComments}" aria-controls="call-comments">
    <span class="control-disc">${I("MessagesSquare")}<span class="comments-control-count" aria-hidden="true">${thread().comments.length}</span></span><span>Комментарии</span>
  </button>`;

  M.renderCallComments = () => {
    const current = thread();
    return `<aside id="call-comments" class="call-comments" aria-labelledby="call-comments-title" ${M.state.callComments ? "" : "hidden"}>
      <header class="comments-header"><div class="comments-heading"><h2 id="call-comments-title" tabindex="-1">Комментарии</h2><span class="comments-count">${current.comments.length}</span></div>
      <button class="icon-button" data-action="callComments" aria-label="Закрыть комментарии">${I("X")}</button>
      <p>К встрече «${E(title())}»</p></header>
      <div class="comments-feed" tabindex="0" aria-label="Список комментариев">
        <p class="comments-date">Сегодня</p>
        <ol class="comments-list" role="log" aria-label="Комментарии к встрече" aria-live="polite" aria-relevant="additions">${current.comments.map(comment).join("")}</ol>
        <div class="comments-empty" ${current.comments.length ? "hidden" : ""}>${I("MessagesSquare")}<h3>Начните обсуждение</h3><p>Оставьте первый комментарий к этой встрече.</p></div>
      </div>
      <form id="call-comment-form" class="comment-composer">
        <label for="call-comment-input">Ваш комментарий</label>
        <textarea id="call-comment-input" name="comment" rows="3" maxlength="2000" placeholder="Поделитесь идеей или оставьте замечание…">${E(current.draft)}</textarea>
        <div class="comment-compose-actions"><span>Видно участникам встречи</span><button type="submit" ${current.draft.trim() ? "" : "disabled"} aria-label="Добавить комментарий">${I("ArrowUp")} Добавить</button></div>
      </form>
    </aside>`;
  };

  M.bindCallComments = () => {
    const panel = document.getElementById("call-comments");
    if (!panel || panel.hidden) return;
    const current = thread();
    const feed = panel.querySelector(".comments-feed");
    const form = panel.querySelector("form");
    const input = form.elements.comment;
    const submit = form.querySelector("button");
    feed.scrollTop = current.scroll;
    feed.addEventListener("scroll", () => { current.scroll = feed.scrollTop; });
    input.addEventListener("input", () => {
      current.draft = input.value;
      submit.disabled = !input.value.trim();
    });
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const text = input.value.trim();
      if (!text) return;
      const name = M.state.participantName || M.state.profileName || "Александр Соколов";
      const item = {
        name,
        initials: name.split(/\s+/).slice(0, 2).map((part) => part[0]).join("").toUpperCase(),
        tone: "blue", own: true, text,
        time: new Intl.DateTimeFormat("ru-RU", { hour: "2-digit", minute: "2-digit" }).format(new Date()),
      };
      current.comments.push(item);
      current.draft = "";
      panel.querySelector(".comments-list").insertAdjacentHTML("beforeend", comment(item));
      panel.querySelector(".comments-empty").hidden = true;
      panel.querySelector(".comments-count").textContent = current.comments.length;
      document.querySelector(".comments-control-count").textContent = current.comments.length;
      input.value = "";
      submit.disabled = true;
      feed.scrollTop = feed.scrollHeight;
      current.scroll = feed.scrollTop;
      input.focus();
    });
    input.addEventListener("keydown", (event) => {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter" && !event.isComposing) {
        event.preventDefault();
        form.requestSubmit();
      }
    });
    panel.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        M.actions.callComments();
      }
    });
  };

  M.actions.callComments = () => {
    M.state.callComments = !M.state.callComments;
    if (M.state.callComments) M.state.callChat = false;
    M.render();
    document.querySelector(M.state.callComments ? "#call-comments-title" : ".comments-control")?.focus();
  };
})();
