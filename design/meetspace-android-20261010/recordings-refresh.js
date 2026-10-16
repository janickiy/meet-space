/* Recordings library and player: local, fictional data; no media or server mutations. */
(() => {
  'use strict';
  const M = window.MS, I = name => M.icon(name), E = value => M.escape(value);
  const records = [
    { title: 'Продукт. Следующая глава', meeting: 'product', date: '15 октября 2026', age: 1, time: '10:00', seconds: 2538, size: '128 МБ', mode: 'video', people: 5, owner: 'Вы', self: true },
    { title: 'Дизайн-ревью мобильного приложения', meeting: 'design', date: '14 октября 2026', age: 2, time: '14:30', seconds: 1866, size: '96 МБ', mode: 'video', people: 4, owner: 'Анна Кузнецова' },
    { title: 'Планы на октябрь', meeting: 'plans', date: '12 октября 2026', age: 4, time: '09:00', seconds: 1722, size: '27 МБ', mode: 'audio', people: 4, owner: 'Вы', self: true },
    { title: 'Демо новой версии', meeting: 'demo', date: '11 октября 2026', age: 5, time: '16:00', seconds: 3252, size: '168 МБ', mode: 'video', people: 8, owner: 'Максим Петров', requested: true },
    { title: 'Продукт. Следующая глава', meeting: 'product', date: '15 октября 2026', age: 1, time: '10:45', seconds: 720, size: '42 МБ', mode: 'tracks', people: 5, owner: 'Вы', self: true },
    { title: 'Ежедневная синхронизация', meeting: 'live', date: '16 октября 2026', age: 0, time: '10:00', seconds: 0, size: '—', mode: 'video', people: 6, owner: 'Вы', self: true, live: true, processing: true },
  ];
  const seedCount = records.length;
  const normalizeMode = mode => ['audio', 'audio_only'].includes(mode) ? 'audio' : ['tracks', 'individual_tracks'].includes(mode) ? 'tracks' : 'video';
  function dynamicRecord(value) {
    if (!value || typeof value.key !== 'string' || !value.key.trim() || typeof value.title !== 'string' || !value.title.trim()) return null;
    const key = value.key.slice(0, 200);
    return {
      key, id: `meeting-recording:${encodeURIComponent(key)}`,
      title: value.title.trim().slice(0, 180), meeting: `recorded:${key}`,
      date: typeof value.date === 'string' ? value.date.slice(0, 60) : '16 октября 2026',
      time: typeof value.time === 'string' && /^\d{2}:\d{2}$/.test(value.time) ? value.time : '10:00',
      age: 0, seconds: Number.isFinite(value.seconds) ? Math.max(0, Math.min(86400, Math.floor(value.seconds))) : 60,
      size: 'Демо', mode: normalizeMode(value.mode), people: Number.isInteger(value.people) ? Math.max(1, Math.min(100, value.people)) : 5,
      owner: 'Вы', self: true, requested: true, dynamic: true,
    };
  }
  const people = n => `${n} ${n%100>=11&&n%100<=14?'участников':n%10===1?'участник':n%10>=2&&n%10<=4?'участника':'участников'}`;
  const labels = { video: 'Видеозапись', audio: 'Аудиозапись', tracks: 'Дорожки участников' };
  const icons = { video: 'Video', audio: 'Mic', tracks: 'Users' };
  const initial = Number(new URLSearchParams(location.search).get('record'));
  M.state.recordLibrary = { query: '', meeting: 'all', status: 'past', period: 'all', sort: 'newest', view: 'list', deleted: [], position: 0, playing: false, materialTab: 'transcript' };
  if (Number.isInteger(initial) && initial >= 0 && initial < records.length) M.state.record = initial;
  const S = () => M.state.recordLibrary;
  const selected = () => Number.isInteger(M.state.record) && records[M.state.record] ? M.state.record : 0;
  const visible = index => records[index] && !S().deleted.includes(index);
  const canDelete = index => visible(index) && !records[index].processing && (records[index].self || records[index].requested);
  const clock = seconds => `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
  const owner = r => r.self ? `${M.state.profileName || 'Александр Соколов'} (вы)` : r.owner;
  let timer = null, pendingDelete = null;
  function stop() { clearInterval(timer); timer = null; }
  function broadcast() { M.recordingsChanged?.(); }
  M.addMeetingRecording = ({ key, title, mode, seconds, people } = {}) => {
    const existing = records.findIndex(record => record.dynamic && record.key === String(key || '').slice(0, 200));
    if (existing !== -1) return existing;
    const now = new Date();
    const record = dynamicRecord({ key, title, mode, seconds, people,
      date: now.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'Europe/Moscow' }).replace(/\s?г\.$/, ''),
      time: now.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' }),
    });
    if (!record) return -1;
    records.push(record);
    broadcast();
    return records.length - 1;
  };
  M.recordingLibrarySnapshot = () => ({ ...S(), deleted: [...S().deleted], record: selected(), dynamicRecords: records.slice(seedCount).map(record => ({ ...record })) });
  M.setRecordingLibrarySnapshot = value => {
    if (!value || typeof value !== 'object') return false;
    let recordsChanged = false;
    if (Array.isArray(value.dynamicRecords)) {
      const keys = new Set();
      const additions = value.dynamicRecords.slice(0, 100).map(dynamicRecord).filter(record => {
        if (!record || keys.has(record.key)) return false;
        keys.add(record.key); return true;
      });
      recordsChanged = JSON.stringify(additions) !== JSON.stringify(records.slice(seedCount));
      if (recordsChanged) records.splice(seedCount, records.length - seedCount, ...additions);
    }
    const next = { ...S() };
    for (const [key, options] of Object.entries({ status: ['past', 'live'], period: ['all', '7', '30', '90'], sort: ['newest', 'oldest'], view: ['list', 'grid'], materialTab: ['transcript', 'summary'] })) {
      if (options.includes(value[key])) next[key] = value[key];
    }
    if (typeof value.query === 'string') next.query = value.query.slice(0, 150);
    if (typeof value.meeting === 'string' && (value.meeting === 'all' || records.some(r => r.meeting === value.meeting))) next.meeting = value.meeting;
    if (Array.isArray(value.deleted)) next.deleted = [...new Set(value.deleted.filter(i => Number.isInteger(i) && records[i] && (records[i].self || records[i].requested)))];
    next.position = Number.isFinite(value.position) ? Math.max(0, Math.floor(value.position)) : 0;
    next.playing = Boolean(value.playing);
    const index = Number.isInteger(value.record) && records[value.record] ? value.record : selected();
    const changed = recordsChanged || JSON.stringify(next) !== JSON.stringify(S()) || index !== selected();
    M.state.recordLibrary = next; M.state.record = index;
    return changed;
  };
  const menuButton = index => `<button class="icon-button rr-menu-button" data-action="rrMenu" data-index="${index}" aria-label="Действия с записью: ${E(records[index].title)}" aria-haspopup="dialog">${I('MoreHorizontal')}</button>`;
  const retention = () => `<div class="rr-retention">${I('Info')}<p>Готовые записи хранятся <strong>7 дней</strong>. Ссылка на страницу доступна только участникам встречи.</p></div>`;
  const waves = () => `<span class="rr-wave" aria-hidden="true">${Array.from({ length: 25 }, (_, i) => `<i style="--wave:${18 + (i * 17 % 38)}px"></i>`).join('')}</span>`;
  function cover(r, index, large = false) {
    return `<div class="rr-cover rr-cover-${r.mode} rr-tone-${index % 3} ${large ? 'rr-cover-large' : ''}">${r.mode === 'audio' ? waves() : r.mode === 'tracks' ? `<div class="rr-tracks" aria-hidden="true">${['АС', 'АК', 'МП'].map((x, i) => `<div><b>${x}</b>${waves()}</div>`).join('')}</div>` : `<div class="rr-participant-grid" aria-hidden="true">${['АС', 'АК', 'МП', 'ДВ'].map((x, i) => `<span class="rr-person-${i}"><b>${x}</b></span>`).join('')}</div>`}<span class="rr-media-icon">${I(r.processing ? 'Circle' : icons[r.mode])}</span>${r.processing ? '<span class="rr-duration rr-live">В эфире</span>' : `<span class="rr-duration">${clock(r.seconds)}</span>`}</div>`;
  }
  function found() {
    const query = S().query.trim().toLocaleLowerCase('ru-RU');
    return records.map((r, index) => ({ ...r, index })).filter(r => visible(r.index) && (S().status === 'live' ? r.live : !r.live) && (S().meeting === 'all' || r.meeting === S().meeting) && (S().period === 'all' || r.age <= Number(S().period)) && (!query || `${r.title} ${r.owner} ${labels[r.mode]}`.toLocaleLowerCase('ru-RU').includes(query))).sort((a, b) => S().sort === 'oldest' ? b.age - a.age || a.time.localeCompare(b.time) : a.age - b.age || b.time.localeCompare(a.time));
  }
  function cards() {
    const list = found();
    if (!list.length) return `<div class="rr-empty">${I('CirclePlay')}<h2>Записи не найдены</h2><p>Попробуйте другую встречу или измените условия поиска.</p><button class="button secondary" data-action="rrReset">Сбросить фильтры</button></div>`;
    return list.map(r => `<article class="rr-card"><button class="rr-open-cover" data-action="record" data-index="${r.index}" aria-label="Открыть запись: ${E(r.title)}">${cover(r, r.index)}</button><div class="rr-card-body"><span class="rr-kind">${I(icons[r.mode])}${labels[r.mode]}${r.processing ? ' · Идёт запись' : ''}</span><button class="rr-record-title" data-action="record" data-index="${r.index}">${E(r.title)}</button><p class="rr-card-meta">${E(r.date.replace(' 2026', ''))} · ${r.time} · ${people(r.people)}</p><p class="rr-owner">Организатор: ${E(owner(r))}</p></div><div class="rr-card-actions">${!r.processing ? `<button class="rr-watch" data-action="record" data-index="${r.index}">${I(r.mode === 'tracks' ? 'FolderOpen' : 'Play')}<span>${r.mode === 'tracks' ? 'Материалы' : r.mode === 'audio' ? 'Слушать' : 'Смотреть'}</span></button>` : `<span class="rr-processing">В эфире</span>`}${menuButton(r.index)}</div></article>`).join('');
  }
  function redrawResults() {
    const results = document.getElementById('rr-results');
    if (results) { results.className = `rr-results rr-view-${S().view}`; results.innerHTML = cards(); }
    const count = document.getElementById('rr-count');
    if (count) count.textContent = `Найдено: ${found().length}`;
    broadcast();
  }
  M.screens.recordings = {
    title: 'Записи встреч',
    render() {
      stop(); S().playing = false;
      const meetings = [...new Map(records.filter(r => S().status === 'live' ? r.live : !r.live).map(r => [r.meeting, r.title])).entries()];
      return M.shell(`<section class="rr-library"><div class="rr-page-heading"><div><span class="rr-eyebrow">БИБЛИОТЕКА ВСТРЕЧ</span><h2>Важное остаётся с вами</h2><p>Видео, аудио и материалы ваших встреч.</p></div></div><div class="rr-meeting-tabs" role="group" aria-label="Состояние встречи">${[['past','Завершённые'],['live','В эфире']].map(([id,label]) => `<button data-action="rrStatus" data-value="${id}" aria-pressed="${S().status === id}">${label}${id === 'live' ? '<i></i>' : ''}</button>`).join('')}</div><section class="rr-selection" aria-label="Поиск записей"><label class="rr-search">${I('Search')}<input id="rr-search" type="search" placeholder="Найти запись встречи" aria-label="Найти запись встречи" value="${E(S().query)}" /></label><label class="rr-meeting-field"><span>Встреча</span><select id="rr-meeting" aria-label="Встреча"><option value="all">Все встречи</option>${meetings.map(([id,title]) => `<option value="${id}" ${S().meeting === id ? 'selected' : ''}>${E(title)}</option>`).join('')}</select></label></section><div class="rr-list-toolbar"><div class="rr-filters"><select id="rr-period" aria-label="Период записей">${[['all','Всё время'],['7','За 7 дней'],['30','За 30 дней'],['90','За 90 дней']].map(([id,label]) => `<option value="${id}" ${S().period === id ? 'selected' : ''}>${label}</option>`).join('')}</select><select id="rr-sort" aria-label="Сортировка записей"><option value="newest" ${S().sort === 'newest' ? 'selected' : ''}>Сначала новые</option><option value="oldest" ${S().sort === 'oldest' ? 'selected' : ''}>Сначала старые</option></select></div><div class="rr-view-switch" role="group" aria-label="Вид записей">${[['list','List','Список'],['grid','Grid2X2','Плитка']].map(([id,icon,label]) => `<button data-action="rrView" data-value="${id}" aria-pressed="${S().view === id}" aria-label="${label}">${I(icon)}</button>`).join('')}</div></div><div class="rr-results-label"><span id="rr-count" role="status">Найдено: ${found().length}</span><span>Демонстрационные записи</span></div><div id="rr-results" class="rr-results rr-view-${S().view}">${cards()}</div>${retention()}</section>`, 'meetings', 'rr-content');
    },
    bind() {
      document.getElementById('rr-search').oninput = e => { S().query = e.target.value; redrawResults(); };
      for (const key of ['meeting', 'period', 'sort']) document.getElementById(`rr-${key}`).onchange = e => { S()[key] = e.target.value; redrawResults(); };
    },
  };
  function details(r) {
    return `<dl class="rr-details"><div><dt>Дата записи</dt><dd>${E(r.date)} · ${r.time}</dd></div><div><dt>Длительность</dt><dd>${r.processing ? 'Идёт запись' : clock(r.seconds)}</dd></div><div><dt>Размер</dt><dd>${r.size}</dd></div><div><dt>Организатор</dt><dd>${E(owner(r))}</dd></div><div><dt>Участники</dt><dd>${r.people}</dd></div><div><dt>Доступ</dt><dd>Участники встречи</dd></div></dl>`;
  }
  const materials = r => r.processing ? [] : r.mode === 'audio' ? [['Аудиозапись', 'M4A', 'Mic']] : r.mode === 'tracks' ? [['Архив дорожек', 'ZIP', 'FolderOpen']] : [['Видеозапись', 'MP4', 'Video'], ['Аудиозапись', 'M4A', 'Mic']];
  function playerButtons() {
    const playing = S().playing;
    document.querySelectorAll('[data-action="rrPlay"]').forEach(button => { button.innerHTML = I(playing ? 'Pause' : 'Play'); button.setAttribute('aria-label', playing ? 'Приостановить демо воспроизведения' : 'Начать демо воспроизведения'); });
    const time = document.getElementById('rr-time'), seek = document.getElementById('rr-seek');
    if (time) time.textContent = clock(S().position);
    if (seek) seek.value = S().position;
  }
  function startTimer() {
    stop();
    if (!S().playing) return;
    timer = setInterval(() => {
      if (M.state.screen !== 'recording') { S().playing = false; stop(); return; }
      S().position = Math.min(S().position + 1, records[selected()].seconds);
      if (S().position >= records[selected()].seconds) { S().playing = false; stop(); }
      playerButtons();
    }, 1000);
  }
  // Illustrative materials are specific to their recording; no transcription service is called.
  const insightFixtures = {
    0: { status: 'ready', lines: [[15, 'Анна Кузнецова', 'Главное — сделать путь от обсуждения до решения короче.'], [146, 'Максим Петров', 'Соберём поиск, записи и материалы встречи в одном месте.'], [328, 'Александр Соколов', 'Проверим мобильный сценарий и покажем результат команде в пятницу.']], summary: 'Обсудили следующую версию продукта и доступ к материалам встреч.', decision: 'Объединить записи, расшифровку и итоги в одном сценарии.', task: 'Анна — проверить мобильный макет к пятнице.' },
    1: { status: 'processing' },
    2: { status: 'ready', lines: [[12, 'Александр Соколов', 'Согласуем приоритеты на оставшуюся часть октября.'], [98, 'Анна Кузнецова', 'Предлагаю завершить библиотеку записей и тестирование настроек.'], [240, 'Максим Петров', 'Возьму проверку интеграции, результаты обсудим в понедельник.']], summary: 'Команда согласовала задачи на вторую половину октября.', decision: 'Приоритет — библиотека записей и настройки аккаунта.', task: 'Максим — проверить интеграцию к понедельнику.' },
    3: { status: 'unavailable' }, 4: { status: 'unavailable' }, 5: { status: 'processing' },
  };
  function insights(index) {
    const item = insightFixtures[index] || { status: records[index]?.dynamic ? 'processing' : 'unavailable' }, tab = S().materialTab || 'transcript';
    const content = item.status === 'processing' ? `<div class="rr-insight-empty">${I('Clock')}<strong>Материалы обрабатываются</strong><p>Расшифровка и итоги появятся после обработки записи.</p></div>` : item.status === 'unavailable' ? `<div class="rr-insight-empty">${I('FileText')}<strong>Материалы пока недоступны</strong><p>Для этой записи нет готовой расшифровки и итогов.</p></div>` : tab === 'transcript' ? `<div class="rr-transcript">${item.lines.map(([time,name,text]) => `<article><button data-action="rrSeek" data-time="${time}" aria-label="Перейти к ${clock(time)}">${clock(time)}</button><div><strong>${E(name)}</strong><p>${E(text)}</p></div></article>`).join('')}</div>` : `<div class="rr-summary"><p>${E(item.summary)}</p><h4>Решение</h4><p>${E(item.decision)}</p><h4>Задача</h4><p>${E(item.task)}</p><small>Пример итогов ИИ. Проверьте важные детали по записи.</small></div>`;
    return `<section class="rr-insights"><div class="rr-insight-tabs" role="group" aria-label="Материалы встречи">${[['transcript','Расшифровка'],['summary','Итоги ИИ']].map(([id,label]) => `<button data-action="rrMaterialTab" data-value="${id}" aria-pressed="${tab === id}">${label}</button>`).join('')}</div><div id="rr-insight-content">${content}</div><p class="rr-insight-demo">Демонстрационные материалы</p></section>`;
  }
  M.screens.recording = {
    title: 'Просмотр записи',
    render() {
      stop(); const index = selected(), r = records[index];
      if (!visible(index)) return M.shell(`<section class="rr-empty">${I('CirclePlay')}<h2>Запись удалена</h2><p>Этой записи больше нет в библиотеке макета.</p><button class="button primary" data-go="recordings">Все записи</button></section>`, 'meetings', 'rr-content');
      S().position = Math.min(S().position, r.seconds);
      return M.shell(`<section class="rr-player-page"><button class="rr-back" data-go="recordings">${I('ArrowLeft')}Все записи</button><div class="rr-player-heading"><div><span class="rr-eyebrow">${labels[r.mode]}</span><h2>${E(r.title)}</h2><p>${E(r.date)} · ${people(r.people)}</p></div>${menuButton(index)}</div><div class="rr-player-layout"><div class="rr-playback"><div class="rr-player-stage">${cover(r, index, true)}${r.processing ? `<div class="rr-stage-message"><strong>Встреча ещё в эфире</strong><p>Материалы появятся после завершения обработки.</p></div>` : r.mode === 'tracks' ? `<div class="rr-stage-message"><strong>Отдельная дорожка каждого участника</strong><p>Скачайте архив в разделе «Материалы».</p></div>` : `<button class="rr-center-play" data-action="rrPlay" aria-label="${S().playing ? 'Приостановить' : 'Начать'} демо воспроизведения">${I(S().playing ? 'Pause' : 'Play')}</button>`}</div>${!r.processing && r.mode !== 'tracks' ? `<div class="rr-player-controls"><button class="icon-button" data-action="rrPlay" aria-label="Начать демо воспроизведения">${I(S().playing ? 'Pause' : 'Play')}</button><span id="rr-time">${clock(S().position)}</span><input id="rr-seek" type="range" min="0" max="${r.seconds}" value="${S().position}" aria-label="Позиция в записи"/><span>${clock(r.seconds)}</span></div><p class="rr-demo-note">Демонстрация плеера · медиафайл не подключён</p>` : ''}${retention()}<button class="button secondary rr-share" data-action="rrCopy" data-index="${index}">${I('Link')}Скопировать ссылку</button>${insights(index)}</div><aside class="rr-player-info"><section class="rr-info-card"><h3>О записи</h3>${details(r)}</section><section class="rr-info-card"><h3>Материалы</h3>${materials(r).length ? materials(r).map(([name,ext,icon]) => `<div class="rr-material"><span class="rr-file-icon">${I(icon)}</span><div><strong>${name}</strong><small>${ext} · демонстрационный файл</small></div><button class="icon-button" data-action="rrDownload" data-index="${index}" data-file="${ext}" aria-label="Скачать: ${name}">${I('Download')}</button></div>`).join('') : '<p class="rr-muted">Материалы пока не готовы.</p>'}</section></aside></div></section>`, 'meetings', 'rr-content');
    },
    bind() {
      const seek = document.getElementById('rr-seek');
      if (seek) seek.oninput = event => { S().position = Number(event.target.value); playerButtons(); broadcast(); };
      startTimer();
    },
  };
  function recordIndex(button) {
    const value = button?.dataset.index ?? button?.dataset.record ?? button?.dataset.id ?? selected();
    const index = Number(value);
    return Number.isInteger(index) && records[index] ? index : -1;
  }
  async function copy(index) {
    if (!visible(index)) return;
    const url = new URL(location.href); url.searchParams.set('screen', 'recording'); url.searchParams.set('record', index);
    if (window.parent !== window) url.pathname = url.pathname.replace(/preview\.html$/, 'index.html');
    try { await navigator.clipboard.writeText(url.href); M.closeModal(); M.toast('Ссылка на запись скопирована'); }
    catch { M.modal('Ссылка на запись', `<div class="stack"><p>Ссылка открывает эту запись в локальном макете.</p><label class="field">Ссылка<input value="${E(url.href)}" readonly /></label><button class="button primary" data-close>Готово</button></div>`, () => document.querySelector('.modal input')?.select()); }
  }
  Object.assign(M.actions, {
    record(button) { const index = recordIndex(button); if (!visible(index)) { M.toast('Запись недоступна'); return; } stop(); M.state.record = index; S().position = 0; S().playing = false; M.go('recording'); },
    rrMaterialTab(button) { S().materialTab = button.dataset.value === 'summary' ? 'summary' : 'transcript'; M.render(); },
    rrSeek(button) { S().position = Math.max(0, Math.min(Number(button.dataset.time) || 0, records[selected()].seconds)); playerButtons(); broadcast(); document.getElementById('rr-seek')?.focus({ preventScroll: true }); M.toast(`Позиция записи: ${clock(S().position)}`); },
    rrStatus(button) { S().status = button.dataset.value === 'live' ? 'live' : 'past'; S().meeting = 'all'; M.render(); },
    rrView(button) { S().view = button.dataset.value === 'grid' ? 'grid' : 'list'; document.querySelectorAll('[data-action="rrView"]').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.value === S().view))); redrawResults(); },
    rrReset() { Object.assign(S(), { query: '', meeting: 'all', period: 'all' }); M.render(); },
    rrPlay() { S().playing = !S().playing; if (S().playing && S().position >= records[selected()].seconds) S().position = 0; playerButtons(); startTimer(); broadcast(); },
    rrCopy(button) { copy(recordIndex(button)); },
    rrMenu(button) {
      const index = recordIndex(button); if (!visible(index)) return;
      const r = records[index];
      M.modal('Действия с записью', `<div class="rr-action-sheet"><p>${E(r.title)}</p><button data-action="record" data-index="${index}">${I('CirclePlay')}Открыть запись</button><button data-action="rrCopy" data-index="${index}">${I('Link')}Скопировать ссылку</button>${!r.processing ? `<button data-action="rrDownload" data-index="${index}">${I('Download')}Скачать запись</button>` : ''}${canDelete(index) ? `<button class="rr-danger" data-action="rrDelete" data-index="${index}">${I('Trash2')}Удалить запись</button>` : ''}<small>Ссылка не меняет права доступа к встрече.</small></div>`);
    },
    rrDownload(button) {
      const index = recordIndex(button); if (!visible(index) || records[index].processing) return;
      M.modal('Скачать материалы', `<div class="stack"><p>В приложении здесь скачивается ${button.dataset.file === 'ZIP' || records[index].mode === 'tracks' ? 'архив дорожек' : 'медиафайл записи'}. В макете файл не подключён.</p><p class="rr-muted">Можно скачать сведения о демонстрационной записи в текстовом файле.</p><button class="button primary" data-action="rrDownloadInfo" data-index="${index}">${I('Download')}Скачать сведения · TXT</button><button class="button secondary" data-close>Закрыть</button></div>`);
    },
    rrDownloadInfo(button) {
      const index = recordIndex(button); if (!visible(index)) return;
      const r = records[index], blob = new Blob([`MeetSpace — демонстрационная запись\n${r.title}\n${r.date} · ${r.time}\n${labels[r.mode]} · ${clock(r.seconds)}\nОрганизатор: ${owner(r)}\nУчастников: ${r.people}\n\nМедиафайл не подключён. Это сведения о локальном макете.`], { type: 'text/plain;charset=utf-8' });
      const url = URL.createObjectURL(blob), link = document.createElement('a'); link.href = url; link.download = `MeetSpace-запись-${index + 1}.txt`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); M.closeModal(); M.toast('Сведения о записи скачаны');
    },
    rrDelete(button) {
      const index = recordIndex(button); if (!canDelete(index)) return;
      pendingDelete = index;
      M.modal('Удалить запись?', `<div class="stack"><strong>${E(records[index].title)}</strong><p>Запись и все её материалы будут удалены для всех участников. Восстановить их не получится.</p><p class="rr-muted">В демо удаляется только пример из локального списка.</p><button class="button secondary" data-close>Отмена</button><button class="button danger" data-action="rrDeleteConfirm" data-index="${index}">Удалить запись</button></div>`);
    },
    rrDeleteConfirm(button) { const index = recordIndex(button); if (index !== pendingDelete || !canDelete(index) || !document.querySelector('.modal [data-action="rrDeleteConfirm"]')) return; S().deleted.push(index); pendingDelete = null; S().playing = false; stop(); M.closeModal(); M.go('recordings'); M.toast('Демонстрационная запись удалена'); },
  });
})();
