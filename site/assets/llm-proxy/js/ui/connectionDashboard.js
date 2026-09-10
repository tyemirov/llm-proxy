// @ts-check

import * as backend from '../core/backendClient.js?v=20260903f037';
import {profileFailureMessage} from '../core/managementProfile.js?v=20260903f037';

import {COPY} from '../constants.js?v=20260903f037';

const MCP_PATH = '/mcp';

export const CONNECTION_CONTEXT_EVENT = 'llm-proxy:connection-context';
/** @param {unknown} value */
function escapeHTML(value) { return String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c] || c)); }
/** @param {string} provider */
function providerIcon(provider) { return `<brand-icon kind="provider" identifier="${escapeHTML(provider)}"></brand-icon>`; }

/** @param {import('../types.d.js').AccountConnection} connection */
function connectionReady(connection) {
  return connection.fields.every(field => !field.required || field.configured || (!field.secret && Boolean(field.value?.trim())));
}

/** The tenant dashboard owns configuration selection and emits its usage context. */
export class ConnectionDashboard extends HTMLElement {
  /** @type {import('../types.d.js').ManagementTenantSummary[]} */ tenants = [];
  /** @type {import('../types.d.js').AccountConnection[]} */ connections = [];
  /** @type {import('../types.d.js').ProviderProfile[]} */ providers = [];
  /** @type {import('../types.d.js').ManagementTenantProfile|null} */ profile = null;
  /** @type {Record<string, string>} */ modelFamilies = {};
  tenantID = '';
  connectionID = '';
  modelID = '';
  capability = 'text';
  search = '';
  busy = false;
  message = '';
  failure = '';
  secret = '';
  requestExample = '';
  controller = new AbortController();
  observer = new ResizeObserver(() => this.drawRoutes());
  revision = 0;
  /** @type {HTMLElement|null} */ returnFocus = null;

  connectedCallback() {
    this.controller = new AbortController();
    this.addEventListener('click', event => this.handleClick(event), {signal:this.controller.signal});
    this.addEventListener('input', event => {
      if (event.target instanceof HTMLInputElement && event.target.name === 'dashboard-search') {
        this.search = event.target.value; this.renderMap();
      }
    }, {signal:this.controller.signal});
    this.observer.observe(this);
    void this.run(async () => { this.modelFamilies = await backend.fetchModelFamilies(this.controller.signal); await this.reload(); await this.selectTenant(this.tenants[0]?.id || ''); });
  }
  disconnectedCallback() { this.controller.abort(); this.observer.disconnect(); this.secret = ''; this.requestExample = ''; this.revision += 1; }
  get connection() { return this.connections.find(c => c.id === this.connectionID); }
  get provider() { return this.providers.find(p => p.id === this.connection?.provider); }
  get attached() { return Boolean(this.connection?.tenant_ids.includes(this.tenantID)); }
  get ready() { return Boolean(this.connection && connectionReady(this.connection)); }
  get tenantName() { return this.tenants.find(t => t.id === this.tenantID)?.name || 'Account'; }

  /** @param {string} id */
  isDefaultModel(id) {
    const defaults=this.profile?.tenant.defaults;
    return this.connection?.provider===(this.capability==='text'?defaults?.provider:defaults?.dictation_provider)
      && id===(this.capability==='text'?defaults?.model:defaults?.dictation_model);
  }

