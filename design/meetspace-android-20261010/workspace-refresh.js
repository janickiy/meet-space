/* October 2026 product scenarios. Local fixtures only; no media or server requests. */
(() => {
  const M = window.MS, I = name => M.icon(name), E = text => M.escape(text);
  const modes = [
    ['composite','Video','Общая запись','Видео участников, звук и демонстрация экрана'],
    ['audio','AudioLines','Только аудио','Разговор без видеодорожки'],
    ['tracks','Users','Отдельные дорожки','Дорожки участников и общий микс'],
    ['screen','MonitorUp','Фокус на экране','Демонстрация экрана и звук встречи'],
  ];
  const isOwner = () => !M.state.guestMode && (!M.state.meetingMeta || M.state.meetingMeta.mine === true);
  const accountId=()=>M.accountManagerSnapshot?.().activeId;
  const mayStop=()=>!M.state.guestMode && (isOwner() || room().requestedBy===accountId());
  const key = () => M.state.meetingMeta?.id || M.state.meetingTitle || 'design';
  const room = () => {
    M.state.roomRefresh ||= {};
    return M.state.roomRefresh[key()] ||= {recording:'idle',mode:'composite',people:[
      {id:'self',name:'Вы',initials:'АС',role:isOwner()?'owner':'participant'},
      {id:'anna',name:'Анна Кузнецова',initials:'АК',role:isOwner()?'co_host':'owner'},
      {id:'maria',name:'Мария Полякова',initials:'МП',role:'participant'},
      {id:'dmitry',name:'Дмитрий Волков',initials:'ДВ',role:'participant'},
      {id:'max',name:'Максим Петров',initials:'МП',role:'participant'}
    ],waiting:[{id:'guest',name:'Елена Смирнова',initials:'ЕС',role:'guest'}], invitations:[]};
  };
  const labels = {idle:'Записать',starting:'Запуск записи',recording:'Идёт запись',processing:'Обработка записи',ready:'Запись готова'};
  M.renderRecordingControl = () => `<button class="recording-control ${room().recording}" data-action="manageRecording" aria-label="Управление записью: ${labels[room().recording]}" aria-haspopup="dialog">${I(room().recording === 'recording' ? 'CircleDot':'Circle')}<span>${labels[room().recording]}</span>${I('ChevronDown')}</button>`;
  M.actions.manageRecording = () => {
    if(M.state.guestMode){M.modal('Запись встречи', `<div class="stack"><p>Гости не могут запускать запись. Попросите участника встречи с аккаунтом начать запись.</p><button class="button secondary" data-close>Понятно</button></div>`);return;}
    const s = room();
    M.modal('Запись встречи', `<div class="stack"><div class="wr-state"><span class="wr-symbol">${I('Video')}</span><div><h3>${labels[s.recording]}</h3><p>${E(M.state.meetingTitle || 'Дизайн-синхронизация')}</p></div></div>${['idle','ready'].includes(s.recording) ? `${s.recording === 'ready' && Number.isInteger(s.recordIndex) && s.recordIndex >= 0 ? `<button class="button secondary" data-action="record" data-index="${s.recordIndex}">${I('CirclePlay')}Открыть запись</button>` : ''}<fieldset class="wr-modes"><legend>Формат записи</legend>${modes.map(([id,icon,title,hint])=>`<label class="wr-mode"><input type="radio" name="record-mode" value="${id}" ${s.mode === id ? 'checked':''}/><span>${I(icon)}</span><div><strong>${title}</strong><small>${hint}</small></div></label>`).join('')}</fieldset><button class="button primary" data-action="wrStartRecording">${I('Circle')}Начать запись</button>` : s.recording === 'recording' ? `<div class="wr-notice">${I('CircleDot')}Записывается: ${modes.find(x=>x[0] === s.mode)[2]}</div><button class="button danger" data-action="wrStopRecording">${I('Square')}Остановить запись</button>` : `<div class="wr-notice" role="status">${I('Clock')}${s.recording === 'processing' ? 'Готовим файлы. После обработки они появятся в записях встречи.' : 'Подготавливаем запись. Это займёт несколько секунд.'}</div>`}<p class="wr-hint">Записи хранятся 7 дней. Остановить запись может её инициатор или организатор встречи.</p><small class="wr-hint">Демонстрация интерфейса · камера и микрофон не записываются.</small></div>`);
  };
  M.actions.wrStartRecording = () => {
    const s=room(); if (M.state.guestMode || !['idle','ready'].includes(s.recording)) return;
    s.requestedBy=accountId();
    s.recordToken=crypto.randomUUID();
    s.mode=document.querySelector('[name="record-mode"]:checked')?.value || s.mode;
    s.recording='starting'; M.closeModal(); M.render();
    const roomKey=key(),recordToken=s.recordToken;
    setTimeout(()=>{const current=M.state.roomRefresh?.[roomKey];if(!current||current.recording!=='starting'||current.recordToken!==recordToken)return;current.recording='recording';M.render();M.toast('Демо: запись начата. Участники увидят индикатор.');},900);
  };
  M.actions.wrStopRecording = () => {
    const s=room(); if (s.recording !== 'recording' || !mayStop()) return;
    M.modal('Остановить запись?',`<div class="stack"><p>Текущая запись завершится. Встреча продолжится, а файлы будут доступны после обработки.</p><button class="button danger" data-action="wrConfirmStop">Остановить запись</button><button class="button secondary" data-close>Продолжить запись</button></div>`);
  };
  M.actions.wrConfirmStop = () => {
    const s=room(); if (s.recording !== 'recording' || !mayStop()) return;
    const roomKey=key(),recordToken=s.recordToken,title=M.state.meetingTitle || 'Дизайн-синхронизация';
    s.recording='processing';M.closeModal();M.render();setTimeout(()=>{const current=M.state.roomRefresh?.[roomKey];if(!current||current.recording!=='processing'||current.recordToken!==recordToken)return;current.recording='ready';current.recordIndex=M.addMeetingRecording?.({key:roomKey+':'+recordToken,title,mode:current.mode,people:current.people.length});M.render();M.toast('Демо: запись добавлена в библиотеку.');},1600);
  };
  const roleLabel = p => ({owner:'Организатор',co_host:'Соорганизатор',participant:'Участник',guest:'Гость'})[p.role];
  M.actions.participants = () => {
    const s=room();
    M.modal(`Участники · ${s.people.length}`,`<div class="stack"><div class="wr-notice">${I('ShieldCheck')}${isOwner()?'Вы управляете встречей как организатор.':'Участники и роли в этой встрече.'}</div>${isOwner() && s.waiting.length ? `<section class="wr-waiting"><h3>Зал ожидания <span>${s.waiting.length}</span></h3>${s.waiting.map(p=>`<div class="wr-person"><span class="avatar">${p.initials}</span><div><strong>${E(p.name)}</strong><small>Гость · ожидает допуска</small></div></div><div class="wr-inline-actions"><button class="button primary" data-action="wrAdmit" data-id="${p.id}">Допустить</button><button class="button secondary" data-action="wrReject" data-id="${p.id}">Отклонить</button></div>`).join('')}</section>`:''}<div class="wr-people">${s.people.map(p=>`<div class="wr-person"><span class="avatar ${p.id==='self'?'blue':'mint'}">${p.initials}</span><div><strong>${E(p.name)}</strong><small>${roleLabel(p)}${p.mute?' · звук запрещён':''}${p.camera?' · видео запрещено':''}${p.screen?' · экран запрещён':''}</small></div>${isOwner() && p.id!=='self'?`<button class="icon-button" data-action="wrPerson" data-id="${p.id}" aria-label="Управление: ${E(p.name)}">${I('Ellipsis')}</button>`:''}</div>`).join('')}</div><button class="button secondary" data-action="wrInvite">${I('UserPlus')}Пригласить участников</button></div>`);
  };
  M.actions.wrAdmit = b => { const s=room();if(!isOwner())return;const p=s.waiting.find(p=>p.id===b.dataset.id);if(!p)return;s.people.push(p);s.waiting=s.waiting.filter(x=>x!==p);M.render();M.actions.participants(); };
  M.actions.wrReject = b => {if(!isOwner())return;room().waiting=room().waiting.filter(p=>p.id!==b.dataset.id);M.render();M.actions.participants();};
  M.actions.wrPerson = b => {
    const p=room().people.find(p=>p.id===b.dataset.id);if(!p||p.id==='self'||!isOwner())return;
    M.modal(p.name,`<div class="stack"><p>${roleLabel(p)}</p>${[['mute','MicOff','микрофон'],['camera','VideoOff','камеру'],['screen','Monitor','экран']].map(([id,icon,label])=>`<button class="button secondary" data-action="wrModerate" data-id="${p.id}" data-control="${id}">${I(icon)}${p[id]?'Разрешить':'Отключить'} ${label}</button>`).join('')}<button class="button secondary" data-action="wrModerate" data-id="${p.id}" data-control="role">${I('Users')}${p.role==='co_host'?'Убрать роль соорганизатора':'Назначить соорганизатором'}</button><button class="button danger" data-action="wrKick" data-id="${p.id}">Исключить из встречи</button><button class="button quiet" data-action="participants">К участникам</button></div>`);
  };
  M.actions.wrModerate = b => {const p=room().people.find(p=>p.id===b.dataset.id);if(!p||p.id==='self'||!isOwner())return;const c=b.dataset.control;if(c==='role')p.role=p.role==='co_host'?'participant':'co_host';else if(['mute','camera','screen'].includes(c))p[c]=!p[c];M.render();M.actions.wrPerson(b);};
  M.actions.wrKick = b => {const p=room().people.find(p=>p.id===b.dataset.id);if(!p||p.id==='self'||!isOwner())return;M.modal('Исключить участника?',`<div class="stack"><p>${E(p.name)} покинет встречу. Повторное присоединение будет запрещено.</p><button class="button danger" data-action="wrConfirmKick" data-id="${p.id}">Исключить</button><button class="button secondary" data-action="participants">Отмена</button></div>`);};
  M.actions.wrConfirmKick = b => {if(!isOwner())return;room().people=room().people.filter(p=>p.id!==b.dataset.id||p.id==='self');M.render();M.actions.participants();};
  M.actions.wrInvite = () => M.modal('Пригласить участников',`<form id="wr-invite-form" class="stack"><p>Найдите коллегу или укажите email для приглашения.</p><label class="field">Имя или email<input id="wr-invite-query" type="search" autocomplete="off" placeholder="Например, Мария"/></label><div id="wr-invite-results"></div><div class="wr-selected" id="wr-invite-selected"></div><button class="button primary" id="wr-invite-send" disabled>Пригласить</button><button class="button secondary" type="button" data-action="copyInvite">${I('Link')}Скопировать ссылку</button><p class="wr-hint">По действующей ссылке можно войти без аккаунта и без ожидания допуска.</p><small class="wr-hint">Демо: приглашения не отправляются.</small></form>`,()=>{
    const selected=new Map(), input=document.getElementById('wr-invite-query'),results=document.getElementById('wr-invite-results');
    const directory=[['maria','Мария Полякова','maria@example.org'],['anna','Анна Кузнецова','anna@example.org'],['max','Максим Петров','max@example.org']];
    function update(){const q=input.value.trim().toLowerCase();const options=directory.filter(x=>q&&x.join(' ').toLowerCase().includes(q));if(/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(q)&&!options.some(x=>x[2]===q))options.push([q,q,q]);results.innerHTML=options.map(([id,name,email])=>`<button type="button" class="wr-search-person" data-email="${E(email)}" data-name="${E(name)}"><span class="avatar">${E(name[0].toUpperCase())}</span><span><strong>${E(name)}</strong><small>${E(email)}</small></span>${I('Plus')}</button>`).join('')||(q?'<p class="wr-hint">Нет совпадений. Введите полный email.</p>':'');results.querySelectorAll('button').forEach(b=>b.onclick=()=>{selected.set(b.dataset.email,b.dataset.name);input.value='';update();});document.getElementById('wr-invite-selected').innerHTML=[...selected].map(([email,name])=>`<button type="button" data-remove="${E(email)}" aria-label="Убрать ${E(name)}">${E(name)} ${I('X')}</button>`).join('');document.querySelectorAll('[data-remove]').forEach(b=>b.onclick=()=>{selected.delete(b.dataset.remove);update();});document.getElementById('wr-invite-send').disabled=!selected.size;}
    input.oninput=update;document.getElementById('wr-invite-form').onsubmit=e=>{e.preventDefault();if(!selected.size)return;room().invitations.push(...selected.keys());M.closeModal();M.toast(`Демо: приглашено участников — ${selected.size}`);};
  });
  const oldDetails=M.actions.meetingDetails;
  M.actions.meetingDetails=b=>{oldDetails(b);document.querySelector('.cf-details')?.insertAdjacentHTML('beforeend',`<button class="button secondary" data-action="folderAssign" data-item="meeting:${E(b.dataset.id)}">${I('Folder')}Добавить в папку</button>`);};
  const oldConferenceInvite=M.actions.conferenceInvite;
  M.actions.conferenceInvite = b => {oldConferenceInvite(b);const target=document.querySelector('.cf-form-note');target?.insertAdjacentHTML('beforebegin',`<button class="button secondary" data-action="wrInvite">${I('UserPlus')}Пригласить по имени или email</button><p class="wr-hint">По ссылке можно присоединиться без аккаунта.</p>`);};
  const prejoinRender=M.screens.prejoin.render;
  M.screens.prejoin.render=()=>prejoinRender().replace('<label class="field">Ваше имя',`<div class="wr-guest-choice"><button type="button" data-action="wrGuestMode" aria-pressed="${Boolean(M.state.guestMode)}">${I('UserRound')}${M.state.guestMode?'Вход без аккаунта · гость':'Присоединиться как гость'}</button></div><label class="field">${M.state.guestMode?'Как вас представить?':'Ваше имя'}`);
  M.actions.wrGuestMode=()=>{M.state.guestMode=!M.state.guestMode;M.render();};
  const openConference=M.actions.openConference;
  M.actions.openConference=b=>{M.state.guestMode=false;openConference(b);};
  const originalMeetingRender=M.screens.meetings.render;
  M.screens.meetings.render=()=>originalMeetingRender().replace('<section class="cf-list-section"',`<div class="wr-shortcuts"><button data-go="recordings">${I('CirclePlay')}Записи и материалы ${I('ChevronRight')}</button><button data-go="folders">${I('Folder')}Мои папки ${I('ChevronRight')}</button></div><section class="cf-list-section"`);
  const callRender=M.screens.call.render;
  M.screens.call.render=()=>callRender().replaceAll('5 участников',`${room().people.length} участников`).replace('<div class="call-body">',`<div class="wr-call-context"><span>${I('ShieldCheck')}${isOwner()?'Вы — организатор':M.state.guestMode?'Вы — гость':'Вы — участник'}</span><button data-action="participants">${I('Users')}${isOwner() && room().waiting.length ? `Ожидают допуска: ${room().waiting.length}`:'Участники встречи'}</button></div><div class="call-body">`);
  const syncFields=['meetingTitle','meetingMeta','meetingItems','meetings','created','guestMode','participantName'];
  M.workspaceRefreshSnapshot=()=>JSON.parse(JSON.stringify({rooms:M.state.roomRefresh||{},fields:Object.fromEntries(syncFields.filter(k=>M.state[k]!==undefined).map(k=>[k,M.state[k]]))}));
  M.setWorkspaceRefreshSnapshot=value=>{
    if(!value||typeof value.rooms!=='object')return false;
    let changed=JSON.stringify(M.state.roomRefresh||{})!==JSON.stringify(value.rooms);
    if(changed)M.state.roomRefresh=JSON.parse(JSON.stringify(value.rooms));
    for(const k of syncFields)if(Object.hasOwn(value.fields||{},k)&&JSON.stringify(M.state[k])!==JSON.stringify(value.fields[k])){M.state[k]=JSON.parse(JSON.stringify(value.fields[k]));changed=true;}
    return changed;
  };
})();
