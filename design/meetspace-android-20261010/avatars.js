/* Local avatar editor. Images stay in the prototype; no uploads to a server. */
(() => {
  'use strict';
  const M=window.MS, I=name=>M.icon(name), E=value=>M.escape(value??'');
  let store={profiles:{},groups:{}}, draft=null;
  const account=()=>M.accountManagerSnapshot?.().activeId||'personal';
  const groupKey=(id,accountId=account())=>`${accountId}:${id}`;
  const photo=entry=>entry?.image?`<img class="av-photo" src="${E(entry.image)}" alt="" draggable="false"/>`:'';
  const profileTarget=()=>({kind:'profile',id:account(),accountId:account()});
  const groupTarget=id=>({kind:'group',id,accountId:account()});
  const groupControlTarget=button=>{const target=groupTarget(button.dataset.id||M.state.chat);if(button.closest('.ge-editor')){target.returnTo='groupEdit';target.returnToken=M.groupEditToken?.(target.id);}return target;};
  const returnToGroup=target=>{if(target.kind!=='group'||!canEdit(target)||M.state.chat!==target.id)return;if(target.returnTo==='groupEdit')M.resumeGroupEditor?.(target.id,target.returnToken);else M.actions.chatInfo();};
  const entryFor=target=>target.kind==='profile'?store.profiles[target.id]:store.groups[groupKey(target.id,target.accountId)];
  const canEdit=target=>target.accountId===account()&&(target.kind==='profile'||M.canEditGroupAvatar(M.chatContact(target.id)));
  M.canEditGroupAvatar=c=>Boolean(c?.kind==='group'&&c.owner==='self'&&M.chatIsVisible?.(c)!==false);
  M.profileAvatarMarkup=(fallback,id=account())=>photo(store.profiles[id])||E(fallback);
  M.chatAvatarMarkup=c=>c.kind==='group'?photo(store.groups[groupKey(c.id)])||E(c.initials):E(c.initials);
  M.renderProfileAvatarEditor=()=>{
    const has=Boolean(entryFor(profileTarget()));
    return `<section class="av-profile-card" aria-label="Фото профиля"><button class="av-edit-preview" data-action="avatarProfile" aria-label="${has?'Изменить':'Добавить'} фото профиля">${M.profileAvatarMarkup(M.profileInitials())}<span class="av-camera-badge">${I('Camera')}</span></button><div><h3>Фото профиля</h3><p>Ваше фото видно участникам встреч и чатов.</p><div class="av-actions"><button class="button secondary" data-action="avatarProfile">${I(has?'Pencil':'Upload')}${has?'Изменить фото':'Добавить фото'}</button>${has?'<button class="av-remove" data-action="avatarRemove" data-kind="profile">Удалить</button>':''}</div></div></section>`;
  };
  M.renderGroupAvatarEditor=c=>{
    if(!M.canEditGroupAvatar(c))return '<p class="av-permission-note">Фото группы меняет администратор.</p>';
    const has=Boolean(entryFor(groupTarget(c.id)));
    return `<section class="av-group-photo-row" aria-label="Фото группы"><div><strong>Фото группы</strong><p>Отображается в списке чатов и переписке.</p></div><div class="av-actions"><button class="button secondary" data-action="avatarGroup" data-id="${E(c.id)}">${I(has?'Pencil':'Camera')}${has?'Изменить фото':'Добавить фото'}</button>${has?`<button class="av-remove" data-action="avatarRemove" data-kind="group" data-id="${E(c.id)}">Удалить</button>`:''}</div></section>`;
  };
  const validImage=(value,max)=>typeof value==='string'&&value.length<=max&&/^data:image\/(?:webp|png|jpeg);base64,[A-Za-z0-9+/=]+$/.test(value);
  const normalizeCrop=value=>({zoom:Math.min(3,Math.max(1,Number(value?.zoom)||1)),x:Math.min(1,Math.max(-1,Number(value?.x)||0)),y:Math.min(1,Math.max(-1,Number(value?.y)||0)),rotation:[0,90,180,270].includes(value?.rotation)?value.rotation:0});
  const picture=src=>new Promise((resolve,reject)=>{const image=new Image();image.onload=()=>resolve(image);image.onerror=reject;image.src=src;});
  const live=d=>draft===d&&canEdit(d.target)&&document.querySelector('.av-editor')?.dataset.session===d.token;
  const closeModal=M.closeModal;
  M.closeModal=function(){const editor=document.querySelector('.av-editor');closeModal.call(this);if(editor)draft=null;};
  const uploadField=has=>`<label class="av-upload-zone">${I('Upload')}<strong>${has?'Заменить фото':'Выбрать изображение'}</strong><span>JPG, PNG или WebP · до 8 МБ</span><input id="av-file-input" type="file" accept="image/jpeg,image/png,image/webp" aria-label="${has?'Заменить фото':'Выбрать изображение'}"/></label>`;
  function showError(d,text){if(!live(d))return;const error=document.getElementById('av-editor-error');error.textContent=text;error.hidden=!text;}
  function mount(d,body){
    M.modal(d.target.kind==='profile'?'Фото профиля':'Фото группы',`${body}<p id="av-editor-error" class="av-editor-error" role="alert" hidden></p><small class="av-editor-hint">Изображение остаётся в макете и не отправляется на сервер.</small>`);
    const modal=document.querySelector('.modal');modal.classList.add('av-editor');modal.dataset.session=d.token;
    const close=modal.querySelector('[data-close]');close.removeAttribute('data-close');close.dataset.action='avatarCancel';
    document.getElementById('av-file-input').addEventListener('change',event=>{const file=event.target.files?.[0];if(file)loadFile(d,file);});
  }
  function showUpload(d,loading=false){
    mount(d,`<div class="stack">${uploadField(false)}<p class="av-editor-hint" role="status">${loading?'Подготавливаем изображение…':'Выберите фото, затем настройте область и масштаб.'}</p><button class="button secondary" data-action="avatarCancel">Отмена</button></div>`);
  }
  async function loadFile(d,file){
    if(!live(d))return;
    const version=++d.version;
    const previousSave=document.getElementById('av-save');if(previousSave)previousSave.disabled=false;
    if(!['image/jpeg','image/png','image/webp'].includes(file.type)){showError(d,'Выберите изображение JPG, PNG или WebP.');return;}
    if(file.size>8*1024*1024){showError(d,'Размер файла не должен превышать 8 МБ.');return;}
    const url=URL.createObjectURL(file);
    showError(d,'');
    const save=document.getElementById('av-save');if(save)save.disabled=true;
    try{
      const image=await picture(url);if(!live(d)||d.version!==version)return;
      if(!image.naturalWidth||image.naturalWidth*image.naturalHeight>40000000)throw new Error('size');
      const scale=Math.min(1,1200/Math.max(image.naturalWidth,image.naturalHeight));
      const canvas=document.createElement('canvas');canvas.width=Math.max(1,Math.round(image.naturalWidth*scale));canvas.height=Math.max(1,Math.round(image.naturalHeight*scale));
      canvas.getContext('2d').drawImage(image,0,0,canvas.width,canvas.height);
      const source=canvas.toDataURL('image/webp',.88);if(!validImage(source,3000000))throw new Error('size');
      const normalized=await picture(source);if(!live(d)||d.version!==version)return;
      d.source=source;d.image=normalized;d.crop=normalizeCrop();d.filename=file.name;showCrop(d);
    }catch{if(live(d)&&d.version===version){showError(d,'Не удалось прочитать фото. Выберите другой файл размером до 8 МБ.');const save=document.getElementById('av-save');if(save)save.disabled=false;}}
    finally{URL.revokeObjectURL(url);}
  }
  const slider=(key,label,min,max,value,step=1)=>`<label class="av-slider"><span>${label}<output id="av-${key}-value"></output></span><input type="range" id="av-${key}" data-av-value="${key}" min="${min}" max="${max}" step="${step}" value="${value}" aria-label="${label}"/></label>`;
  function draw(d){
    const canvas=document.getElementById('av-crop-canvas');if(!live(d)||!canvas||!d.image)return;
    const size=canvas.width, c=d.crop, sideways=c.rotation%180!==0, w=d.image.naturalWidth,h=d.image.naturalHeight;
    const rw=sideways?h:w,rh=sideways?w:h,scale=Math.max(size/rw,size/rh)*c.zoom;
    const maxX=Math.max(0,(rw*scale-size)/2),maxY=Math.max(0,(rh*scale-size)/2),ctx=canvas.getContext('2d');
    ctx.clearRect(0,0,size,size);ctx.save();ctx.translate(size/2+c.x*maxX,size/2+c.y*maxY);ctx.rotate(c.rotation*Math.PI/180);ctx.drawImage(d.image,-w*scale/2,-h*scale/2,w*scale,h*scale);ctx.restore();
    document.getElementById('av-preview-image').src=canvas.toDataURL('image/webp',.85);
    for(const key of ['zoom','x','y']){
      const input=document.getElementById('av-'+key),value=Math.round(c[key]*100);input.value=value;
      input.disabled=key==='x'?!maxX:key==='y'?!maxY:false;
      document.getElementById('av-'+key+'-value').textContent=value+'%';
    }
    return {maxX,maxY};
  }
  function showCrop(d){
    mount(d,`<div class="av-editor-layout"><div><div class="av-crop-stage"><canvas id="av-crop-canvas" width="320" height="320" role="img" aria-label="Область обрезки фотографии"></canvas><div class="av-crop-guide" aria-hidden="true"></div></div><p class="av-editor-hint">Перетащите фото или используйте ползунки.</p></div><div class="av-editor-controls"><div class="av-editor-preview"><img id="av-preview-image" alt="Предпросмотр аватара"/></div>${slider('zoom','Масштаб',100,300,d.crop.zoom*100)}${slider('x','По горизонтали',-100,100,d.crop.x*100)}${slider('y','По вертикали',-100,100,d.crop.y*100)}<div class="av-reset-actions"><button class="button quiet" id="av-reset">Сбросить</button></div>${uploadField(true)}</div></div><div class="av-editor-footer"><button class="button secondary" data-action="avatarCancel">Отмена</button><button class="button primary" id="av-save">${I('Check')}Сохранить фото</button></div>`);
    document.querySelectorAll('[data-av-value]').forEach(input=>input.addEventListener('input',()=>{d.crop[input.dataset.avValue]=Number(input.value)/100;draw(d);}));
    document.getElementById('av-reset').onclick=()=>{d.crop=normalizeCrop();draw(d);};
    document.getElementById('av-save').onclick=()=>save(d);
    const canvas=document.getElementById('av-crop-canvas');let drag=null;
    canvas.addEventListener('pointerdown',event=>{if(event.button!==0)return;const metrics=draw(d);drag={x:event.clientX,y:event.clientY,crop:{...d.crop},...metrics};canvas.setPointerCapture(event.pointerId);});
    canvas.addEventListener('pointermove',event=>{if(!drag||!live(d))return;const ratio=320/canvas.getBoundingClientRect().width;d.crop.x=drag.maxX?Math.min(1,Math.max(-1,drag.crop.x+(event.clientX-drag.x)*ratio/drag.maxX)):0;d.crop.y=drag.maxY?Math.min(1,Math.max(-1,drag.crop.y+(event.clientY-drag.y)*ratio/drag.maxY)):0;draw(d);});
    ['pointerup','pointercancel','lostpointercapture'].forEach(type=>canvas.addEventListener(type,()=>{drag=null;}));
    draw(d);const heading=document.querySelector('.av-editor .modal-head h2');heading.tabIndex=-1;heading.focus({preventScroll:true});document.querySelector('.av-editor').scrollTop=0;
  }
  async function open(target){
    if(!canEdit(target))return;
    const current=entryFor(target),d=draft={target,token:crypto.randomUUID(),version:0,source:current?.source||'',crop:normalizeCrop(current?.crop),image:null};
    showUpload(d,Boolean(current));
    if(current){const version=d.version;try{const image=await picture(current.source);if(!live(d)||d.version!==version)return;d.image=image;showCrop(d);}catch{if(live(d)&&d.version===version)showError(d,'Не удалось открыть фото. Выберите другое изображение.');}}
  }
  function update(target,value){
    if(target.kind==='profile'){if(value)store.profiles[target.id]=value;else delete store.profiles[target.id];}
    else {const key=groupKey(target.id,target.accountId);if(value)store.groups[key]=value;else delete store.groups[key];}
    M.closeModal();M.render();
    returnToGroup(target);
  }
  function save(d){
    if(!live(d)||!d.image)return;
    const output=document.createElement('canvas');output.width=output.height=256;output.getContext('2d').drawImage(document.getElementById('av-crop-canvas'),0,0,256,256);
    const image=output.toDataURL('image/webp',.9);if(!validImage(image,400000)){showError(d,'Не удалось сохранить фото. Попробуйте другое изображение.');return;}
    update(d.target,{image,source:d.source,crop:{...d.crop}});M.toast(d.target.kind==='profile'?'Фото профиля сохранено':'Фото группы сохранено');
  }
  Object.assign(M.actions,{
    avatarProfile:()=>open(profileTarget()),
    avatarGroup:button=>open(groupControlTarget(button)),
    avatarCancel:()=>{const target=draft?.target;M.closeModal();if(target)returnToGroup(target);},
    avatarRemove:button=>{
      const target=button.dataset.kind==='group'?groupControlTarget(button):profileTarget();if(!canEdit(target)||!entryFor(target))return;
      M.modal('Удалить фото?',`<div class="stack"><p>Вместо фото ${target.kind==='group'?'группы':'профиля'} снова появятся инициалы. Вы сможете добавить новое изображение.</p><button class="button danger" id="av-confirm-remove">Удалить фото</button><button class="button secondary" id="av-cancel-remove">Отмена</button></div>`,()=>{const cancel=()=>{M.closeModal();returnToGroup(target);};document.getElementById('av-cancel-remove').onclick=cancel;const close=document.querySelector('.modal [data-close]');close.removeAttribute('data-close');close.onclick=cancel;document.getElementById('av-confirm-remove').onclick=()=>{if(!canEdit(target))return;update(target,null);M.toast('Фото удалено');};});
    },
  });
  document.addEventListener('keydown',event=>{if(event.key!=='Escape')return;const cancel=document.getElementById('av-cancel-remove');if(cancel){event.preventDefault();event.stopImmediatePropagation();cancel.click();return;}if(document.querySelector('.av-editor')&&draft?.target.kind==='group'){event.preventDefault();event.stopImmediatePropagation();M.actions.avatarCancel();}},true);
  M.avatarSnapshot=()=>JSON.parse(JSON.stringify(store));
  M.setAvatarSnapshot=value=>{
    if(!value?.profiles||!value?.groups||typeof value.profiles!=='object'||typeof value.groups!=='object'||Array.isArray(value.profiles)||Array.isArray(value.groups))return false;
    const next={profiles:{},groups:{}};
    for(const kind of ['profiles','groups'])for(const [key,item]of Object.entries(value[kind])){
      if(!item||!validImage(item.image,400000)||!validImage(item.source,3000000))return false;
      Object.defineProperty(next[kind],key,{value:{image:item.image,source:item.source,crop:normalizeCrop(item.crop)},enumerable:true,writable:true,configurable:true});
    }
    if(JSON.stringify(store)===JSON.stringify(next))return false;store=next;return true;
  };
})();