  async reload() {
    const [account, catalog] = await Promise.all([backend.fetchAccount(this.controller.signal), backend.fetchConnections(this.controller.signal)]);
    this.tenants = account.tenants; this.connections = catalog.connections; this.providers = catalog.providers;
    if (this.tenantID) {
      const profile = await backend.fetchTenant(this.tenantID, this.controller.signal);
      this.profile = profile;
    }
    this.emitContext();
  }
  emitContext() {
    this.dispatchEvent(new CustomEvent(CONNECTION_CONTEXT_EVENT, {bubbles:true, detail:{tenantID:this.tenantID,tenants:this.tenants,profile:this.profile}}));
  }
  /** @param {string} id */
  async selectTenant(id) {
    const revision = ++this.revision;
    this.tenantID = id; this.connectionID = ''; this.modelID = ''; this.secret = ''; this.requestExample = ''; this.profile = null;
    this.message = ''; this.failure = ''; this.render();
    if (id) {
      const profile = await backend.fetchTenant(id, this.controller.signal);
      if (revision !== this.revision) return;
      this.profile = profile;
      this.connectionID = this.connections.find(c => c.tenant_ids.includes(id))?.id || '';
    }
    this.emitContext(); this.render();
  }
  /** @param {() => Promise<void>} action */
  async run(action) {
    if (this.busy) return;
    this.busy = true; this.failure = ''; this.message = ''; this.setBusy(); this.updateNotice();
    try { await action(); }
    catch (error) {
      if (this.controller.signal.aborted) return;
      const failure = profileFailureMessage(error) === COPY.appIntegrityError ? profileFailureMessage(error) : error instanceof backend.BackendClientError ? `The change failed (${error.status}). ${backend.managementFailureMessage(error)}` : 'Unable to complete this change. Try again.';
      this.failure = this.message ? `${this.message} ${failure}` : failure;
    } finally {
      this.busy = false;
      if (this.isConnected) {
        const form = this.querySelector('[data-default-form], [data-provider-profile]');
        const draft = this.failure && form instanceof HTMLFormElement ? new FormData(form) : null;
        const selector = form?.hasAttribute('data-default-form') ? '[data-default-form]' : '[data-provider-profile]';
        this.render();
        if (draft) {
          const renderedForm = this.querySelector(selector);
          if (renderedForm instanceof HTMLFormElement) for (const [name, value] of draft) {
            const input = renderedForm.elements.namedItem(name);
            if (typeof value === 'string' && (input instanceof HTMLInputElement || input instanceof HTMLTextAreaElement || input instanceof HTMLSelectElement)) input.value = value;
          }
        }
        if(!this.querySelector('dialog'))this.restoreDialogFocus();
      }
    }
  }
  setBusy() { this.setAttribute('aria-busy',String(this.busy)); }
  render() {
    this.setBusy();
    const dialog = this.querySelector('dialog[open]');
    if (dialog) { this.updateNotice(); return; }
    this.innerHTML = `<section class="connection-dashboard" aria-label="Tenant connections">
      <header class="cw-header"><div><p class="eyebrow">Your dashboard</p><h2>Tenants → connections → models</h2></div>
      <button data-action="copy-mcp" ${this.busy?'disabled':''}>Copy MCP URL</button><label><span class="visually-hidden">Search dashboard</span><input name="dashboard-search" type="search" placeholder="Find a tenant, connection, or model" value="${escapeHTML(this.search)}"></label></header>
      <div class="cw-map" data-map></div><footer class="cw-route" data-route></footer>
      <p class="cw-notice" role="status" aria-live="polite" data-notice></p>
      <section class="cw-details" aria-label="Selection details" data-details></section>
    </section>`;
    this.renderMap(); this.renderDetails(); this.updateNotice();
  }
  updateNotice() {
    this.querySelectorAll('[data-notice]').forEach(notice => { notice.textContent = this.failure || (this.busy ? 'Saving…' : this.message); notice.setAttribute('role',this.failure ? 'alert':'status'); });
    this.querySelectorAll('dialog button[type="submit"]').forEach(button => { if (button instanceof HTMLButtonElement) button.disabled=this.busy; });
  }
  renderMap() {
    const map = this.querySelector('[data-map]'); if (!map) return;
    const matches = (/** @type {string} */ text) => text.toLowerCase().includes(this.search.toLowerCase());
    const disabled = this.busy ? 'disabled':'';
    const modelIDs = this.attached && this.ready && this.provider ? (this.capability === 'text' ? this.provider.text_models.map(m=>m.id) : this.provider.dictation_models) : [];
    map.innerHTML = `<svg class="cw-wires" aria-hidden="true"></svg>
      <section class="cw-column"><header><h3>Tenants <span>${this.tenants.length}</span></h3><button data-action="create-tenant" ${disabled}>Create tenant</button></header>
      <button class="cw-account ${this.tenantID ? '':'selected'}" data-action="account">Account usage</button><div class="cw-list">
      ${this.tenants.filter(t=>matches(t.name)).map(t=>{const count=this.connections.filter(c=>c.tenant_ids.includes(t.id)).length;return `<button class="cw-node ${t.id===this.tenantID?'selected':''}" data-tenant="${escapeHTML(t.id)}" aria-pressed="${t.id===this.tenantID}" ${disabled}><strong>${escapeHTML(t.name)}</strong><small>${count ? `${count} connection${count===1?'':'s'}`:'No connections yet'}</small></button>`;}).join('')}</div></section>
      <section class="cw-column"><header><h3>Connections <span>${this.connections.length}</span></h3><button data-action="create-connection" ${disabled}>Create connection</button></header><div class="cw-list">
      ${this.connections.filter(c=>matches(c.name+' '+c.provider)).map(c=>{
        const assigned=c.tenant_ids.includes(this.tenantID); const occupied=this.connections.some(other=>other.provider===c.provider && other.tenant_ids.includes(this.tenantID));
        return `<article class="cw-node ${c.id===this.connectionID?(assigned?'selected':'browsing'):''}" data-connection-node="${escapeHTML(c.id)}"><div class="cw-row"><button class="cw-name" data-connection="${escapeHTML(c.id)}">${providerIcon(c.provider)}<strong>${escapeHTML(c.name)}</strong></button>${this.tenantID ? assigned ? '<span class="cw-connected">✓ Connected</span>' : occupied ? '<span class="cw-muted">Provider already connected</span>' : `<button class="cw-connect" data-connect="${escapeHTML(c.id)}" ${disabled}>Connect</button>` : ''}</div><small>${escapeHTML(this.providers.find(p=>p.id===c.provider)?.api_service_label || c.provider)}</small><small>${c.tenant_ids.length ? `Used by ${c.tenant_ids.length} tenant${c.tenant_ids.length===1?'':'s'}`:'Unassigned'}</small>${connectionReady(c)?'':'<small>Credentials needed</small>'}</article>`;
      }).join('') || '<p class="cw-empty">Create a connection to add provider credentials.</p>'}</div></section>
      <section class="cw-column"><header><h3>Models</h3></header><div class="cw-tabs" role="group" aria-label="Model capability"><button data-capability="text" aria-pressed="${this.capability==='text'}">Text</button><button data-capability="dictation" aria-pressed="${this.capability==='dictation'}">Dictation</button></div><div class="cw-list">
      ${modelIDs.filter(matches).map(id=>{const saved=this.isDefaultModel(id);return `<button class="cw-node ${this.modelID===id?(saved?'selected':'preview'):''}" data-model="${escapeHTML(id)}" ${disabled}><span class="cw-row"><brand-icon kind="family" identifier="${escapeHTML(this.modelFamilies[id])}"></brand-icon><code>${escapeHTML(id)}</code></span>${saved?'<small class="cw-connected">★ Default model</small>':''}</button>`;}).join('') || `<p class="cw-empty">${this.attached ? this.ready ? 'No models for this capability.' : 'Add credentials to choose models. Edit this connection to complete its setup.' : 'Connect to choose models.<br>Use a Connect button in the middle column.'}</p>`}</div></section>`;
    const footer=this.querySelector('[data-route]');
    if (footer) footer.textContent=this.connection ? `${this.tenantName} ${this.attached?'→':'· Viewing'} ${this.connection.name}${this.attached?'':' · Not connected'}${this.modelID?(this.isDefaultModel(this.modelID)?' · Saved default: ':' · Preview: ')+this.modelID:''}` : `${this.tenantName} · Select a connection`;
    requestAnimationFrame(()=>this.drawRoutes());
  }
  drawRoutes() {
    const svg=this.querySelector('.cw-wires'); const map=this.querySelector('[data-map]');
    if (!(svg instanceof SVGElement) || !map) return;
    svg.replaceChildren(); const bounds=map.getBoundingClientRect();
    if (bounds.width<700) return;
    const tenant=this.querySelector(`[data-tenant="${CSS.escape(this.tenantID)}"]`);
    const line=(/** @type {Element} */ from,/** @type {Element} */ to,/** @type {boolean} */ preview=false)=>{
      const a=from.getBoundingClientRect(),b=to.getBoundingClientRect();
      const x=a.right-bounds.left,y=a.top+a.height/2-bounds.top,x2=b.left-bounds.left,y2=b.top+b.height/2-bounds.top;
      const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',`M ${x} ${y} C ${x+25} ${y}, ${x2-25} ${y2}, ${x2} ${y2}`);if(preview)path.classList.add('preview');svg.append(path);
    };
    if (tenant) for (const connection of this.connections.filter(c=>c.tenant_ids.includes(this.tenantID))) {
      const node=this.querySelector(`[data-connection-node="${CSS.escape(connection.id)}"]`);if(node)line(tenant,node);
    }
    if (this.attached) {
      const connection=this.querySelector(`[data-connection-node="${CSS.escape(this.connectionID)}"]`);
      if(connection) this.querySelectorAll('[data-model]').forEach(model=>line(connection,model,model.getAttribute('data-model')===this.modelID && !this.isDefaultModel(this.modelID)));
    }
  }
  renderDetails() {
    const details=this.querySelector('[data-details]');if(!details)return;
    const disabled=this.busy?'disabled':'';
    const c=this.connection;
    if(c) {
      details.innerHTML=`<header class="cw-row"><h3>${escapeHTML(c.name)}</h3><button data-action="edit-connection" ${disabled}>Edit connection</button>${this.attached?`<button data-action="detach" ${disabled}>Detach from ${escapeHTML(this.tenantName)}</button>`:''}${c.tenant_ids.length===0?`<button data-action="delete-connection" ${disabled}>Delete connection</button>`:''}</header>
      <p class="cw-muted">${c.tenant_ids.length?'Used by '+c.tenant_ids.map(id=>escapeHTML(this.tenants.find(t=>t.id===id)?.name || id)).join(', '):'Unassigned'}</p>
      <dl class="cw-fields">${c.fields.map(f=>`<div><dt>${escapeHTML(f.label)}</dt><dd>${escapeHTML(f.secret?(f.masked_value||'Not configured'):f.value)}</dd></div>`).join('')}</dl>
      ${this.attached && this.modelID ? this.modelDetails() : ''}
      ${this.attached ? `<form data-provider-profile><label>Provider system prompt for ${escapeHTML(this.tenantName)}<textarea name="provider_prompt">${escapeHTML(this.profile?.providers.find(p=>p.id===c.provider)?.system_prompt || '')}</textarea></label><button type="button" data-action="save-provider-profile" ${disabled}>Save provider prompt</button></form>` : ''}`;
    } else {
      details.innerHTML=`<header class="cw-row"><h3>${escapeHTML(this.tenantName)}</h3>${this.tenantID?`<button data-action="rename-tenant" ${disabled}>Rename tenant</button><button data-action="tenant-access" ${disabled}>API access</button><button data-action="delete-tenant" ${this.tenants.length===1?'disabled':disabled}>Delete tenant</button>`:''}</header><p class="cw-muted">${this.tenantID?'Connect providers to keep this tenant’s usage separate.':'Choose a tenant to manage its connections.'}</p>`;
    }
    if(this.tenantID && c) details.insertAdjacentHTML('beforeend',`<footer class="cw-row"><span>${escapeHTML(this.tenantName)}</span><button data-action="tenant-details">Tenant details and API access</button></footer>`);
  }
  modelDetails() {
    const model=this.provider?.text_models.find(m=>m.id===this.modelID);
    const efforts=this.capability==='text'?(model?.reasoning_effort?.efforts||[]):[];
    const defaults=this.profile?.tenant.defaults;
    return `<form data-default-form><h4>${escapeHTML(this.modelID)}</h4><p class="cw-muted">${this.isDefaultModel(this.modelID)?'Saved default':'Preview'} · Save to change ${escapeHTML(this.tenantName)}’s ${this.capability} default.</p>
    ${efforts.length?`<label>Reasoning effort<select aria-label="Reasoning effort" name="reasoning_effort"><option value="">Provider default</option>${efforts.map(e=>`<option ${e===defaults?.reasoning_effort?'selected':''}>${escapeHTML(e)}</option>`).join('')}</select></label>`:''}
    ${this.capability==='text'?`<label>Tenant system prompt<textarea name="system_prompt">${escapeHTML(defaults?.system_prompt||'')}</textarea></label>`:''}
    <button class="cw-primary" data-action="save-default" type="button" ${this.busy?'disabled':''}>Save ${this.capability} default</button></form>`;
  }
  /** @param {MouseEvent} event */
  handleClick(event) {
    const button=event.target instanceof Element?event.target.closest('button'):null;if(!button || this.busy)return;
    if(button.dataset.tenant) {void this.run(()=>this.selectTenant(button.dataset.tenant || ''));return;}
    if(button.dataset.connection) {this.connectionID=button.dataset.connection;this.modelID='';this.render();return;}
    if(button.dataset.connect) {const id=button.dataset.connect;void this.run(async()=>{const c=this.connections.find(c=>c.id===id);if(!c)return;await backend.assignConnection(this.tenantID,c.provider,c.id,this.controller.signal);this.connectionID=id;this.modelID='';await this.reload();this.message=`Connected ${this.tenantName} to ${c.name}. Choose a model.`;});return;}
    if(button.dataset.model) {this.modelID=button.dataset.model;this.render();return;}
    if(button.dataset.capability) {this.capability=button.dataset.capability;this.modelID='';this.render();return;}
    switch(button.dataset.action) {
      case 'copy-mcp':void this.run(async()=>{const runtime=await backend.loadFrontendRuntimeConfig();await navigator.clipboard.writeText(new URL(MCP_PATH,runtime.proxyOrigin).href);this.message='MCP URL copied.';});break;
      case 'account': void this.run(()=>this.selectTenant(''));break;
      case 'tenant-details':this.connectionID='';this.modelID='';this.render();break;
      case 'create-tenant':this.tenantForm(false);break;
      case 'rename-tenant':this.tenantForm(true);break;
      case 'create-connection':this.connectionForm(false);break;
      case 'edit-connection':this.connectionForm(true);break;
      case 'detach':this.confirmDetach();break;
      case 'delete-connection':this.confirm('Delete connection',`Delete ${this.connection?.name}?`,async()=>{await backend.deleteConnection(this.connectionID,this.controller.signal);this.connectionID='';await this.reload();});break;
      case 'delete-tenant':this.confirm('Delete tenant',`Delete ${this.tenantName} and its usage history? Shared connections remain available.`,async()=>{await backend.deleteTenant(this.tenantID,this.controller.signal);this.tenantID='';await this.reload();await this.selectTenant(this.tenants[0].id);});break;
      case 'save-default':void this.run(()=>this.saveDefault());break;
      case 'save-provider-profile':void this.run(()=>this.saveProviderPrompt());break;
      case 'tenant-access':this.accessDialog();break;
      case 'close-dialog':this.closeDialog();break;
      case 'copy-secret':void navigator.clipboard.writeText(this.secret).then(()=>{this.message='API key copied.';this.updateNotice();}).catch(()=>{this.failure='Unable to copy the key.';this.updateNotice();});break;
      case 'copy-example':void navigator.clipboard.writeText(this.requestExample).then(()=>{this.message='Request example copied.';this.updateNotice();}).catch(()=>{this.failure='Unable to copy the example.';this.updateNotice();});break;
      case 'rotate-secret':if(this.profile?.tenant.has_secret)this.confirm('Replace API key','Existing clients must use the new key after replacement.',()=>this.rotateSecret());else void this.run(()=>this.rotateSecret());break;
    }
  }
  async saveProviderPrompt() {
    const c=this.connection;const form=this.querySelector('[data-provider-profile]');
    if(!c || !this.attached || !(form instanceof HTMLFormElement))return;
    const provider=this.profile?.providers.find(p=>p.id===c.provider);if(!provider)return;
    await backend.saveTenantProviderProfile(this.tenantID,c.provider,{text_model:provider.text_model,system_prompt:String(new FormData(form).get('provider_prompt')||'')},this.controller.signal);
    await this.reload();this.message='Provider prompt saved for this tenant.';
  }
  async saveDefault() {
    if(!this.profile || !this.connection || !this.attached)return;
    const form=this.querySelector('[data-default-form]');if(!(form instanceof HTMLFormElement))return;
    const values=new FormData(form);const defaults={...this.profile.tenant.defaults};
    if(this.capability==='text') {defaults.provider=this.connection.provider;defaults.model=this.modelID;defaults.reasoning_effort=String(values.get('reasoning_effort')||'');defaults.system_prompt=String(values.get('system_prompt')||'');}
    else {defaults.dictation_provider=this.connection.provider;defaults.dictation_model=this.modelID;}
    await backend.updateDefaults(this.tenantID,defaults,this.controller.signal);await this.reload();this.message='Default saved.';this.modelID='';
  }
  /** @param {string} title @param {string} body @param {(form: HTMLFormElement)=>Promise<void>} [submit] */
  dialog(title,body,submit) {
    const previousDialog=this.querySelector('dialog');
    if(!previousDialog)this.returnFocus=document.activeElement instanceof HTMLElement?document.activeElement:null;
    previousDialog?.remove();
    const dialog=document.createElement('dialog');dialog.className='cw-dialog';
    dialog.setAttribute('aria-label',title);
    dialog.innerHTML=`<form><header class="cw-row"><h3>${escapeHTML(title)}</h3><button type="button" data-action="close-dialog" aria-label="Close dialog" class="icon-only"><svg class="utility-icon close-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true" focusable="false"><path d="M6 6l12 12"></path><path d="M18 6L6 18"></path></svg></button></header>${body}<p data-notice role="status" aria-live="polite"></p></form>`;
    this.append(dialog);dialog.showModal();
    dialog.addEventListener('cancel',e=>{e.preventDefault();if(!this.busy)this.closeDialog();});
    const form=dialog.querySelector('form');
    if(submit && form)form.addEventListener('submit',e=>{e.preventDefault();void this.run(async()=>{await submit(form);if(dialog.isConnected)this.closeDialog();});});
  }
  closeDialog() {
    this.querySelector('dialog')?.remove();this.secret='';this.requestExample='';
    if(this.busy)return;
    this.render();this.restoreDialogFocus();
  }
  restoreDialogFocus() {
    if(!this.returnFocus)return;
    const action=this.returnFocus.dataset.action;
    const currentButton=Array.from(this.querySelectorAll('button')).find(button=>action && button.dataset.action===action);
    if(currentButton)currentButton.focus();
    else if(this.returnFocus?.isConnected)this.returnFocus.focus();
    else this.querySelector('button')?.focus();
    this.returnFocus=null;
  }
  /** @param {string} title @param {string} message @param {()=>Promise<void>} action */
  confirm(title,message,action) {this.dialog(title,`<p>${escapeHTML(message)}</p><button type="submit" class="cw-primary">Confirm</button>`,action);}
  confirmDetach() {
    const c=this.connection;if(!c)return;
    const d=this.profile?.tenant.defaults;const affected=d?.provider===c.provider || d?.dictation_provider===c.provider;
    this.confirm('Detach connection',`Detach ${c.name} from ${this.tenantName}? ${affected?'Its saved default routes will be cleared. ':''}Other tenants keep their assignments.`,async()=>{await backend.detachConnection(this.tenantID,c.provider,affected,this.controller.signal);this.connectionID='';this.modelID='';await this.reload();this.message='Connection detached.';});
  }
  /** @param {boolean} rename */
  tenantForm(rename) {
    this.dialog(rename?'Rename tenant':'Create tenant',`<label>Tenant name<input name="name" required maxlength="80" value="${rename?escapeHTML(this.tenantName):''}" autofocus></label>${rename?'':`<label>Next step<select aria-label="Next step" name="next"><option value="existing">Use an existing connection</option><option value="new">Create a new connection</option><option value="later">Configure later</option></select></label>`}<button class="cw-primary" type="submit">${rename?'Save name':'Create tenant'}</button>`,async form=>{
      const data=new FormData(form);const name=String(data.get('name'));
      const profile=rename?await backend.renameTenant(this.tenantID,name,this.controller.signal):await backend.createTenant(name,this.controller.signal);
      this.tenantID=profile.tenant.id;await this.reload();this.connectionID='';this.modelID='';this.message=data.get('next')==='later'?'Tenant created. You can connect providers later.':'Tenant ready. Use Connect on an existing connection.';
      if(data.get('next')==='new') {setTimeout(()=>this.connectionForm(false),0);}
    });
  }
  /** @param {boolean} edit */
  connectionForm(edit) {
    const idempotencyKey=edit?undefined:crypto.randomUUID();
    const c=edit?this.connection:null;let provider=this.providers.find(p=>p.id===c?.provider)||this.providers[0];if(!provider)return;
    const body=`<label>Connection name<input name="name" required maxlength="80" value="${escapeHTML(c?.name||'')}" autofocus></label><label>Provider<select aria-label="Provider" name="provider" ${edit?'disabled':''}>${this.providers.map(p=>`<option value="${escapeHTML(p.id)}" ${p.id===provider.id?'selected':''}>${escapeHTML(p.api_service_label)}</option>`).join('')}</select></label><div data-credential-fields></div>${c?`<p>Changes affect: ${c.tenant_ids.map(id=>escapeHTML(this.tenants.find(t=>t.id===id)?.name||id)).join(', ')||'No tenants'}</p>`:this.tenantID?`<label class="cw-row"><input type="checkbox" name="attach" checked>Connect to ${escapeHTML(this.tenantName)}</label>`:''}<button class="cw-primary" type="submit">${edit?'Save connection':'Create connection'}</button>`;
    this.dialog(edit?'Edit connection':'Create connection',body,async form=>{
      const data=new FormData(form);
      /** @type {Record<string,string>} */
      const fields={};for(const field of provider.fields)fields[field.id]=String(data.get('field-'+field.id)||'');
      const result=await backend.saveConnection(c?.id||'',{name:String(data.get('name')),provider:provider.id,fields,version:c?.version||0},this.controller.signal,idempotencyKey);
      this.connectionID=result.id;
      this.connections=[...this.connections.filter(connection=>connection.id!==result.id),result];
      this.closeDialog();
      this.message=edit?'Connection saved.':'Connection created. Use Connect to attach it to this tenant.';
      if(!edit && data.get('attach'))await backend.assignConnection(this.tenantID,result.provider,result.id,this.controller.signal);
      await this.reload();this.message='Connection saved.';
    });
    const renderFields=()=>{
      const host=this.querySelector('[data-credential-fields]');if(!host)return;
      host.innerHTML=`<p><a href="${escapeHTML(provider.key_acquisition_url)}" target="_blank" rel="noopener noreferrer">Get ${escapeHTML(provider.label)} credentials ↗</a></p>${provider.fields.map(field=>{const saved=c?.fields.find(f=>f.id===field.id);return `<label>${escapeHTML(field.label)}<input name="field-${escapeHTML(field.id)}" type="${field.secret?'password':field.type==='url'?'url':'text'}" autocomplete="off" ${field.required && !(edit&&field.secret&&saved?.configured)?'required':''} value="${field.secret?'':escapeHTML(saved?.value||field.default)}" placeholder="${escapeHTML(saved?.masked_value||(field.secret?'Enter credential':''))}"></label>`;}).join('')}${edit?'<p class="cw-muted">Leave a saved credential blank to retain it.</p>':''}`;
    };
    renderFields();this.querySelector('select[name="provider"]')?.addEventListener('change',event=>{if(event.target instanceof HTMLSelectElement){const value=event.target.value;const next=this.providers.find(p=>p.id===value);if(next){provider=next;renderFields();}}});
  }
  accessDialog() {
    this.dialog(`${this.tenantName} API access`,`<p>${this.profile?.tenant.has_secret?'This tenant has an API key. Replace it to get a new key.':'Create an API key for requests through this tenant.'}</p><button type="button" data-action="rotate-secret">${this.profile?.tenant.has_secret?'Replace API key':'Create API key'}</button><p>Usage is recorded for ${escapeHTML(this.tenantName)} regardless of shared provider connections.</p>`);
  }
  async rotateSecret() {
    const response=await backend.generateSecret(this.tenantID,this.controller.signal);this.secret=response.secret;await this.reload();
    const config=await backend.loadFrontendRuntimeConfig();
    const profile=this.profile;
    const defaults=profile?.tenant.defaults;
    if(profile && defaults?.provider && defaults.model) {
      const url=new URL(profile.proxy.v2_path,config.proxyOrigin);
      url.searchParams.set('key','<generated-secret>');url.searchParams.set('provider',defaults.provider);
      const body=JSON.stringify({model:defaults.model,messages:[{role:'user',content:'Hello'}]});
      this.requestExample=`curl '${url}' -H 'Content-Type: application/json' -d '${body}'`;
    } else this.requestExample='';
    this.dialog('Save your tenant API key',`<p>This key is shown once. Copy it before closing.</p><input readonly aria-label="Tenant API key"><button type="button" data-action="copy-secret">Copy API key</button>${this.requestExample?`<pre>${escapeHTML(this.requestExample)}</pre><button type="button" data-action="copy-example">Copy request example</button>`:'<p>Save a text default to get a request example for this tenant.</p>'}`);
    const keyInput=this.querySelector('input[aria-label="Tenant API key"]');
    if(keyInput instanceof HTMLInputElement)keyInput.value=this.secret;
  }
}

customElements.define('connection-dashboard', ConnectionDashboard);
