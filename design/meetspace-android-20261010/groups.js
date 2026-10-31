/* Group settings for the local prototype. Changes are scoped to the demo account. */
(() => {
  'use strict';
  const M=window.MS, I=name=>M.icon(name), E=value=>M.escape(value??'');
  let stores={}, drafts={};
  const account=()=>M.accountManagerSnapshot?.().activeId||'personal';
  const key=id=>`${account()}:${id}`;
  const rawContact=M.chatContact;
  const initials=name=>Array.from(name.trim())[0]?.toLocaleUpperCase('ru')||'Г';
  M.resolveGroupContact=c=>{
    if(!c||c.kind!=='group')return c;
    const value=stores[account()]?.[c.id];
    return value?{...c,name:value.name,description:value.description,members:[...value.members],initials:initials(value.name)}:c;
  };
  M.chatContact=id=>M.resolveGroupContact(rawContact(id));
  M.canEditGroup=c=>Boolean(c?.kind==='group'&&c.owner==='self'&&c.members.includes('self')&&M.chatIsVisible?.(c)!==false);
  M.renderGroupEditEntry=c=>M.canEditGroup(c)?`<button class="button secondary ge-edit-entry" data-action="groupEdit" data-id="${E(c.id)}">${I('Pencil')}Редактировать группу</button>`:'';
  const members=()=>M.chatMemberDirectory().filter((person,index,all)=>all.findIndex(item=>item.id===person.id)===index);
  const sameContext=d=>d.accountId===account()&&M.state.chat===d.id&&M.canEditGroup(M.chatContact(d.id));
  M.groupEditToken=id=>drafts[key(id)]?.token;
  M.resumeGroupEditor=(id,token)=>{
    const d=drafts[key(id)];if(d&&d.token===token&&sameContext(d))open(id,true);
  };
  function open(id,resume=false){
    const c=M.chatContact(id);if(!M.canEditGroup(c)||M.state.chat!==id)return;
    const d=resume?drafts[key(id)]:{id,accountId:account(),token:crypto.randomUUID(),name:c.name,description:c.description||'',members:[...c.members]};
    if(!d||!sameContext(d))return;drafts[key(id)]=d;
    M.modal('Редактировать группу',`<div class="ge-status">${I('ShieldCheck')}Вы — администратор группы</div><section class="ge-photo-section"><span class="avatar ge-avatar">${M.chatAvatarMarkup(c)}</span>${M.renderGroupAvatarEditor(c)}</section><form id="ge-form" novalidate><div class="ge-fields"><label class="field">Название группы<input id="ge-name" name="name" value="${E(d.name)}" maxlength="50" autocomplete="off" required aria-describedby="ge-error"/></label><label class="field">Описание<textarea id="ge-description" name="description" maxlength="300" rows="3" placeholder="О чём эта группа">${E(d.description)}</textarea></label></div><section class="ge-members"><div class="section-heading"><h3>Участники</h3><span class="ge-member-count" id="ge-member-count">${d.members.length}</span></div><label class="search-box ge-member-search">${I('Search')}<input id="ge-search" type="search" placeholder="Найти участника" aria-label="Найти участника" autocomplete="off"/></label><div class="ge-member-list">${members().map(person=>`<label class="ge-member-row ${person.id==='self'?'is-owner':''}" data-search="${E(person.name.toLocaleLowerCase('ru'))}"><span class="avatar ${E(person.tone||'')}">${person.id==='self'?M.profileAvatarMarkup(M.profileInitials()):E(person.initials)}</span><span><strong>${E(person.name)}</strong><small>${person.id==='self'?'Вы · Администратор':d.members.includes(person.id)?'В группе':'Добавить в группу'}</small></span><input type="checkbox" name="members" value="${E(person.id)}" ${d.members.includes(person.id)?'checked':''} ${person.id==='self'?'disabled':''} aria-label="${person.id==='self'?'Администратор':`Участник ${E(person.name)}`}"/></label>`).join('')}</div><p id="ge-members-empty" class="ge-note" hidden>Участник не найден</p><p class="ge-note">Отметьте участников, которые должны быть в группе. Администратор остаётся в составе группы.</p></section><p id="ge-error" class="ge-error" role="alert" hidden></p><footer class="ge-footer"><button class="button secondary" type="button" data-action="groupEditCancel">Отмена</button><button class="button primary" type="submit">${I('Check')}Сохранить изменения</button></footer></form>`,()=>{
      const form=document.getElementById('ge-form'), name=form.elements.name, description=form.elements.description;
      const error=document.getElementById('ge-error');
      const fail=text=>{error.textContent=text;error.hidden=false;};
      name.oninput=()=>{d.name=name.value;error.hidden=true;name.removeAttribute('aria-invalid');};
      description.oninput=()=>{d.description=description.value;};
      form.querySelectorAll('[name="members"]').forEach(input=>input.onchange=()=>{
        d.members=['self',...Array.from(form.querySelectorAll('[name="members"]:checked:not(:disabled)')).map(node=>node.value)];
        document.getElementById('ge-member-count').textContent=d.members.length;
        form.querySelectorAll('[name="members"]:not(:disabled)').forEach(node=>node.closest('label').querySelector('small').textContent=node.checked?'В группе':'Добавить в группу');error.hidden=true;
      });
      document.getElementById('ge-search').oninput=event=>{
        const q=event.target.value.trim().toLocaleLowerCase('ru');let count=0;
        document.querySelectorAll('.ge-member-row').forEach(row=>{row.hidden=!row.dataset.search.includes(q);if(!row.hidden)count++;});
        document.getElementById('ge-members-empty').hidden=Boolean(count);
      };
      form.onsubmit=event=>{
        event.preventDefault();if(!sameContext(d))return;
        const clean=d.name.trim();
        if(!clean||Array.from(clean).length>50||/[\u0000-\u001f\u007f]/.test(clean)){name.setAttribute('aria-invalid','true');fail('Введите название группы от 1 до 50 символов.');name.focus();return;}
        if(d.members.length<2){fail('Оставьте в группе хотя бы одного участника, кроме администратора.');return;}
        const next={name:clean,description:d.description.trim().slice(0,300),members:[...d.members]};
        stores[d.accountId]||={};stores[d.accountId][d.id]=next;delete drafts[key(d.id)];
        M.closeModal();M.render();M.actions.chatInfo();M.toast('Настройки группы сохранены');
      };
    });
    const modal=document.querySelector('.modal');modal.classList.add('ge-editor');modal.dataset.groupId=id;
    const close=modal.querySelector('[data-close]');close.removeAttribute('data-close');close.dataset.action='groupEditCancel';
  }
  Object.assign(M.actions,{
    groupEdit:button=>open(button.dataset.id||M.state.chat),
    groupEditCancel:()=>{const id=document.querySelector('.ge-editor')?.dataset.groupId;if(!id)return;delete drafts[key(id)];M.closeModal();if(M.state.chat===id)M.actions.chatInfo();},
  });
  document.addEventListener('keydown',event=>{if(event.key==='Escape'&&document.querySelector('.ge-editor')){event.preventDefault();event.stopImmediatePropagation();M.actions.groupEditCancel();}},true);
  M.groupEditingSnapshot=()=>JSON.parse(JSON.stringify(stores));
  M.setGroupEditingSnapshot=value=>{
    if(!value||typeof value!=='object'||Array.isArray(value))return false;
    const next={};const known=new Set(members().map(person=>person.id));
    for(const [accountId,groups]of Object.entries(value)){
      if(!groups||typeof groups!=='object'||Array.isArray(groups))return false;
      const entries={};
      for(const [id,group]of Object.entries(groups)){
        if(rawContact(id)?.kind!=='group'||rawContact(id)?.owner!=='self'||!group||typeof group.name!=='string'||!group.name.trim()||Array.from(group.name).length>50||typeof group.description!=='string'||group.description.length>300||!Array.isArray(group.members)||!group.members.includes('self')||group.members.length<2||group.members.some(member=>!known.has(member))||new Set(group.members).size!==group.members.length)return false;
        Object.defineProperty(entries,id,{value:{name:group.name,description:group.description,members:[...group.members]},enumerable:true,writable:true,configurable:true});
      }
      Object.defineProperty(next,accountId,{value:entries,enumerable:true,writable:true,configurable:true});
    }
    if(JSON.stringify(stores)===JSON.stringify(next))return false;stores=next;return true;
  };
})();
