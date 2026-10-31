/* Notification centre for the local design prototype. No push or server requests. */
(() => {
  'use strict';
  const M = window.MS, I = name => M.icon(name), E = value => M.escape(value);
  const events = [
    {id:'reminder', title:'Встреча скоро начнётся', body:'Дизайн-синхронизация · начало в 10:00', time:'10 минут назад', category:'Напоминание', icon:'CalendarDays', group:'Недавно', meeting:'design'},
    {id:'invitation', title:'Приглашение на встречу', body:'Анна Кузнецова пригласила вас на дизайн-синхронизацию.', time:'25 минут назад', category:'Приглашение', icon:'UserPlus', group:'Недавно', meeting:'design'},
    {id:'recording', title:'Запись встречи готова', body:'Продукт. Следующая глава — видео и материалы уже доступны.', time:'1 час назад', category:'Запись', icon:'CirclePlay', group:'Недавно', record:0},
    {id:'mention', title:'Вас упомянули в группе', body:'Мария Полякова · Команда продукта', time:'Ранее', category:'Сообщение', icon:'MessagesSquare', group:'Ранее', chat:'team'},
    {id:'summary', title:'Итоги встречи готовы', body:'Продукт. Следующая глава — решения и задачи по итогам встречи.', time:'Ранее', category:'Итоги встречи', icon:'FileText', group:'Ранее', record:0, summary:true},
  ];
  const ids = new Set(events.map(event => event.id));
  let stores = {}, filter = 'all';
  const account = () => M.accountManagerSnapshot?.().activeId || 'personal';
  const state = () => stores[account()] ||= {read:['mention','summary']};
  const unread = event => !state().read.includes(event.id);
  const count = () => events.filter(unread).length;
  const countLabel = n => `${n} ${n===1?'непрочитанное':'непрочитанных'}`;
  const newLabel = n => `${n} ${n===1?'новое':'новых'}`;
  const bellContent = () => `${I('Bell')}${count()?`<span class="nt-badge" aria-hidden="true">${count()}</span>`:''}`;
  M.renderNotificationBell = () => `<button class="icon-button nt-bell" data-action="notifications" aria-label="Уведомления${count()?`, ${countLabel(count())}`:', всё прочитано'}" aria-haspopup="dialog" aria-expanded="false">${bellContent()}</button>`;
  const list = () => {
    const shown = events.filter(event => filter==='all' || unread(event));
    if(!shown.length)return `<div class="nt-empty">${I('CheckCheck')}<h3>Всё прочитано</h3><p>Новых уведомлений пока нет. Все события доступны во вкладке «Все».</p></div>`;
    let group = '';
    return shown.map(event => {
      const heading = event.group!==group?`<h3 class="nt-date">${E(event.group)}</h3>`:'';group=event.group;
      return `${heading}<button class="nt-event ${unread(event)?'is-unread':''}" data-action="ntOpen" data-id="${event.id}" aria-label="${E(event.title)}. ${E(event.body)}. ${unread(event)?'Не прочитано':'Прочитано'}"><span class="nt-event-icon">${I(event.icon)}</span><span class="nt-event-copy"><strong>${E(event.title)}</strong><p>${E(event.body)}</p><span class="nt-event-meta">${E(event.time)} · ${E(event.category)}</span></span>${unread(event)?'<i class="nt-unread-dot" aria-hidden="true"></i>':''}${I('ChevronRight')}</button>`;
    }).join('');
  };
  M.refreshNotifications = () => {
    document.querySelectorAll('.nt-bell').forEach(button => {
      button.innerHTML=bellContent();
      button.setAttribute('aria-label',`Уведомления${count()?`, ${countLabel(count())}`:', всё прочитано'}`);
      button.setAttribute('aria-expanded',String(Boolean(document.querySelector('.nt-panel'))));
    });
    const panel=document.querySelector('.nt-panel');if(!panel)return;
    panel.querySelector('.nt-total').textContent=count()?newLabel(count()):'Всё прочитано';
    panel.querySelector('.nt-read-all').disabled=!count();
    panel.querySelectorAll('[data-action="ntFilter"]').forEach(button=>button.setAttribute('aria-pressed',String(button.dataset.value===filter)));
    const log=panel.querySelector('.nt-list'), top=log.scrollTop;log.innerHTML=list();log.scrollTop=top;
  };
  const position = () => {
    const panel=document.querySelector('.nt-panel');if(!panel)return;
    const bell=document.querySelector('.nt-bell')?.getBoundingClientRect();
    const width=Math.min(400,innerWidth-24), top=Math.min((bell?.bottom || 68)+8,Math.max(12,innerHeight-240));
    panel.style.width=width+'px';panel.style.left=Math.max(12,Math.min(innerWidth-width-12,(bell?.right||innerWidth-12)-width))+'px';
    panel.style.right='auto';panel.style.top=top+'px';panel.style.setProperty('--nt-panel-top',top+'px');
  };
  const open = () => {
    filter='all';
    M.modal('Уведомления',`<div class="nt-toolbar"><div class="nt-filters" role="group" aria-label="Фильтр уведомлений"><button data-action="ntFilter" data-value="all" aria-pressed="true">Все</button><button data-action="ntFilter" data-value="unread" aria-pressed="false">Непрочитанные</button></div><button class="nt-read-all" data-action="ntReadAll" ${count()?'':'disabled'} aria-label="Прочитать все уведомления">${I('CheckCheck')}<span>Прочитать все</span></button></div><div class="nt-list" role="region" aria-label="Список уведомлений">${list()}</div><footer class="nt-footer"><button data-action="ntSettings">${I('Settings')}Настроить уведомления${I('ChevronRight')}</button></footer>`);
    const panel=document.querySelector('.modal'), backdrop=document.querySelector('.modal-backdrop');
    panel.classList.add('nt-panel');backdrop.classList.add('nt-backdrop');
    const head=panel.querySelector('.modal-head');head.classList.add('nt-heading');
    head.innerHTML=`<div><h2 id="modal-title">Уведомления</h2><span class="nt-total" aria-live="polite">${count()?newLabel(count()):'Всё прочитано'}</span></div><button class="icon-button" data-close aria-label="Закрыть уведомления">${I('X')}</button>`;
    backdrop.addEventListener('click',event=>{if(event.target===backdrop)M.closeModal();});
    position();M.refreshNotifications();head.querySelector('button').focus();
  };
  const closeModal=M.closeModal;
  M.closeModal=function(){closeModal.call(this);document.querySelectorAll('.nt-bell').forEach(button=>button.setAttribute('aria-expanded','false'));};
  Object.assign(M.actions, {
    notifications:open,
    ntFilter:button=>{filter=button.dataset.value==='unread'?'unread':'all';M.refreshNotifications();},
    ntReadAll:()=>{state().read=events.map(event=>event.id);M.refreshNotifications();M.productStateChanged?.();document.querySelector('.nt-filters [aria-pressed="true"]')?.focus();},
    ntOpen:button=>{
      const event=events.find(item=>item.id===button.dataset.id);if(!event)return;
      if(unread(event))state().read.push(event.id);
      M.refreshNotifications();M.productStateChanged?.();M.closeModal();
      if(event.meeting){M.go('meetings');M.actions.meetingDetails({dataset:{id:event.meeting}});}
      else if(event.chat)M.actions.chat({dataset:{chat:event.chat}});
      else if(event.record!==undefined){M.actions.record({dataset:{index:String(event.record)}});if(event.summary)M.actions.rrMaterialTab({dataset:{value:'summary'}});}
    },
    ntSettings:()=>{M.closeModal();M.actions.accountSection({dataset:{section:'notifications'}});},
  });
  M.notificationSnapshot=()=>{state();return {stores:JSON.parse(JSON.stringify(stores))};};
  M.setNotificationSnapshot=value=>{
    if(!value?.stores||typeof value.stores!=='object'||Array.isArray(value.stores))return false;
    const next={};
    for(const [key,item]of Object.entries(value.stores)){
      if(!item||!Array.isArray(item.read)||item.read.some(id=>!ids.has(id)))return false;
      Object.defineProperty(next,key,{value:{read:[...new Set(item.read)]},enumerable:true,writable:true,configurable:true});
    }
    if(JSON.stringify(stores)===JSON.stringify(next))return false;stores=next;return true;
  };
  window.addEventListener('resize',position);
})();
