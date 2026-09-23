// @ts-check
import * as backend from '../core/backendClient.js?v=20260903f037';
import {profileFailureMessage} from '../core/managementProfile.js?v=20260903f037';

/** @param {unknown} value */
function escapeHTML(value) { return String(value).replace(/[&<>"']/g,character=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character] || character)); }
/** @param {string} cents */
function dollars(cents) { const value=BigInt(cents);return `$${value/100n}.${String(value%100n).padStart(2,'0')}`; }
/** @param {string} value */
function title(value) { const text=value.replaceAll('_',' ');return text.charAt(0).toUpperCase()+text.slice(1); }

class PaymentHistory extends HTMLElement {
  accountID='';
  controller=new AbortController();
  busy=false;
  loaded=false;
  failure='';
  /** @type {import('../types.d.js').FundingOrderPage} */ page={orders:[],next_cursor:''};
  /** @type {import('../types.d.js').PaymentReceipt|null} */ receipt=null;
  /** @type {Window|null} */ pendingPortal=null;

  connectedCallback() {
    this.accountID=this.getAttribute('billing-account-id') || '';
    this.controller=new AbortController();
    this.addEventListener('click',event=>{
      if (!(event.target instanceof Element) || this.busy) return;
      const button=event.target.closest('button');
      switch(button?.dataset.paymentAction) {
        case 'refresh':void this.run(()=>this.refresh());break;
        case 'more':void this.run(()=>this.more());break;
        case 'receipt':void this.run(()=>this.readReceipt(button.dataset.orderId || ''));break;
        case 'portal':void this.run(()=>this.openPortal());break;
      }
    },{signal:this.controller.signal});
    this.render();
  }
  disconnectedCallback() {
    this.controller.abort();this.pendingPortal?.close();this.pendingPortal=null;
  }
  /** @param {()=>Promise<void>} action */
  async run(action) {
    const focused=document.activeElement instanceof HTMLButtonElement?document.activeElement:null;
    const focusAction=focused?.dataset.paymentAction,focusOrder=focused?.dataset.orderId;
    this.busy=true;this.failure='';
    const pending=action();this.render();
    try {await pending;}
    catch(error) {if (!this.controller.signal.aborted) this.failure=error instanceof backend.BackendClientError?backend.managementFailureMessage(error):profileFailureMessage(error);}
    finally {
      if (this.isConnected) {
        this.busy=false;this.render();
        const button=focusAction?this.querySelector(`[data-payment-action="${CSS.escape(focusAction)}"]${focusOrder?`[data-order-id="${CSS.escape(focusOrder)}"]`:''}`):null;
        if (button instanceof HTMLButtonElement) button.focus();
      }
    }
  }
  async refresh() {
    this.loaded=false;this.receipt=null;this.page={orders:[],next_cursor:''};
    this.page=await backend.fetchFundingOrders(this.accountID,'',this.controller.signal);this.loaded=true;
  }
  async more() {
    const page=await backend.fetchFundingOrders(this.accountID,this.page.next_cursor,this.controller.signal);
    this.page={orders:[...this.page.orders,...page.orders],next_cursor:page.next_cursor};
  }
  /** @param {string} orderID */
  async readReceipt(orderID) {
    this.receipt=null;
    this.receipt=await backend.fetchPaymentReceipt(this.accountID,orderID,this.controller.signal);
  }
  async openPortal() {
    const popup=window.open('about:blank','_blank');
    if (!popup) {this.failure='Allow a new tab to open Paddle invoices.';return;}
    popup.opener=null;this.pendingPortal=popup;
    try {
      const session=await backend.createPaymentPortalSession(this.accountID,this.controller.signal);
      if (!this.controller.signal.aborted) popup.location.replace(session.url);
    } catch(error) {popup.close();throw error;}
    finally {this.pendingPortal=null;}
  }
  render() {
    const disabled=this.busy?'disabled':'';
    this.innerHTML=`<section class="prepaid-balance cw-details" aria-label="Funding history" aria-busy="${this.busy}">
      <header class="cw-row"><h3>Funding history</h3><button data-payment-action="refresh" ${disabled}>Refresh payments</button></header>
      <p>Funds become available after the server verifies payment. A checkout confirmation alone does not change your balance.</p>
      ${this.failure?`<p role="alert">${escapeHTML(this.failure)}</p>`:''}
      ${this.busy?'<p role="status">Loading payment records…</p>':''}
      ${this.loaded&&!this.page.orders.length?'<p>No payments yet.</p>':''}
      <ul class="funds-history">${this.page.orders.map(order=>`<li data-payment-order="${escapeHTML(order.id)}">
        <strong>${escapeHTML(title(order.state))} · ${dollars(order.funding_cents)} account credit</strong>
        <span>${escapeHTML(title(order.environment))}</span><code>${escapeHTML(order.id)}</code>
        <time datetime="${escapeHTML(order.created_at)}">${escapeHTML(order.created_at)}</time>
        ${['paid','partially_refunded','refunded','disputed'].includes(order.state)?`<button data-payment-action="receipt" data-order-id="${escapeHTML(order.id)}" ${disabled}>View receipt</button>`:''}
      </li>`).join('')}</ul>
      ${this.page.next_cursor?`<button data-payment-action="more" ${disabled}>Load more payments</button>`:''}
      ${this.page.orders.some(order=>order.state!=='created')?`<button data-payment-action="portal" ${disabled}>Open Paddle invoices</button>`:''}
      ${this.receipt?this.renderReceipt(this.receipt):''}
    </section>`;
  }
  /** @param {import('../types.d.js').PaymentReceipt} receipt */
  renderReceipt(receipt) {
    return `<section aria-label="Payment receipt"><h4>Payment receipt</h4>
      <p>${escapeHTML(receipt.invoice_number || 'Invoice number not assigned')} · ${escapeHTML(title(receipt.state))} · ${escapeHTML(title(receipt.environment))}</p>
      <code>${escapeHTML(receipt.funding_order_id)}</code><p><time datetime="${escapeHTML(receipt.paid_at)}">${escapeHTML(receipt.paid_at)}</time></p>
      <p>Original payment: ${dollars(receipt.gross_cents)}</p><p>Original tax: ${dollars(receipt.tax_cents)}</p>
      <p>Account credit: ${dollars(receipt.credit_cents)}</p><p>Current payment: ${dollars(receipt.adjusted_gross_cents)}</p>
      <p>Current tax: ${dollars(receipt.adjusted_tax_cents)}</p><p>Credit reversed: ${dollars(receipt.reversed_cents)}</p>
      <p>Pending credit reversal: ${dollars(receipt.pending_refund_cents)}</p></section>`;
  }
}
customElements.define('payment-history',PaymentHistory);
