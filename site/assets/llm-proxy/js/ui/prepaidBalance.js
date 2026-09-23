// @ts-check
import * as backend from '../core/backendClient.js?v=20260903f037';
import {profileFailureMessage} from '../core/managementProfile.js?v=20260903f037';

/** @param {unknown} value */
function escapeHTML(value) { return String(value).replace(/[&<>"']/g, character=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character] || character)); }
/** @param {string} cents */
function dollars(cents) {
  const value=BigInt(cents), absolute=value<0n?-value:value;
  return `${value<0n?'-':''}$${absolute/100n}.${String(absolute%100n).padStart(2,'0')}`;
}
/** @param {string} value */
function title(value) { const text=value.replaceAll('_',' ');return text.charAt(0).toUpperCase()+text.slice(1); }

class PrepaidBalance extends HTMLElement {
  accountID='';
  tenantID='';
  tenantName='';
  limitDraft='';
  limitFailure='';
  /** @type {import('../types.d.js').FundsTenantLimit|null} */ tenantLimit=null;
  controller=new AbortController();
  busy=false;
  failure='';
  historyLoaded=false;
  /** @type {import('../types.d.js').FundsBalance|null} */ balance=null;
  /** @type {import('../types.d.js').FundsReservationPage} */ reservations={reservations:[],next_cursor:''};
  /** @type {import('../types.d.js').FundsEntryPage} */ entries={entries:[],next_cursor:''};

  connectedCallback() {
    this.accountID=this.getAttribute('billing-account-id') || '';
    this.tenantID=this.getAttribute('tenant-id') || '';
    this.tenantName=this.getAttribute('tenant-name') || '';
    this.controller=new AbortController();
    this.addEventListener('input',event=>{
      if (event.target instanceof HTMLInputElement && event.target.name==='tenant-limit') this.limitDraft=event.target.value;
    },{signal:this.controller.signal});
    this.addEventListener('click',event=>{
      if (!(event.target instanceof Element) || this.busy) return;
      switch(event.target.closest('button')?.dataset.fundsAction) {
        case 'refresh':void this.run(()=>this.loadBalance());break;
        case 'history':void this.run(()=>this.loadHistory());break;
        case 'reservations':void this.run(()=>this.moreReservations());break;
        case 'entries':void this.run(()=>this.moreEntries());break;
        case 'save-limit':void this.run(()=>this.saveLimit(false));break;
        case 'remove-limit':void this.run(()=>this.saveLimit(true));break;
      }
    },{signal:this.controller.signal});
    void this.run(()=>this.loadBalance());
  }
  disconnectedCallback() { this.controller.abort();this.balance=null;this.clearHistory(); }
  clearHistory() { this.historyLoaded=false;this.reservations={reservations:[],next_cursor:''};this.entries={entries:[],next_cursor:''}; }
  /** @param {()=>Promise<void>} action */
  async run(action) {
    const focused=document.activeElement instanceof HTMLButtonElement ? document.activeElement.dataset.fundsAction : '';
    this.busy=true;this.failure='';this.limitFailure='';
    const pending=action();this.render();
    try { await pending; }
    catch(error) {
      if (!this.controller.signal.aborted) this.failure=error instanceof backend.BackendClientError?backend.managementFailureMessage(error):profileFailureMessage(error);
    } finally {
      if (this.isConnected) {
        this.busy=false;this.render();
        const button=focused?this.querySelector(`[data-funds-action="${CSS.escape(focused)}"]`):null;
        if (button instanceof HTMLButtonElement) button.focus();
      }
    }
  }
  async loadBalance() {
    this.balance=null;this.tenantLimit=null;this.clearHistory();
    this.balance=await backend.fetchFundsBalance(this.accountID,this.controller.signal);
    if (this.tenantID) this.setTenantLimit(await backend.fetchFundsTenantLimit(this.accountID,this.tenantID,this.controller.signal));
  }
  /** @param {import('../types.d.js').FundsTenantLimit} value */
  setTenantLimit(value) { this.tenantLimit=value;this.limitDraft=value.limit_cents===null?'':dollars(value.limit_cents).slice(1); }
  /** @param {boolean} remove */
  async saveLimit(remove) {
    if (!this.tenantLimit) return;
    let cents=null;
    if (!remove) {
      const match=/^(0|[1-9][0-9]*)(?:\.([0-9]{1,2}))?$/.exec(this.limitDraft);
      if (!match) { this.limitFailure='Use a nonnegative USD amount with at most two decimal places.';return; }
      cents=(BigInt(match[1])*100n+BigInt((match[2]||'').padEnd(2,'0'))).toString();
      if (BigInt(cents)>9223372036854775807n) { this.limitFailure='This limit is too large.';return; }
    }
    try {
      this.setTenantLimit(await backend.saveFundsTenantLimit(this.accountID,this.tenantID,cents,this.tenantLimit.revision,this.controller.signal));
    } catch(error) {
      if (!(error instanceof backend.BackendClientError) || error.status!==409) throw error;
      this.tenantLimit=null;
      this.setTenantLimit(await backend.fetchFundsTenantLimit(this.accountID,this.tenantID,this.controller.signal));
      this.limitFailure='The limit changed in another session. Review the current value before saving again.';
    }
  }
  async loadHistory() {
    this.clearHistory();
    const [reservations,entries]=await Promise.all([
      backend.fetchFundsReservations(this.accountID,'',this.controller.signal),
      backend.fetchFundsEntries(this.accountID,'',this.controller.signal),
    ]);
    this.reservations=reservations;this.entries=entries;this.historyLoaded=true;
  }
  async moreReservations() {
    const page=await backend.fetchFundsReservations(this.accountID,this.reservations.next_cursor,this.controller.signal);
    this.reservations={reservations:[...this.reservations.reservations,...page.reservations],next_cursor:page.next_cursor};
  }
  async moreEntries() {
    const page=await backend.fetchFundsEntries(this.accountID,this.entries.next_cursor,this.controller.signal);
    this.entries={entries:[...this.entries.entries,...page.entries],next_cursor:page.next_cursor};
  }
  render() {
    const balance=this.balance,disabled=this.busy?'disabled':'';
    this.innerHTML=`<section class="prepaid-balance cw-details" aria-label="Prepaid balance" aria-busy="${this.busy}">
      <header class="cw-row"><h3>Prepaid balance</h3><button data-funds-action="refresh" ${disabled}>Refresh balance</button></header>
      <p>Minimum funding: $5.00. You can spend your balance down to $0.00.</p>
      ${this.failure?`<p role="alert">${escapeHTML(this.failure)}</p>`:''}
      ${this.busy?'<p role="status">Loading funds…</p>':''}
      ${balance?this.renderBalance(balance):''}
      ${this.tenantID?this.renderTenantLimit():''}
      ${balance?`<button data-funds-action="history" ${disabled}>View financial history</button>`:''}
      ${this.historyLoaded?this.renderHistory():''}
    </section>`;
  }
  renderTenantLimit() {
    const value=this.tenantLimit,disabled=this.busy?'disabled':'';
    return `<section aria-label="Tenant spending limit"><h4>Spending limit · ${escapeHTML(this.tenantName)}</h4>
      <p>This optional limit covers total net usage and active reservations. It does not reset automatically. Existing reservations remain authorized.</p>
      ${this.limitFailure?`<p role="alert">${escapeHTML(this.limitFailure)}</p>`:''}
      ${value?`<p>${value.remaining_cents===null?'No tenant limit':`Remaining allowance: ${dollars(value.remaining_cents)}`}</p>
      <label>Limit in USD <input name="tenant-limit" inputmode="decimal" value="${escapeHTML(this.limitDraft)}" ${disabled}></label>
      <button data-funds-action="save-limit" ${disabled}>Save limit</button>
      ${value.limit_cents!==null?`<button data-funds-action="remove-limit" ${disabled}>Remove limit</button>`:''}`:''}</section>`;
  }
  /** @param {import('../types.d.js').FundsBalance} balance */
  renderBalance(balance) {
    /** @type {[keyof Pick<import('../types.d.js').FundsBalance,'posted_cents'|'available_cents'|'reserved_cents'|'spent_cents'|'pending_cents'>,string][]} */
    const fields=[['available_cents','Available'],['reserved_cents','Reserved'],['spent_cents','Spent before credits'],['pending_cents','Pending reconciliation'],['posted_cents','Posted balance']];
    return `<p>Account: ${escapeHTML(title(balance.state))}</p>
      <dl class="funds-totals">${fields.map(([field,label])=>`<div><dt>${label}</dt><dd data-funds-value="${field}">${dollars(balance[field])}</dd></div>`).join('')}</dl>
      <p>Pending reconciliation is included in reserved funds.</p>
      ${balance.state!=='active'?'<p role="alert">Hosted spending is suspended for this account.</p>':BigInt(balance.available_cents)<=0n?'<p>No funds available for hosted requests.</p>':''}
      ${balance.unsettled_fraction.numerator!=='0'?`<p>Unsettled usage: ${escapeHTML(balance.unsettled_fraction.numerator)}/${escapeHTML(balance.unsettled_fraction.denominator)} USD (less than $0.01), carried to the next settlement.</p>`:''}`;
  }
  renderHistory() {
    const disabled=this.busy?'disabled':'';
    return `<section aria-label="Financial history"><h4>Reservations</h4>
      ${!this.reservations.reservations.length?'<p>No reservations yet.</p>':''}
      <ul class="funds-history">${this.reservations.reservations.map(record=>`<li data-funds-reservation="${escapeHTML(record.id)}"><strong>${escapeHTML(title(record.state))} · ${dollars(record.maximum_cents)}</strong><code>${escapeHTML(record.id)}</code><time datetime="${escapeHTML(record.updated_at)}">${escapeHTML(record.updated_at)}</time></li>`).join('')}</ul>
      ${this.reservations.next_cursor?`<button data-funds-action="reservations" ${disabled}>Load more reservations</button>`:''}
      <h4>Ledger entries</h4>${!this.entries.entries.length?'<p>No ledger entries yet.</p>':''}
      <ul class="funds-history">${this.entries.entries.map(record=>`<li data-funds-entry="${escapeHTML(record.id)}"><strong>${escapeHTML(title(record.type))} · ${dollars(record.amount_cents)}</strong><code>${escapeHTML(record.id)}</code><time datetime="${escapeHTML(record.created_at)}">${escapeHTML(record.created_at)}</time>${record.reservation_id?`<span>Reservation: <code>${escapeHTML(record.reservation_id)}</code></span>`:''}${record.refund_of_entry_id?`<span>Credit for entry: <code>${escapeHTML(record.refund_of_entry_id)}</code></span>`:''}</li>`).join('')}</ul>
      ${this.entries.next_cursor?`<button data-funds-action="entries" ${disabled}>Load more ledger entries</button>`:''}</section>`;
  }
}
customElements.define('prepaid-balance',PrepaidBalance);
