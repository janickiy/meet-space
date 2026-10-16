/* Chat interactions in the design prototype. No network or persistent file storage. */
(() => {
  'use strict';
  const M = window.MS, E = value => M.escape(value ?? ''), I = name => M.icon(name);
  const copy = value => JSON.parse(JSON.stringify(value));
  let threads = {}, focused = null, forwarding = null;
  const reactions = [['👍','Нравится'],['❤️','Любовь'],['😂','Смешно'],['😮','Удивление'],['😢','Грусть'],['🙏','Спасибо']];
  const reactionOf = item => reactions.some(([emoji]) => emoji === item.reaction) ? item.reaction : item.reaction === true ? '👍' : '';
  const files = new Map();
  const scope = (id = M.state.chat) => `${M.accountManagerSnapshot?.().activeId || 'personal'}:${id}`;
  const contact = () => M.chatContact(M.state.chat);
  const initials = name => name.split(/\s+/).filter(Boolean).slice(0, 2).map(word => word[0]).join('');
  const uid = () => 'msg-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 7);
  const author = message => message.own ? 'Вы' : message.author;
  const preview = message => message.deleted ? 'Сообщение удалено' : message.text || message.attachments?.[0]?.name || 'Вложение';
  const memberCount = count => `${count} ${count % 100 >= 11 && count % 100 <= 14 ? 'участников' : count % 10 === 1 ? 'участник' : count % 10 >= 2 && count % 10 <= 4 ? 'участника' : 'участников'}`;
  const formatSize = size => size >= 1048576 ? `${(size / 1048576).toLocaleString('ru', {maximumFractionDigits: 1})} МБ` : `${Math.max(1, Math.round(size / 1024))} КБ`;
  function data() {
    const key = scope(), c = contact();
    if (!threads[key]) {
      const group = c?.kind === 'group';
      const people = {anna: 'Анна Кузнецова', maria: 'Мария Полякова', max: 'Максим Петров', dmitry: 'Дмитрий Волков'};
      const first = group ? people[c.members.find(id => id !== 'self')] || 'Участник группы' : c?.name || 'Собеседник';
      const second = group ? people[c.members.filter(id => id !== 'self')[1]] || first : first;
      threads[key] = {
        messages: c?.created ? [] : [
          {id: 'm1', author: first, initials: initials(first), tone: group ? 'mint' : c?.tone, text: 'Доброе утро! Материалы к встрече готовы. Посмотрите, когда будет минутка.', time: '09:32', attachments: [{id: 'demo-pdf', name: 'Обзор продукта.pdf', size: 2516582, demo: true}], reaction: false},
          {id: 'm2', own: true, text: 'Спасибо! Посмотрю перед встречей. Есть пара идей по навигации.', time: '09:34', attachments: []},
          {id: 'm3', author: second, initials: initials(second), tone: 'orange', text: 'Встречаемся в 10:00. Обсудим новую версию и планы команды.', time: '09:38', attachments: []},
        ],
        muted: false, hidden: false, pins: c?.created ? [] : ['m3'], draft: '', pending: [], reply: null, editing: null, editText: ''
      };
    }
    return threads[key];
  }
  const message = id => data().messages.find(item => item.id === id);
  const pins = () => data().pins.map(message).filter(item => item && !item.deleted);
  const avatar = (name, tone = '') => `<span class="avatar ${E(tone)}">${E(name)}</span>`;
  const jumpButton = (item, cls = '') => `<button class="${cls}" data-action="chatRefreshJump" data-id="${E(item.id)}"><strong>${E(author(item))}</strong><span>${E(preview(item))}</span></button>`;
  function quote(item) {
    if (!item.reply) return '';
    const original = message(item.reply);
    return original ? jumpButton(original, 'cr-quote') : '<div class="cr-quote">Сообщение удалено</div>';
  }
  function attachment(file, pending = false) {
    return `<div class="cr-attachment">${I('FileText')}<span><strong>${E(file.name)}</strong><small>${formatSize(file.size)}</small></span><button type="button" class="icon-button" data-action="${pending ? 'chatRefreshRemoveFile' : 'chatRefreshFile'}" data-id="${E(file.id)}" aria-label="${pending ? 'Убрать' : 'Открыть'} ${E(file.name)}">${I(pending ? 'X' : 'Download')}</button></div>`;
  }
  function renderMessage(item) {
    const pinned = data().pins.includes(item.id) && !item.deleted;
    return `<article class="chat-message cr-message ${item.own ? 'mine' : ''} ${focused?.key === scope() && focused.id === item.id ? 'cr-focused' : ''}" id="cr-${E(item.id)}" data-message-id="${E(item.id)}" tabindex="${item.deleted ? '-1' : '0'}" aria-label="Сообщение от ${E(author(item))} в ${E(item.time)}" ${item.deleted ? '' : 'aria-haspopup="dialog"'}>${item.own ? '' : avatar(item.initials, item.tone)}<div class="cr-message-content"><div class="bubble">${item.forwarded ? `<span class="cr-forwarded-label">${I('ArrowRight')}Переслано от ${E(item.forwarded)}</span>` : ''}${item.own ? '' : `<h4>${E(author(item))}</h4>`}${quote(item)}<div class="cr-message-text ${item.deleted ? 'muted' : ''}">${E(item.deleted ? 'Сообщение удалено' : item.text)}</div>${item.deleted ? '' : (item.attachments || []).map(file => attachment(file)).join('')}<footer class="cr-meta">${pinned ? `<span aria-label="Закреплено">${I('Pin')}</span>` : ''}${item.edited && !item.deleted ? '<span>изменено</span>' : ''}<time>${E(item.time)}</time>${item.own ? `<span aria-label="Отправлено">${I('CheckCheck')}</span>` : ''}</footer></div>${!item.deleted ? `<div class="cr-message-tools">${reactionOf(item) ? `<button class="chat-reaction" data-action="chatRefreshReaction" data-id="${E(item.id)}" aria-label="Убрать реакцию ${E(reactionOf(item))}" aria-pressed="true">${E(reactionOf(item))} <span>1</span></button>` : ''}<button class="icon-button cr-more" data-action="chatRefreshMenu" data-id="${E(item.id)}" aria-haspopup="dialog" aria-label="Действия с сообщением от ${E(author(item))} в ${E(item.time)}">${I('Ellipsis')}</button></div>` : ''}</div></article>`;
  }
  function pinBar() {
    const list = pins();
    return list.length ? `<div class="cr-pinned-bar"><button class="cr-pinned-jump" data-action="chatRefreshJump" data-id="${E(list[0].id)}">${I('Pin')}<span><strong>Закреплённое сообщение</strong><small>${E(preview(list[0]))}</small></span></button><button class="icon-button cr-pin-count" data-action="chatRefreshPins" aria-label="Закреплённые сообщения: ${list.length}">${list.length}${I('ChevronRight')}</button></div>` : '';
  }
  function composerContext() {
    const d = data(), target = message(d.editing || d.reply);
    return target ? `<div class="cr-composer-context">${I(d.editing ? 'Pencil' : 'Reply')}<span><strong>${d.editing ? 'Редактирование сообщения' : `Ответ · ${E(author(target))}`}</strong><small>${E(preview(target))}</small></span><button class="icon-button" data-action="chatRefreshCancel" aria-label="${d.editing ? 'Отменить редактирование' : 'Отменить ответ'}">${I('X')}</button></div>` : '';
  }
  M.chatIsVisible = c => !threads[scope(c.id)]?.hidden && !threads[scope(c.id)]?.left;
  M.chatIsMuted = c => Boolean(threads[scope(c.id)]?.muted);
  M.renderChatThread = c => {
    if (!M.chatIsVisible(c)) return `<section class="thread cr-thread cr-chat-empty"><span>${I('MessagesSquare')}</span><h3>Выберите чат</h3><p>Откройте переписку из списка или начните новую.</p><button class="button primary" data-action="newChat">${I('Plus')}Новый чат</button></section>`;
    const d = data(), group = c.kind === 'group';
    return `<section class="thread cr-thread"><header class="thread-header"><button class="icon-button phone-back" data-go="messages" aria-label="Назад к чатам">${I('ArrowLeft')}</button><span class="chat-avatar ${group ? 'is-group' : ''}">${avatar(c.initials, c.tone)}${group ? `<span class="group-avatar-badge" aria-label="Группа">${I('Users')}</span>` : ''}</span><div class="cr-contact-copy"><h3>${E(c.name)}${d.muted ? `<span class="cr-muted-indicator" aria-label="Уведомления выключены">${I('BellOff')}</span>` : ''}</h3><p>${group ? `Группа · ${memberCount(c.members.length)}` : 'В сети'}</p></div><span class="spacer"></span>${!group ? `<button class="icon-button personal-call-trigger" data-action="startPersonalCall" data-contact="${E(c.id)}" aria-label="Позвонить: ${E(c.name)}">${I('Phone')}</button>` : ''}<button class="icon-button" data-action="chatRefreshSearch" aria-label="Поиск в переписке">${I('Search')}</button><div class="cr-header-actions"><button class="icon-button cr-header-menu-trigger" data-action="chatRefreshHeader" aria-label="Действия с чатом" aria-haspopup="menu" aria-expanded="false" aria-controls="cr-chat-dropdown">${I('Ellipsis')}</button></div></header>${M.renderPersonalCallBanner?.(c) || ''}${pinBar()}<div class="thread-log" id="thread-log" aria-label="Сообщения"><div class="chat-date">${c.created ? 'Вы создали группу' : 'Сегодня, 16 октября'}</div>${d.messages.some(item => !item.deleted) ? '<p class="cr-gesture-hint">Удерживайте сообщение, чтобы открыть действия</p>' : ''}${d.messages.map(renderMessage).join('') || `<div class="group-welcome">${avatar(c.initials, c.tone)}<h3>${E(c.name)}</h3><p>Напишите первое сообщение.</p></div>`}</div>${focused?.key === scope() ? '<div class="cr-context-return"><button data-action="chatRefreshLatest">К последним сообщениям ↓</button></div>' : ''}<div class="thread-create-actions"><button class="button primary" type="button" data-action="newChat" aria-label="Новый чат">${I('Plus')}Новый чат</button></div><div class="cr-composer-shell">${composerContext()}${!d.editing && d.pending.length ? `<div class="cr-pending-files">${d.pending.map(file => attachment(file, true)).join('')}</div>` : ''}<form class="composer" id="message-form"><button class="icon-button" type="button" data-action="chatRefreshAttach" aria-label="Прикрепить файлы" ${d.editing ? 'disabled' : ''}>${I('Paperclip')}</button><textarea id="message-input" rows="1" maxlength="10000" autocomplete="off" aria-label="${d.editing ? 'Редактировать сообщение' : 'Сообщение'}" placeholder="Написать сообщение…">${E(d.editing ? d.editText : d.draft)}</textarea><button class="icon-button send-button" aria-label="${d.editing ? 'Сохранить сообщение' : 'Отправить сообщение'}" ${canSend() ? '' : 'disabled'}>${I(d.editing ? 'Check' : 'ArrowUp')}</button></form></div></section>`;
  };
  function canSend() {
    const d = data();
    return d.editing ? Boolean(d.editText.trim()) : Boolean(d.draft.trim() || d.pending.length);
  }
  function resize() {
    const input = document.getElementById('message-input');
    if (!input) return;
    input.style.height = 'auto';
    input.style.height = Math.min(116, Math.max(42, input.scrollHeight)) + 'px';
    document.querySelector('#message-form .send-button').disabled = !canSend();
  }
  function redraw({focus = false, latest = false} = {}) {
    const position = document.getElementById('thread-log')?.scrollTop;
    M.render();
    const log = document.getElementById('thread-log');
    if (log) log.scrollTop = latest ? log.scrollHeight : position || 0;
    if (focus) document.getElementById('message-input')?.focus();
  }
  function updatePreview() { data().updated = true; }
  M.chatPreview = c => {
    const thread = threads[`${M.accountManagerSnapshot?.().activeId || 'personal'}:${c.id}`];
    if (!thread?.updated) return null;
    const last = [...thread.messages].reverse().find(item => !item.deleted);
    return {text: last ? `${author(last)}: ${preview(last)}` : 'Пока нет сообщений', time: 'Сейчас'};
  };
  M.bindChatThread = () => {
    const input = document.getElementById('message-input'), form = document.getElementById('message-form');
    if (!input || !form) return;
    input.addEventListener('input', () => {
      const d = data();
      if (d.editing) d.editText = input.value; else d.draft = input.value;
      resize();
      M.productStateChanged?.();
    });
    input.addEventListener('keydown', event => {
      if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {event.preventDefault(); form.requestSubmit();}
    });
    form.addEventListener('submit', event => {
      event.preventDefault(); if (!canSend()) return;
      const d = data();
      if (d.editing) {
        const item = message(d.editing);
        if (!item?.own || item.deleted) return;
        item.text = d.editText.trim(); item.edited = true; d.editing = null; d.editText = '';
      } else {
        d.messages.push({id: uid(), own: true, text: d.draft.trim(), time: new Date().toLocaleTimeString('ru', {hour: '2-digit', minute: '2-digit'}), attachments: d.pending, reply: d.reply});
        d.draft = ''; d.pending = []; d.reply = null;
      }
      focused = null; updatePreview(); redraw({focus: true, latest: true});
    });
    bindMessageGestures();
    resize();
  };
  function jump(id) {
    if (!message(id)) return;
    focused = {key: scope(), id}; M.closeModal(); M.render();
    const element = document.getElementById('cr-' + id);
    element?.scrollIntoView({block: 'center', behavior: 'auto'});
    element?.focus({preventScroll: true});
  }
  function togglePin(id, inModal = false) {
    const item = message(id); if (!item || item.deleted) return;
    const d = data(), pinned = d.pins.includes(id);
    if (!pinned && d.pins.length >= 20) {M.toast('Можно закрепить до 20 сообщений'); return;}
    d.pins = pinned ? d.pins.filter(value => value !== id) : [id, ...d.pins];
    M.closeModal(); redraw(); if (inModal) showPins();
    M.toast(pinned ? 'Сообщение откреплено' : 'Сообщение закреплено');
  }
  function showPins() {
    M.modal('Закреплённые сообщения', `<div class="cr-pin-list">${pins().map(item => `<div>${jumpButton(item, 'cr-pin-result')}<button class="icon-button" data-action="chatRefreshUnpin" data-id="${E(item.id)}" aria-label="Открепить сообщение">${I('X')}</button></div>`).join('') || '<p>В этом чате пока нет закреплённых сообщений.</p>'}</div>`);
  }
  function bindMessageGestures() {
    const log=document.getElementById('thread-log'); if(!log)return;
    let hold=null, origin=null, suppressClick=false;
    const cancel=()=>{clearTimeout(hold);hold=null;origin=null;};
    log.addEventListener('pointerdown',event=>{
      cancel();suppressClick=false;
      const article=event.target.closest('.cr-message');
      if(event.button!==0||!article||event.target.closest('button,a,input')||!message(article.dataset.messageId)||message(article.dataset.messageId).deleted)return;
      origin={x:event.clientX,y:event.clientY};
      const key=scope();
      hold=setTimeout(()=>{hold=null;origin=null;if(!article.isConnected||scope()!==key)return;suppressClick=true;openMenu(article.dataset.messageId);},500);
    });
    log.addEventListener('pointermove',event=>{if(origin&&Math.hypot(event.clientX-origin.x,event.clientY-origin.y)>10)cancel();});
    ['pointerup','pointercancel','pointerleave'].forEach(type=>log.addEventListener(type,cancel));
    log.addEventListener('scroll',cancel,{passive:true});
    log.addEventListener('click',event=>{if(suppressClick){event.preventDefault();event.stopPropagation();suppressClick=false;}},true);
    log.addEventListener('contextmenu',event=>{
      const article=event.target.closest('.cr-message');if(!article||event.target.closest('button,a'))return;
      if(message(article.dataset.messageId)?.deleted)return;
      event.preventDefault();cancel();openMenu(article.dataset.messageId);
    });
    log.addEventListener('keydown',event=>{
      if(!event.target.matches('.cr-message'))return;
      if(event.key==='Enter'||event.key===' '||event.key==='ContextMenu'||event.key==='F10'&&event.shiftKey){event.preventDefault();openMenu(event.target.dataset.messageId);}
    });
  }
  function openMenu(id) {
    const item = message(id); if (!item || item.deleted) return;
    const article=document.getElementById('cr-'+id), anchor=article?.querySelector('.bubble')?.getBoundingClientRect();
    if(!anchor)return;
    const options = [['Reply','chatRefreshReply','Ответить'],['Copy','chatRefreshCopy','Копировать'],['ArrowRight','chatRefreshForward','Переслать'],['Pin','chatRefreshPin',data().pins.includes(id)?'Открепить':'Закрепить']];
    if(item.own)options.push(['Pencil','chatRefreshEdit','Изменить'],['Trash2','chatRefreshDelete','Удалить']);
    M.closeModal();M.opener=article;
    document.getElementById('dialog-root').innerHTML=`<div class="modal-backdrop cr-message-backdrop"><section class="modal cr-message-popover" role="dialog" aria-modal="true" aria-labelledby="cr-message-title"><div class="cr-context-caption"><h2 id="cr-message-title">Действия с сообщением</h2><button class="icon-button" data-close aria-label="Закрыть действия с сообщением">${I('X')}</button></div><div class="cr-quick-reactions" role="group" aria-label="Быстрые реакции">${reactions.map(([emoji,label])=>`<button data-action="chatRefreshReaction" data-id="${E(id)}" data-emoji="${E(emoji)}" aria-label="${E(label)} ${emoji}" aria-pressed="${reactionOf(item)===emoji}">${emoji}</button>`).join('')}</div><div class="cr-selected-preview ${item.own?'mine':''}"><strong>${E(author(item))}</strong><p>${E(item.text)}</p>${item.attachments?.length?`<span class="cr-preview-attachment">${I('Paperclip')}${E(item.attachments[0].name)}${item.attachments.length>1?` · ещё ${item.attachments.length-1}`:''}</span>`:''}<small>${item.edited?'изменено · ':''}${E(item.time)}${item.own?' · ✓✓':''}</small></div><div class="cr-context-actions">${options.map(([icon,action,label])=>`<button class="${action==='chatRefreshDelete'?'cr-danger':''}" data-action="${action}" data-id="${E(id)}">${I(icon)}<span>${label}</span></button>`).join('')}</div></section></div>`;
    document.getElementById('app').inert=true;
    const popover=document.querySelector('.cr-message-popover'), backdrop=document.querySelector('.cr-message-backdrop');
    const width=Math.min(320,innerWidth-32);popover.style.width=width+'px';
    const thread=document.querySelector('.cr-thread')?.getBoundingClientRect();
    const minLeft=Math.max(16,(thread?.left||0)+12), maxLeft=innerWidth-width-16;
    const left=Math.min(maxLeft,Math.max(Math.min(minLeft,maxLeft),item.own?anchor.right-width:anchor.left));
    const height=popover.getBoundingClientRect().height;
    popover.style.left=left+'px';popover.style.top=Math.max(16,Math.min(anchor.top-68,innerHeight-height-16))+'px';
    const openedAt=performance.now();
    backdrop.addEventListener('click',event=>{if(event.target===backdrop&&performance.now()-openedAt>300)M.closeModal();});
    popover.querySelector('.cr-context-actions button')?.focus({preventScroll:true});
  }
  function showForward(id) {
    const item=message(id);if(!item||item.deleted)return;
    forwarding={key:scope(),id};
    const recipients=(M.chatContacts?.()||[]).filter(M.chatIsVisible);
    M.modal('Переслать сообщение',`<div class="stack"><label class="field">Получатель<input id="cr-forward-search" type="search" placeholder="Найти чат или группу" autocomplete="off"/></label><div class="cr-menu-preview"><strong>${E(author(item))}</strong><p>${E(preview(item))}</p></div><div class="cr-forward-list">${recipients.map(c=>`<button class="cr-forward-target" data-action="chatRefreshForwardTo" data-chat="${E(c.id)}" data-search="${E(c.name.toLocaleLowerCase('ru'))}">${avatar(c.initials,c.tone)}<span><strong>${E(c.name)}</strong><small>${c.kind==='group'?'Групповой чат':'Личный чат'}</small></span>${I('ChevronRight')}</button>`).join('')}</div><p id="cr-forward-empty" class="muted" hidden>Чат не найден</p></div>`,()=>{
      document.getElementById('cr-forward-search').oninput=event=>{
        const q=event.target.value.trim().toLocaleLowerCase('ru');let count=0;
        document.querySelectorAll('.cr-forward-target').forEach(button=>{button.hidden=!button.dataset.search.includes(q);if(!button.hidden)count++;});
        document.getElementById('cr-forward-empty').hidden=Boolean(count);
      };
    });
  }
  window.addEventListener('resize',()=>{if(document.querySelector('.cr-message-popover'))M.closeModal();});
  function forwardTo(button) {
    if(!forwarding||forwarding.key!==scope())return;
    const item=message(forwarding.id), recipient=M.chatContact(button.dataset.chat);
    if(!item||item.deleted||!recipient||!M.chatIsVisible(recipient))return;
    const forwarded={id:uid(),own:true,text:item.text,attachments:copy(item.attachments||[]),forwarded:item.forwarded||(item.own?M.state.profileName||'вас':author(item)),time:new Date().toLocaleTimeString('ru',{hour:'2-digit',minute:'2-digit'})};
    M.state.chat=recipient.id;data().messages.push(forwarded);updatePreview();forwarding=null;focused=null;M.go('thread');
    const log=document.getElementById('thread-log');if(log)log.scrollTop=log.scrollHeight;
    M.toast('Сообщение переслано');
  }
  function searchResults(query) {
    query = query.trim().toLocaleLowerCase('ru');
    if (!query) return '<p class="cr-search-hint">Поиск по тексту сообщений и названиям файлов.</p>';
    const found = data().messages.filter(item => !item.deleted && [item.text, ...(item.attachments || []).map(file => file.name)].join(' ').toLocaleLowerCase('ru').includes(query));
    return `<p class="cr-search-hint">${found.length ? `Найдено: ${found.length}` : 'Ничего не найдено. Попробуйте другой запрос.'}</p>${found.map(item => `<button class="cr-search-result" data-action="chatRefreshJump" data-id="${E(item.id)}"><span><strong>${E(author(item))}</strong><time>${E(item.time)}</time></span><p>${E(preview(item))}</p></button>`).join('')}`;
  }
  function closeHeaderMenu(restoreFocus = false) {
    const menu=document.getElementById('cr-chat-dropdown'); if (!menu) return;
    menu.remove();
    const trigger=document.querySelector('.cr-header-menu-trigger');
    trigger?.setAttribute('aria-expanded','false');
    if (restoreFocus) trigger?.focus();
  }
  function openHeaderMenu() {
    if (document.getElementById('cr-chat-dropdown')) {closeHeaderMenu(true); return;}
    const wrapper=document.querySelector('.cr-header-actions'); if (!wrapper) return;
    const options=contact()?.kind === 'group' ? [
      ['Info','chatInfo','Информация'],
      ['FolderPlus','folderAssign','Добавить в папку'],
      ['LogOut','chatRefreshLeaveGroup','Покинуть группу']
    ] : [
      ['Info','chatInfo','Информация'],
      [data().muted ? 'Bell' : 'BellOff','chatRefreshMute',data().muted ? 'Включить уведомления' : 'Без уведомлений'],
      ['FolderPlus','folderAssign','Добавить в папку'],
      ['Eraser','chatRefreshClearHistory','Очистить историю'],
      ['Trash2','chatRefreshRemoveChat','Удалить чат']
    ];
    wrapper.insertAdjacentHTML('beforeend',`<div id="cr-chat-dropdown" class="cr-chat-dropdown" role="menu" aria-label="Действия с чатом">${options.map(([icon,action,label])=>`<button type="button" role="menuitem" tabindex="-1" class="${['chatRefreshRemoveChat','chatRefreshLeaveGroup'].includes(action)?'cr-danger':''}" data-action="${action}" ${action==='folderAssign'?`data-item="chat:${E(M.state.chat)}"`:''}>${I(icon)}<span>${label}</span></button>`).join('')}</div>`);
    wrapper.querySelector('.cr-header-menu-trigger').setAttribute('aria-expanded','true');
    wrapper.querySelector('[role="menuitem"]').focus();
  }
  document.addEventListener('click', event => {
    if (event.target.closest('.cr-chat-dropdown [role="menuitem"]')) closeHeaderMenu(true);
    else if (!event.target.closest('.cr-header-actions')) closeHeaderMenu();
  },true);
  document.addEventListener('keydown', event => {
    const menu=document.getElementById('cr-chat-dropdown');
    if (!menu) {
      if (event.key==='ArrowDown' && event.target.matches('.cr-header-menu-trigger')) {event.preventDefault();openHeaderMenu();}
      return;
    }
    if (event.key==='Escape') {event.preventDefault();closeHeaderMenu(true);return;}
    if (event.key==='Tab') {closeHeaderMenu(true);return;}
    const items=[...menu.querySelectorAll('[role="menuitem"]')], index=items.indexOf(document.activeElement);
    if (['ArrowDown','ArrowUp','Home','End'].includes(event.key)) {
      event.preventDefault();
      const next=event.key==='Home'?0:event.key==='End'?items.length-1:(index+(event.key==='ArrowDown'?1:-1)+items.length)%items.length;
      items[next]?.focus();
    }
  });
  function confirmChatRemoval(removeChat) {
    const key=scope(), c=contact();
    M.modal(removeChat?'Удалить чат?':'Очистить историю?',`<div class="stack"><p>${removeChat?`Чат «${E(c.name)}» исчезнет из вашего списка, а его история будет очищена.`:`История чата «${E(c.name)}» будет очищена только у вас.`} У собеседников сообщения сохранятся.</p><p class="muted">Действие выполняется только в демо-макете.</p><button id="cr-chat-remove-confirm" class="button ${removeChat?'cr-delete-button':'primary'}">${removeChat?'Удалить чат':'Очистить историю'}</button><button class="button secondary" data-close>Отмена</button></div>`,()=>{
      document.getElementById('cr-chat-remove-confirm').onclick=()=>{
        if (scope()!==key) return;
        const d=data(); Object.assign(d,{messages:[],pins:[],draft:'',pending:[],reply:null,editing:null,editText:'',updated:true,hidden:removeChat});
        focused=null; M.closeModal();
        if (removeChat) {const next=(M.chatContacts?.()||[]).find(M.chatIsVisible); if(next)M.state.chat=next.id;M.go('messages');}
        else redraw();
        M.toast(removeChat?'Чат удалён из списка':'История чата очищена');
      };
    });
  }
  function confirmLeaveGroup() {
    const c=contact(), key=scope(); if(c?.kind!=='group'||data().left)return;
    M.modal('Покинуть группу?',`<div class="stack"><p>Вы выйдете из группы «${E(c.name)}» и перестанете получать новые сообщения. Чтобы вернуться, потребуется приглашение участника группы.</p><p class="muted">Действие выполняется только в демо-макете.</p><button id="cr-leave-group-confirm" class="button cr-delete-button">Покинуть группу</button><button class="button secondary" data-close>Отмена</button></div>`,()=>{
      document.getElementById('cr-leave-group-confirm').onclick=()=>{
        if(scope()!==key||contact()?.kind!=='group')return;
        Object.assign(data(),{left:true,hidden:true,draft:'',pending:[],reply:null,editing:null,editText:''});
        focused=null; M.closeModal();
        const next=(M.chatContacts?.()||[]).find(M.chatIsVisible);if(next)M.state.chat=next.id;
        M.go('messages');M.toast('Вы покинули группу');
      };
    });
  }
  const openChat=M.actions.chat;
  M.actions.chat=button=>{const d=threads[scope(button.dataset.chat)];if(d?.left){M.toast('Вы покинули эту группу. Для возвращения нужно приглашение.');return;}if(d)d.hidden=false;openChat(button);};
  Object.assign(M.actions, {
    chatRefreshHeader: openHeaderMenu,
    chatRefreshMute: () => {const d=data(); d.muted=!d.muted; redraw(); M.toast(d.muted ? 'Уведомления чата выключены' : 'Уведомления чата включены');},
    chatRefreshClearHistory: () => confirmChatRemoval(false),
    chatRefreshRemoveChat: () => confirmChatRemoval(true),
    chatRefreshLeaveGroup: confirmLeaveGroup,
    chatRefreshMenu: button => openMenu(button.dataset.id),
    chatRefreshReply: button => {const d = data(); if (!message(button.dataset.id) || message(button.dataset.id).deleted) return; d.reply = button.dataset.id; d.editing = null; d.editText = ''; M.closeModal(); redraw({focus: true});},
    chatRefreshEdit: button => {const item = message(button.dataset.id); if (!item?.own || item.deleted) return; const d = data(); d.editing = item.id; d.editText = item.text; d.reply = null; M.closeModal(); redraw({focus: true});},
    chatRefreshCancel: () => {data().reply = null; data().editing = null; data().editText = ''; redraw({focus: true});},
    chatRefreshReaction: button => {const item=message(button.dataset.id);if(!item||item.deleted)return;const emoji=button.dataset.emoji||reactionOf(item)||'👍';if(!reactions.some(([value])=>value===emoji))return;item.reaction=reactionOf(item)===emoji?false:emoji;M.closeModal();redraw();},
    chatRefreshForward: button => showForward(button.dataset.id),
    chatRefreshForwardTo: forwardTo,
    chatRefreshPin: button => togglePin(button.dataset.id),
    chatRefreshUnpin: button => togglePin(button.dataset.id, true),
    chatRefreshPins: showPins,
    chatRefreshJump: button => jump(button.dataset.id),
    chatRefreshLatest: () => {focused = null; redraw({latest: true});},
    chatRefreshSearch: () => M.modal('Поиск в переписке', `<label class="field">${E(contact()?.name)}<input id="cr-search" type="search" maxlength="200" placeholder="Найти сообщение или файл" autocomplete="off"/></label><div id="cr-search-results" class="cr-search-results" aria-live="polite">${searchResults('')}</div>`, () => {const input = document.getElementById('cr-search'); input.oninput = () => document.getElementById('cr-search-results').innerHTML = searchResults(input.value); input.focus();}),
    chatRefreshCopy: async button => {const item = message(button.dataset.id); if (!item || item.deleted) return; const text = item.text || preview(item); M.closeModal(); try {await navigator.clipboard.writeText(text); M.toast('Текст скопирован');} catch {M.modal('Текст сообщения', `<label class="field">Скопируйте текст<textarea readonly rows="4">${E(text)}</textarea></label>`);}},
    chatRefreshDelete: button => {
      const item = message(button.dataset.id), key = scope(); if (!item?.own || item.deleted) return;
      M.modal('Удалить сообщение?', `<div class="stack"><p>Сообщение исчезнет у участников чата. Это действие нельзя отменить.</p><div class="cr-menu-preview">${E(preview(item))}</div><button id="cr-delete-confirm" class="button cr-delete-button">Удалить сообщение</button><button class="button secondary" data-close>Отмена</button></div>`, () => document.getElementById('cr-delete-confirm').onclick = () => {
        if (scope() !== key) return;
        const d = data(); item.deleted = true; item.text = ''; item.attachments = []; item.reaction = false; d.pins = d.pins.filter(id => id !== item.id);
        if (d.reply === item.id) d.reply = null; if (d.editing === item.id) {d.editing = null; d.editText = '';}
        updatePreview(); M.closeModal(); redraw(); M.toast('Сообщение удалено');
      });
    },
    chatRefreshAttach: () => {
      const key = scope();
      M.modal('Прикрепить файлы', `<div class="stack"><label class="cr-file-picker">${I('Upload')}<strong>Выберите файлы</strong><span>Документы, изображения и видео</span><input id="cr-file-picker" type="file" multiple/></label><p class="muted">Файлы появятся в сообщении после отправки.</p><p id="cr-files-summary" aria-live="polite"></p><button id="cr-files-add" class="button primary" disabled>Добавить к сообщению</button></div>`, () => {
        let selected = [];
        document.getElementById('cr-file-picker').onchange = event => {selected = [...event.target.files].slice(0, 10); document.getElementById('cr-files-summary').textContent = selected.map(file => file.name).join(', '); document.getElementById('cr-files-add').disabled = !selected.length;};
        document.getElementById('cr-files-add').onclick = () => {if (key !== scope()) return; const d = data(); for (const file of selected) {const id = uid(); files.set(id, file); d.pending.push({id, name: file.name, size: file.size});} M.closeModal(); redraw({focus: true});};
      });
    },
    chatRefreshRemoveFile: button => {data().pending = data().pending.filter(file => file.id !== button.dataset.id); files.delete(button.dataset.id); redraw({focus: true});},
    chatRefreshFile: button => {
      const file = data().messages.filter(item => !item.deleted).flatMap(item => item.attachments || []).find(item => item.id === button.dataset.id); if (!file) return;
      M.modal(E(file.name), `<div class="stack"><span class="badge blue">${I('FileText')}${formatSize(file.size)}</span><p>${file.demo ? 'Материалы к дизайн-синхронизации: навигация, первый запуск и встреча на планшете.' : 'Файл прикреплён к сообщению.'}</p>${files.has(file.id) ? '<button id="cr-file-download" class="button primary">Скачать файл</button>' : '<p class="muted">Демонстрационная карточка вложения.</p>'}<button class="button secondary" data-close>Закрыть</button></div>`, () => {
        const download = document.getElementById('cr-file-download'); if (!download) return;
        download.onclick = () => {const url = URL.createObjectURL(files.get(file.id)), link = document.createElement('a'); link.href = url; link.download = file.name; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);};
      });
    }
  });
  // Gallery synchronization carries fixture data, never local file bytes.
  M.chatRefreshSnapshot = () => ({
    threads: copy(threads),
    contacts: copy((M.chatContacts?.() || []).filter(person => person.created)),
    chat: M.state.chat
  });
  M.setChatRefreshSnapshot = snapshot => {
    if (!snapshot || typeof snapshot !== 'object' || Array.isArray(snapshot)) return false;
    const source = snapshot.threads || snapshot, next = {};
    for (const [key, thread] of Object.entries(source)) {
      if (!thread || !Array.isArray(thread.messages) || !Array.isArray(thread.pins) || !Array.isArray(thread.pending) || typeof thread.draft !== 'string') return false;
      if (thread.messages.some(item => !item || typeof item.id !== 'string' || typeof item.text !== 'string' || !Array.isArray(item.attachments))) return false;
      next[key] = copy(thread);
    }
    const custom = snapshot.contacts || [];
    if (!Array.isArray(custom) || custom.some(person => !person || person.kind !== 'group' || !person.created || typeof person.id !== 'string' || !/^group[0-9]+$/.test(person.id) || typeof person.name !== 'string' || typeof person.initials !== 'string' || !Array.isArray(person.members) || person.members.some(id => !['self', 'anna', 'max', 'maria', 'dmitry'].includes(id)))) return false;
    let changed = JSON.stringify(next) !== JSON.stringify(threads);
    const directory = M.chatContacts?.();
    if (directory) for (const person of custom) {
      if (!M.chatContact(person.id)) {directory.unshift(copy(person)); changed = true;}
    }
    // A newly created group must exist before its route can be selected.
    if (typeof snapshot.chat === 'string' && M.setChatRoute?.(snapshot.chat)) changed = true;
    threads = next; return changed;
  };
})();
