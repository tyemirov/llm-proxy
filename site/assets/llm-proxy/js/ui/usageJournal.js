// @ts-check
import * as backend from '../core/backendClient.js?v=20260903f037';
import {profileFailureMessage} from '../core/managementProfile.js?v=20260903f037';

/** @param {unknown} value */
function escapeHTML(value) { return String(value).replace(/[&<>"']/g, character=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character] || character)); }
/** @param {string} value */
function label(value) { return value.replaceAll('_',' '); }
/** @param {string} value */
function title(value) { const text=label(value); return text.charAt(0).toUpperCase()+text.slice(1); }

class UsageJournal extends HTMLElement {
  accountID='';
  controller=new AbortController();
  busy=false;
  loaded=false;
  failure='';
  /** @type {import('../types.d.js').JournalRequestPage} */ page={requests:[],next_cursor:''};
  /** @type {import('../types.d.js').JournalEvidence|null} */ evidence=null;

  connectedCallback() {
    this.accountID=this.getAttribute('billing-account-id') || '';
    this.controller=new AbortController();
    this.addEventListener('click',event=>{
      if (!(event.target instanceof Element)) return;
      const button=event.target.closest('button');
      if (!button || this.busy) return;
      const requestID=button.dataset.journalRequest;
      if (requestID) { void this.run(()=>this.selectRequest(requestID)); return; }
      switch(button.dataset.journalAction) {
        case 'refresh':void this.run(()=>this.loadRequests());break;
        case 'requests':void this.run(()=>this.loadRequests(this.page.next_cursor));break;
        case 'attempts':void this.run(()=>this.loadMoreAttempts());break;
        case 'observations':void this.run(()=>this.loadMoreObservations());break;
        case 'cases':void this.run(()=>this.loadMoreCases());break;
      }
    },{signal:this.controller.signal});
    this.render();
  }
  disconnectedCallback() { this.controller.abort(); this.page={requests:[],next_cursor:''}; this.evidence=null; this.loaded=false; }
  /** @param {()=>Promise<void>} action */
  async run(action) {
    const focus=document.activeElement instanceof HTMLButtonElement ? document.activeElement : null;
    const selector=focus?.dataset.journalRequest ? `[data-journal-request="${CSS.escape(focus.dataset.journalRequest)}"]` : focus?.dataset.journalAction ? `[data-journal-action="${CSS.escape(focus.dataset.journalAction)}"]` : '';
    this.busy=true; this.failure=''; this.render();
    try { await action(); }
    catch(error) { if (!this.controller.signal.aborted) this.failure=profileFailureMessage(error); }
    finally {
      if (this.isConnected) {
        this.busy=false; this.render();
        const replacement=selector ? this.querySelector(selector) : null;
        if (replacement instanceof HTMLButtonElement) replacement.focus();
      }
    }
  }
  /** @param {string} [cursor] */
  async loadRequests(cursor='') {
    if (!cursor) {this.evidence=null;this.page={requests:[],next_cursor:''};this.loaded=false;}
    const page=await backend.fetchJournalRequests(this.accountID,cursor,this.controller.signal);
    this.page={requests:[...this.page.requests,...page.requests],next_cursor:page.next_cursor};this.loaded=true;
  }
  /** @param {string} requestID */
  async selectRequest(requestID) {
    this.evidence=null;
    const [request,attempts,observations,cases]=await Promise.all([
      backend.fetchJournalRequest(this.accountID,requestID,this.controller.signal),
      backend.fetchJournalAttempts(this.accountID,requestID,'',this.controller.signal),
      backend.fetchJournalObservations(this.accountID,requestID,'',this.controller.signal),
      backend.fetchJournalCases(this.accountID,requestID,'',this.controller.signal),
    ]);
    this.evidence={request,attempts,observations,cases};
    this.page.requests=this.page.requests.map(value=>value.id===request.id?request:value);
  }
  async loadMoreAttempts() {
    if (!this.evidence) return;
    const page=await backend.fetchJournalAttempts(this.accountID,this.evidence.request.id,this.evidence.attempts.next_cursor,this.controller.signal);
    this.evidence.attempts={attempts:[...this.evidence.attempts.attempts,...page.attempts],next_cursor:page.next_cursor};
  }
  async loadMoreObservations() {
    if (!this.evidence) return;
    const page=await backend.fetchJournalObservations(this.accountID,this.evidence.request.id,this.evidence.observations.next_cursor,this.controller.signal);
    this.evidence.observations={observations:[...this.evidence.observations.observations,...page.observations],next_cursor:page.next_cursor};
  }
  async loadMoreCases() {
    if (!this.evidence) return;
    const page=await backend.fetchJournalCases(this.accountID,this.evidence.request.id,this.evidence.cases.next_cursor,this.controller.signal);
    this.evidence.cases={cases:[...this.evidence.cases.cases,...page.cases],next_cursor:page.next_cursor};
  }
  /** @param {string} kind @param {string} cursor */
  moreButton(kind,cursor) { return cursor?`<button data-journal-action="${kind}" ${this.busy?'disabled':''}>Load more ${kind}</button>`:''; }
  render() {
    this.innerHTML=`<section class="usage-journal cw-details" aria-label="Usage journal" aria-busy="${this.busy}">
      <header class="cw-row"><h3>Usage journal</h3><button data-journal-action="refresh" ${this.busy?'disabled':''}>${this.loaded?'Refresh':'Load'} usage journal</button></header>
      <p>Hosted request history and measured usage. Charges are not available yet.</p>
      ${this.failure?`<p role="alert">${escapeHTML(this.failure)}</p>`:''}
      ${this.busy?'<p role="status">Loading journal…</p>':''}
      ${this.loaded&&!this.page.requests.length?'<p>No hosted requests yet.</p>':''}
      <ul class="journal-list">${this.page.requests.map(request=>`<li><strong>${escapeHTML(request.provider)} · ${escapeHTML(request.model || 'Provider service')}</strong>
        <p>${escapeHTML(label(request.operation))} · ${escapeHTML(title(request.state))} · Usage ${escapeHTML(request.usage_state)}</p>
        <p>Tenant: ${escapeHTML(request.tenant_id)}</p><code>${escapeHTML(request.id)}</code>
        <button data-journal-request="${escapeHTML(request.id)}" ${this.busy?'disabled':''}>View request</button></li>`).join('')}</ul>
      ${this.moreButton('requests',this.page.next_cursor)}
      ${this.evidence?this.renderEvidence(this.evidence):''}
    </section>`;
  }
  /** @param {import('../types.d.js').JournalEvidence} evidence */
  renderEvidence(evidence) {
    const {request,attempts,observations,cases}=evidence;
    return `<section class="journal-evidence" aria-label="Request evidence"><h4>Request evidence</h4><code>${escapeHTML(request.id)}</code>
      <p>${escapeHTML(title(request.state))} · Usage ${escapeHTML(request.usage_state)}</p>${request.failure_code?`<p>${escapeHTML(label(request.failure_code))}</p>`:''}
      <h4>Attempts</h4>${!attempts.attempts.length?'<p>No provider attempts recorded.</p>':''}
      <ul class="journal-list">${attempts.attempts.map(attempt=>`<li><strong>Attempt ${attempt.number} · ${escapeHTML(title(attempt.state))}</strong><code>${escapeHTML(attempt.id)}</code><time datetime="${escapeHTML(attempt.created_at)}">${escapeHTML(attempt.created_at)}</time></li>`).join('')}</ul>
      ${this.moreButton('attempts',attempts.next_cursor)}
      <h4>Usage observations</h4>${!observations.observations.length?'<p>No usage measurements recorded.</p>':''}
      <ul class="journal-list">${observations.observations.map(observation=>`<li><strong>${escapeHTML(title(observation.completeness))} usage</strong><code>${escapeHTML(observation.id)}</code><p>Attempt: <code>${escapeHTML(observation.attempt_id)}</code></p><dl>${observation.quantities.map(quantity=>`<dt>${escapeHTML(label(quantity.dimension))}</dt><dd>${quantity.value===undefined?`Unknown (${escapeHTML(label(quantity.unknown_reason || ''))})`:`${escapeHTML(quantity.value)} ${escapeHTML(quantity.unit)}`}${quantity.included_in?`<p class="cw-muted">Included in ${escapeHTML(label(quantity.included_in))}</p>`:''}</dd>`).join('')}</dl></li>`).join('')}</ul>
      ${this.moreButton('observations',observations.next_cursor)}
      <h4>Reconciliation cases</h4>${!cases.cases.length?'<p>No reconciliation cases recorded.</p>':''}
      <ul class="journal-list">${cases.cases.map(record=>`<li><strong>${escapeHTML(title(record.reason))} · ${escapeHTML(title(record.state))}</strong><code>${escapeHTML(record.id)}</code>${record.resolved_at?`<time datetime="${escapeHTML(record.resolved_at)}">${escapeHTML(record.resolved_at)}</time>`:''}</li>`).join('')}</ul>
      ${this.moreButton('cases',cases.next_cursor)}</section>`;
  }
}
customElements.define('usage-journal',UsageJournal);
