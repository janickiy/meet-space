/* Personal folders: local prototype, scoped to the selected demo account. */
(() => {
  const M = window.MS, E = (v) => M.escape(v), I = (v) => M.icon(v);
  let stores = {};
  const account = () => M.accountManagerSnapshot?.().activeId || 'personal';
  const fresh = () => ({ selected: null, filter: 'all', query: '', folders: [
    { id: 'product', name: 'Команда продукта', items: ['chat:team', 'meeting:design', 'chat:anna', 'meeting:review'] },
    { id: 'release', name: 'Запуск продукта', items: ['chat:dev', 'meeting:release'] },
    { id: 'personal', name: 'Личное', items: ['chat:max'] },
  ] });
  const state = () => stores[account()] ||= fresh();
  const active = () => state().folders.find((f) => f.id === state().selected);
  const meetings = [
    ['design', 'Дизайн-синхронизация', '12 октября · 10:00', false],
    ['release', 'Обсуждение новой версии', '12 октября · 13:30', false],
    ['weekly', 'Планы на неделю', '13 октября · 11:00', false],
    ['room', 'Комната команды', 'Без даты', false],
    ['active', 'Рабочая встреча разработки', 'В эфире', false],
    ['demo', 'Демо новой версии', 'Завершена · 8 октября', true],
    ['cancelled', 'Обсуждение бюджета', 'Отменена · 7 октября', true],
    ['review', 'Продукт. Следующая глава', 'Завершена · 9 октября', true],
  ];
  const entries = () => [
    ...(M.chatContacts?.() || ['team', 'anna', 'dev', 'max', 'design'].map((id) => M.chatContact?.(id))).filter(Boolean).flatMap((c) => {
      const id = c.id;
      return c ? [{ key: `chat:${id}`, id, type: 'chat', name: c.name, detail: c.kind === 'group' ? 'Групповой чат' : 'Личный чат', icon: c.kind === 'group' ? 'Users' : 'MessageCircle' }] : [];
    }),
    ...meetings.map(([id, name, detail, past]) => ({ key: `meeting:${id}`, id, name, detail, past, type: 'meeting', icon: 'Video' })),
    ...(M.state.meetingItems || []).map((m) => ({ key: `meeting:${m.id}`, id: m.id, name: m.title, detail: 'Новая встреча', type: 'meeting', icon: 'Video' })),
  ];
  const entry = (key) => entries().find((item) => item.key === key);
  const quantity = (n, forms) => `${n} ${forms[n % 100 >= 11 && n % 100 <= 14 ? 2 : n % 10 === 1 ? 0 : n % 10 >= 2 && n % 10 <= 4 ? 1 : 2]}`;
  const count = (folder) => {
    const items = folder.items.map(entry).filter(Boolean), chats = items.filter((item) => item.type === 'chat').length;
    return `${quantity(items.length, ['элемент', 'элемента', 'элементов'])} · ${quantity(chats, ['чат', 'чата', 'чатов'])} · ${quantity(items.length - chats, ['встреча', 'встречи', 'встреч'])}`;
  };
  const formName = (folder, done) => M.modal(folder ? 'Переименовать папку' : 'Новая папка', `<form class="stack" id="folder-name-form"><label class="field">Название папки<input id="folder-name-input" name="name" value="${E(folder?.name || '')}" placeholder="Например, Проект Альфа" maxlength="50" required autocomplete="off"/></label><p class="field-error" id="folder-name-error" role="alert" hidden></p><button class="button primary">${folder ? 'Сохранить' : 'Создать папку'}</button></form>`, () => {
    document.getElementById('folder-name-form').onsubmit = (event) => {
      event.preventDefault();
      const name = document.getElementById('folder-name-input').value.trim().normalize('NFC');
      const error = !name || Array.from(name).length > 50 ? 'Введите название от 1 до 50 символов.' : state().folders.some((f) => f.id !== folder?.id && f.name.toLocaleLowerCase('ru') === name.toLocaleLowerCase('ru')) ? 'Папка с таким названием уже существует.' : '';
      const node = document.getElementById('folder-name-error'); node.hidden = !error; node.textContent = error; if (error) return;
      if (folder) folder.name = name;
      else state().folders.push({ id: `folder-${Date.now()}`, name, items: [] });
      M.closeModal(); M.render(); done?.();
    };
  });
  const itemRow = (item) => `<article class="pf-item"><button class="pf-item-link" data-action="folderOpenItem" data-item="${E(item.key)}"><span class="pf-item-icon ${item.type}">${I(item.icon)}</span><span><strong>${E(item.name)}</strong><small>${E(item.detail)}</small></span>${I('ChevronRight')}</button><button class="icon-button" data-action="folderRemoveItem" data-item="${E(item.key)}" aria-label="Убрать из папки: ${E(item.name)}">${I('FolderMinus')}</button></article>`;
  const filtered = () => (active()?.items || []).map(entry).filter(Boolean).filter((item) => (state().filter === 'all' || state().filter === item.type) && item.name.toLocaleLowerCase('ru').includes(state().query.trim().toLocaleLowerCase('ru')));
  const results = () => filtered().map(itemRow).join('') || `<div class="pf-empty">${I('Folder')}<h3>${state().query || state().filter !== 'all' ? 'Ничего не найдено' : 'В папке пока пусто'}</h3><p>${state().query || state().filter !== 'all' ? 'Измените поиск или выберите другой тип.' : 'Добавьте нужные встречи и чаты.'}</p></div>`;
  const list = () => `<div class="pf-heading"><div><span class="pf-eyebrow">ЛИЧНОЕ ПРОСТРАНСТВО</span><h2>Всё нужное — вместе</h2><p>Соберите встречи и чаты по проектам.</p></div><button class="button primary" data-action="folderCreate">${I('Plus')}<span>Новая папка</span></button></div><div class="pf-folder-grid">${state().folders.map((f) => `<article class="pf-folder"><button class="pf-folder-link" data-action="folderOpen" data-folder="${E(f.id)}"><span class="pf-folder-symbol">${I('Folder')}</span><strong>${E(f.name)}</strong><small>${count(f)}</small></button><button class="icon-button pf-folder-menu" data-action="folderMenu" data-folder="${E(f.id)}" aria-label="Действия с папкой ${E(f.name)}">${I('Ellipsis')}</button></article>`).join('') || '<div class="pf-empty"><h3>Создайте первую папку</h3><p>В ней можно собрать чаты и встречи.</p></div>'}</div><div class="pf-private-note">${I('LockKeyhole')}<p>Папки видны только вам. Чаты и встречи могут находиться сразу в нескольких папках.</p></div>`;
  const detail = (folder) => `<button class="text-action pf-back" data-action="folderList">${I('ArrowLeft')} Все папки</button><div class="pf-heading"><div><span class="pf-eyebrow">ВАША ПАПКА</span><h2>${E(folder.name)}</h2><p>${count(folder)}</p></div><div class="pf-heading-actions"><button class="button primary" data-action="folderAddItems">${I('Plus')} Добавить</button><button class="icon-button" data-action="folderMenu" data-folder="${E(folder.id)}" aria-label="Действия с папкой">${I('Ellipsis')}</button></div></div><div class="pf-toolbar"><div class="pf-tabs" role="group" aria-label="Тип элементов">${[['all','Все'],['meeting','Встречи'],['chat','Чаты']].map(([id,label]) => `<button data-action="folderFilter" data-filter="${id}" aria-pressed="${state().filter === id}">${label}</button>`).join('')}</div><label class="search-box">${I('Search')}<input type="search" id="folder-search" value="${E(state().query)}" placeholder="Найти в папке" aria-label="Найти в папке"/></label></div><div class="pf-items" id="folder-results" aria-live="polite">${results()}</div>`;
  M.screens.folders = {
    title: 'Папки', group: 'Рабочее пространство',
    render: () => M.shell(`<section class="pf-page">${active() ? detail(active()) : list()}</section>`, 'messages', 'pf-content'),
    bind: () => document.getElementById('folder-search')?.addEventListener('input', (event) => { state().query = event.target.value; document.getElementById('folder-results').innerHTML = results(); M.productStateChanged?.(); }),
  };
  M.openFolderAssignment = (key) => {
    const item = entry(key); if (!item) return;
    M.modal('Добавить в папку', `<form class="stack" id="folder-assignment"><p class="pf-modal-context">${E(item.name)}</p><div class="pf-picker">${state().folders.map((f) => `<label><span class="pf-item-icon">${I('Folder')}</span><span>${E(f.name)}</span><input type="checkbox" name="folders" value="${E(f.id)}" ${f.items.includes(key) ? 'checked' : ''}/></label>`).join('') || '<p>Создайте папку для этого элемента.</p>'}</div><button type="button" class="button secondary" id="folder-assignment-create">${I('Plus')} Новая папка</button><button class="button primary">Сохранить</button></form>`, () => {
      document.getElementById('folder-assignment-create').onclick = () => formName(null, () => M.openFolderAssignment(key));
      document.getElementById('folder-assignment').onsubmit = (event) => { event.preventDefault(); const ids = new FormData(event.target).getAll('folders'); state().folders.forEach((f) => { f.items = f.items.filter((id) => id !== key); if (ids.includes(f.id)) f.items.push(key); }); M.closeModal(); M.render(); M.toast('Папки обновлены'); };
    });
  };
  Object.assign(M.actions, {
    folders: () => { state().selected = null; M.go('folders'); },
    folderList: () => { state().selected = null; M.render(); },
    folderOpen: (b) => { state().selected = b.dataset.folder; state().filter = 'all'; state().query = ''; M.go('folders'); },
    folderCreate: () => formName(),
    folderFilter: (b) => { state().filter = b.dataset.filter; M.render(); },
    folderAssign: (b) => M.openFolderAssignment(b.dataset.item),
    folderOpenItem: (b) => { const item = entry(b.dataset.item); if (!item) return; if (item.type === 'chat') { M.state.chat = item.id; M.go('thread'); } else M.actions.meetingDetails({ dataset: { id: item.id } }); },
    folderRemoveItem: (b) => { if (!active()) return; active().items = active().items.filter((key) => key !== b.dataset.item); M.render(); M.toast('Элемент убран из папки'); },
    folderMenu: (b) => {
      const folder = state().folders.find((f) => f.id === b.dataset.folder); if (!folder) return;
      const index = state().folders.indexOf(folder);
      M.modal(E(folder.name), `<div class="stack"><button class="button secondary" id="folder-rename">${I('SquarePen')} Переименовать</button><button class="button secondary" id="folder-move-up" ${index === 0 ? 'disabled' : ''}>${I('ArrowUp')} Переместить выше</button><button class="button secondary" id="folder-move-down" ${index === state().folders.length - 1 ? 'disabled' : ''}>${I('ArrowDown')} Переместить ниже</button><button class="button secondary pf-danger" id="folder-delete">${I('Trash2')} Удалить папку</button></div>`, () => {
        document.getElementById('folder-rename').onclick = () => formName(folder);
        const move = (step) => { const next = index + step; if (next < 0 || next >= state().folders.length) return; [state().folders[index], state().folders[next]] = [state().folders[next], state().folders[index]]; M.closeModal(); M.render(); };
        document.getElementById('folder-move-up').onclick = () => move(-1); document.getElementById('folder-move-down').onclick = () => move(1);
        document.getElementById('folder-delete').onclick = () => M.modal('Удалить папку?', `<div class="stack"><p>Папка «${E(folder.name)}» будет удалена. Чаты, встречи и сообщения сохранятся.</p><button class="button pf-delete-button" id="folder-delete-confirm">Удалить папку</button><button class="button secondary" data-close>Отмена</button></div>`, () => { document.getElementById('folder-delete-confirm').onclick = () => { state().folders = state().folders.filter((f) => f.id !== folder.id); if (state().selected === folder.id) state().selected = null; M.closeModal(); M.render(); }; });
      });
    },
    folderAddItems: () => {
      const folder = active(); if (!folder) return;
      M.modal('Добавить в папку', `<form class="stack" id="folder-items-form"><p class="pf-modal-context">${E(folder.name)}</p><div class="pf-picker">${entries().map((item) => `<label><span class="pf-item-icon ${item.type}">${I(item.icon)}</span><span><strong>${E(item.name)}</strong><small>${E(item.detail)}</small></span><input type="checkbox" name="items" value="${E(item.key)}" ${folder.items.includes(item.key) ? 'checked' : ''}/></label>`).join('')}</div><button class="button primary">Сохранить выбор</button></form>`, () => { document.getElementById('folder-items-form').onsubmit = (event) => { event.preventDefault(); folder.items = new FormData(event.target).getAll('items').map(String); M.closeModal(); M.render(); M.toast('Содержимое папки обновлено'); }; });
    },
  });
  M.folderSnapshot = () => JSON.parse(JSON.stringify(stores));
  M.setFolderSnapshot = (value) => { if (!value || typeof value !== 'object' || Array.isArray(value)) return false; const valid = Object.values(value).every((v) => v && Array.isArray(v.folders) && v.folders.length <= 100 && ['all','meeting','chat'].includes(v.filter) && typeof v.query === 'string' && (v.selected === null || typeof v.selected === 'string') && v.folders.every((f) => typeof f.id === 'string' && typeof f.name === 'string' && f.name.length <= 100 && Array.isArray(f.items) && f.items.every((key) => typeof key === 'string'))); if (!valid || JSON.stringify(value) === JSON.stringify(stores)) return false; stores = JSON.parse(JSON.stringify(value)); return true; };
})();
