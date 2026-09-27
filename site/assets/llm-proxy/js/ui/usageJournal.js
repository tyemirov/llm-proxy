// @ts-check
import * as backend from '../core/backendClient.js?v=20260903f037';
import {profileFailureMessage} from '../core/managementProfile.js?v=20260903f037';
import {formatExactUSD} from '../core/exactMoney.js?v=20260903f037';

/** @param {unknown} value */
function escapeHTML(value) { return String(value).replace(/[&<>"']/g, character=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character] || character)); }
/** @param {string} value */
function label(value) { return value.replaceAll('_',' '); }
/** @param {string} value */
function title(value) { const text=label(value); return text.charAt(0).toUpperCase()+text.slice(1); }
/** @param {import('../types.d.js').ExactMoney} value */
function money(value) { return `<span title="Exact: ${escapeHTML(value.numerator)}/${escapeHTML(value.denominator)} USD">${escapeHTML(formatExactUSD(value))}</span>`; }

class UsageJournal extends HTMLElement {
  accountID='';
  controller=new AbortController();
  busy=false;
  loaded=false;
  failure='';
  /** @type {import('../types.d.js').JournalRequestPage} */ page={requests:[],next_cursor:''};
  /** @type {import('../types.d.js').JournalEvidence|null} */ evidence=null;
  /** @type {import('../types.d.js').CustomerChargePage|null} */ charges=null;
  chargesRequested=false;

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
        case 'charges':void this.run(()=>this.loadCharges());break;
        case 'more-charges':void this.run(()=>this.loadCharges(this.charges?.next_cursor));break;
      }
    },{signal:this.controller.signal});
    this.render();
  }
  disconnectedCallback() { this.controller.abort(); this.page={requests:[],next_cursor:''}; this.evidence=null; this.loaded=false; this.charges=null; this.chargesRequested=false; }
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
    const [request,attempts,observations,cases,summary]=await Promise.all([
      backend.fetchJournalRequest(this.accountID,requestID,this.controller.signal),
      backend.fetchJournalAttempts(this.accountID,requestID,'',this.controller.signal),
      backend.fetchJournalObservations(this.accountID,requestID,'',this.controller.signal),
      backend.fetchJournalCases(this.accountID,requestID,'',this.controller.signal),
      backend.fetchRequestChargeSummary(this.accountID,requestID,this.controller.signal),
    ]);
    this.evidence={request,attempts,observations,cases,summary};
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
  /** @param {string} [cursor] */
  async loadCharges(cursor='') {
    this.chargesRequested=true;
    if (!cursor) this.charges=null;
    const page=await backend.fetchCustomerCharges(this.accountID,cursor,this.controller.signal);
    this.charges={charges:[...(this.charges?.charges || []),...page.charges],next_cursor:page.next_cursor};
  }
  /** @param {string} kind @param {string} cursor */
  moreButton(kind,cursor) { return cursor?`<button data-journal-action="${kind}" ${this.busy?'disabled':''}>Load more ${kind}</button>`:''; }
  render() {
    this.innerHTML=`<section class="usage-journal cw-details" aria-label="Usage journal" aria-busy="${this.busy}">
      <header class="cw-row"><h3>Usage journal</h3><button data-journal-action="refresh" ${this.busy?'disabled':''}>${this.loaded?'Refresh':'Load'} usage journal</button></header>
      <p>Hosted request history and measured usage.</p>
      <button data-journal-action="charges" ${this.busy?'disabled':''}>${this.chargesRequested?'Refresh':'Load'} charges</button>
      ${this.failure?`<p role="alert">${escapeHTML(this.failure)}</p>`:''}
      ${this.busy?'<p role="status">Loading journal…</p>':''}
      ${this.loaded&&!this.page.requests.length?'<p>No hosted requests yet.</p>':''}
      <ul class="journal-list">${this.page.requests.map(request=>`<li><strong>${escapeHTML(request.provider)} · ${escapeHTML(request.model || 'Provider service')}</strong>
        <p>${escapeHTML(label(request.operation))} · ${escapeHTML(title(request.state))} · Usage ${escapeHTML(request.usage_state)}</p>
        <p>Tenant: ${escapeHTML(request.tenant_id)}</p><code>${escapeHTML(request.id)}</code>
        <button data-journal-request="${escapeHTML(request.id)}" ${this.busy?'disabled':''}>View request</button></li>`).join('')}</ul>
      ${this.moreButton('requests',this.page.next_cursor)}
      ${this.evidence?this.renderEvidence(this.evidence):''}
      ${this.chargesRequested?this.renderCharges():''}
    </section>`;
  }
  /** @param {import('../types.d.js').JournalEvidence} evidence */
  renderEvidence(evidence) {
    const {request,attempts,observations,cases,summary}=evidence;
    return `<section class="journal-evidence" aria-label="Request evidence"><h4>Request evidence</h4><code>${escapeHTML(request.id)}</code>
      <p>${escapeHTML(title(request.state))} · Usage ${escapeHTML(request.usage_state)}</p>${request.failure_code?`<p>${escapeHTML(label(request.failure_code))}</p>`:''}
      <section aria-label="Request charges"><h4>Request charges</h4>
      <p>${summary.state==='rated'?'Charge complete':summary.state==='pending'?'Charge pending':'Charge unresolved'}</p>
      ${summary.provider_cost?`<p>Provider cost: ${money(summary.provider_cost)}</p>`:''}
      ${summary.customer_charge&&summary.customer_credits&&summary.net_customer_charge?`<p>Customer charge: ${money(summary.customer_charge)}</p><p>Credits: ${money(summary.customer_credits)}</p><p>Net charge: ${money(summary.net_customer_charge)}</p>`:'<p>The final customer charge is not available yet.</p>'}</section>
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
  renderCharges() {
    return `<section aria-label="Itemized charges"><h4>Itemized charges</h4>
      ${this.charges&&!this.charges.charges.length?'<p>No charges yet.</p>':''}
      <ul class="journal-list">${(this.charges?.charges || []).map(charge=>`<li data-charge-id="${escapeHTML(charge.id)}">
        <strong>${charge.state==='rated'?'Charge complete':'Charge unresolved'}</strong><code>${escapeHTML(charge.id)}</code>
        <p>Request: <code>${escapeHTML(charge.request_id)}</code></p><time datetime="${escapeHTML(charge.created_at)}">${escapeHTML(charge.created_at)}</time>
        ${charge.rating.provider_cost?`<p>Provider cost: ${money(charge.rating.provider_cost)}</p>`:''}
        ${charge.customer_charge&&charge.net_customer_charge?`<p>Customer charge: ${money(charge.customer_charge)}</p><p>Net charge: ${money(charge.net_customer_charge)}</p>`:'<p>The final customer charge is not available yet.</p>'}
        <ul>${charge.rating.lines.map(line=>`<li>${escapeHTML(label(line.dimension))}: ${escapeHTML(line.quantity)} ${escapeHTML(line.quantity_unit)}
          <p>Customer rate: ${money(line.customer_rate)} (${escapeHTML(line.rate_unit)})</p><p>Customer charge: ${money(line.customer_charge)}</p></li>`).join('')}</ul>
        ${charge.rating.minimum_adjustment&&charge.rating.minimum_adjustment.numerator!=='0'?`<p>Minimum charge adjustment: ${money(charge.rating.minimum_adjustment)}</p>`:''}
        ${charge.rating.unresolved_dimensions.length?`<p>Unresolved usage: ${escapeHTML(charge.rating.unresolved_dimensions.map(label).join(', '))}</p>`:''}
        <ul>${charge.customer_adjustments.map(credit=>`<li>Credit: ${money(credit.credit)} · ${escapeHTML(credit.reason)}</li>`).join('')}</ul>
      </li>`).join('')}</ul>
      ${this.charges?.next_cursor?`<button data-journal-action="more-charges" ${this.busy?'disabled':''}>Load more charges</button>`:''}
      </section>`;
  }
}
customElements.define('usage-journal',UsageJournal);
