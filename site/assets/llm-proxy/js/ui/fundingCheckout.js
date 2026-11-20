// @ts-check
import * as backend from '../core/backendClient.js?v=20261119f102';
import {readPaymentContinuation,clearPaymentContinuation} from '../core/paymentContinuation.js?v=20261119f102';
import {openPaddleCheckout} from '../core/paddleCheckout.js?v=20261119f102';
import {profileFailureMessage} from '../core/managementProfile.js?v=20261119f102';
import {FUNDING_CHANGED_EVENT,FUNDING_RESUME_EVENT,APP_INTEGRITY_ERROR} from '../constants.js?v=20261119f102';

const INTENT_PREFIX='llm-proxy:funding-intent:';
const POLL_MILLISECONDS=1000,POLL_LIMIT=30;
/** @param {unknown} value */
function escapeHTML(value) {return String(value).replace(/[&<>"']/g,character=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[character] || character));}
/** @param {string} cents */
function dollars(cents) {const value=BigInt(cents);return `$${value/100n}.${String(value%100n).padStart(2,'0')}`;}
/** @param {AbortSignal} signal */
function nextPoll(signal) {
  return new Promise((resolve,reject)=>{
    signal.throwIfAborted();
    const cancel=()=>{clearTimeout(timer);reject(signal.reason);};
    const timer=setTimeout(()=>{signal.removeEventListener('abort',cancel);resolve(undefined);},POLL_MILLISECONDS);
    signal.addEventListener('abort',cancel,{once:true});
  });
}
/** @param {string} accountID @returns {import('../types.d.js').FundingIntent|null} */
function readIntent(accountID) {
  const encoded=sessionStorage.getItem(INTENT_PREFIX+accountID);
  if (encoded===null) return null;
  const value=JSON.parse(encoded);
  if (!value || typeof value.key!=='string' || !/^[a-f0-9-]{36}$/.test(value.key) || typeof value.offer_code!=='string' || !/^[a-z][a-z0-9_]{0,63}$/.test(value.offer_code) ||
      (value.order_id!==null && (typeof value.order_id!=='string' || !/^funding-[a-f0-9]{32}$/.test(value.order_id)))) throw new Error(APP_INTEGRITY_ERROR);
  return value;
}

class FundingCheckout extends HTMLElement {
  accountID='';controller=new AbortController();busy=false;failure='';message='';statusRequested=false;storageFailure=false;
  /** @type {import('../types.d.js').FundingOffers|null} */ offers=null;
  /** @type {import('../types.d.js').FundingIntent|null} */ intent=null;
  /** @type {import('../core/paymentContinuation.js').PaymentContinuation|null} */ continuation=null;
  /** @type {()=>void} */ closeCheckout=()=>{};
  connectedCallback() {
    this.accountID=this.getAttribute('billing-account-id') || '';this.controller=new AbortController();
    this.addEventListener('click',event=>{
      if (!(event.target instanceof Element) || this.busy || this.storageFailure) return;
      const button=event.target.closest('button');
      switch(button?.dataset.fundingAction) {
        case 'link':void this.run(()=>this.continueSavedLink());break;
        case 'offers':void this.run(()=>this.loadOffers());break;
        case 'create':void this.run(()=>this.create(button.dataset.offer || ''));break;
        case 'resume':void this.run(()=>this.resume());break;
        case 'status':void this.run(()=>this.checkStatus());break;
      }
    },{signal:this.controller.signal});
    window.addEventListener(FUNDING_RESUME_EVENT,event=>{
      if (!(event instanceof CustomEvent) || event.detail.accountID!==this.accountID || this.busy) return;
      void this.run(async()=>{
        const order=await backend.fetchFundingOrder(this.accountID,event.detail.orderID,this.controller.signal);
        if(this.finish(order) || !this.selectRetainedOrder(order))return;
        await this.resume();
      });
    },{signal:this.controller.signal});
    try {this.intent=readIntent(this.accountID);}
    catch(error) {this.storageFailure=true;this.failure=profileFailureMessage(error);}
    this.render();
    try {
      this.continuation=readPaymentContinuation();
      if(this.continuation)void this.run(()=>this.continueSavedLink());
    } catch(error) {clearPaymentContinuation();this.failure=profileFailureMessage(error);this.render();}
  }
  disconnectedCallback() {this.controller.abort();this.closeCheckout();}
  /** @param {()=>Promise<void>} action */
  async run(action) {
    this.busy=true;this.failure='';this.message='';this.render();
    try {await action();}
    catch(error) {if (!this.controller.signal.aborted) this.failure=error instanceof backend.BackendClientError?backend.managementFailureMessage(error):'Unable to complete checkout. Retry this funding request or check its payment status.';}
    finally {if (this.isConnected) {this.busy=false;this.render();if(this.statusRequested) {this.statusRequested=false;if(this.intent?.order_id)void this.run(()=>this.checkStatus());}}}
  }
  async loadOffers() {this.offers=null;this.offers=await backend.fetchFundingOffers(this.accountID,this.controller.signal);}
  /** @param {import('../types.d.js').FundingIntent|null} intent */
  saveIntent(intent) {
    if (intent) sessionStorage.setItem(INTENT_PREFIX+this.accountID,JSON.stringify(intent));
    else sessionStorage.removeItem(INTENT_PREFIX+this.accountID);
    this.intent=intent;this.storageFailure=false;
  }
  /** @param {string} code */
  async create(code) {
    if (!this.offers?.offers.some(offer=>offer.code===code) || this.intent) throw new Error(APP_INTEGRITY_ERROR);
    this.saveIntent({key:crypto.randomUUID(),offer_code:code,order_id:null});await this.resume();
  }
  async resume() {
    if (!this.intent) throw new Error(APP_INTEGRITY_ERROR);
    if (!this.offers) await this.loadOffers();
    if (!this.intent.order_id) {
      const order=await backend.createFundingOrder(this.accountID,this.intent.offer_code,this.intent.key,this.controller.signal);
      this.saveIntent({...this.intent,order_id:order.id});
    }
    const orderID=/** @type {string} */(this.intent.order_id);
    for(let attempt=0;attempt<POLL_LIMIT;attempt++) {
      const order=await backend.fetchFundingOrder(this.accountID,orderID,this.controller.signal);
      if(this.finish(order))return;
      if(order.state==='pending') {
        const checkout=await backend.fetchPaymentCheckout(this.accountID,orderID,this.controller.signal);
        await this.openCheckout(checkout);return;
      }
      this.message='Preparing checkout. Your funding order is retained.';this.render();await nextPoll(this.controller.signal);
    }
    this.message='Checkout is still being prepared. Resume this payment later.';
  }
  /** Keep unrelated requests and their original idempotency keys until resolved.
   * @param {import('../types.d.js').FundingOrder} order
   */
  selectRetainedOrder(order) {
    if(this.intent && this.intent.order_id!==order.id) {
      this.message='Finish the retained funding request before continuing this payment link.';
      return false;
    }
    if(!this.intent)this.saveIntent({key:crypto.randomUUID(),offer_code:order.offer_code,order_id:order.id});
    return true;
  }
  async continueSavedLink() {
    const candidate=this.continuation;
    if(!candidate)throw new Error(APP_INTEGRITY_ERROR);
    try {
      let order;
      try {order=await backend.fetchFundingOrder(this.accountID,candidate.order_id,this.controller.signal);}
      catch(error) {
        // An absent/denied owned order is definitive; checkout absence can still be pending delivery.
        if(error instanceof backend.BackendClientError && [400,403,404,422].includes(error.status))this.discardContinuation(candidate);
        throw error;
      }
      let checkout;
      try {checkout=await backend.fetchPaymentCheckout(this.accountID,candidate.order_id,this.controller.signal);}
      catch(error) {
        if(error instanceof backend.BackendClientError && [400,403,422].includes(error.status))this.discardContinuation(candidate);
        throw error;
      }
      if(checkout.transaction_id!==candidate.transaction_id)throw new Error(APP_INTEGRITY_ERROR);
      if(this.finish(order)) {this.discardContinuation(candidate);return;}
      if(order.state==='created') {this.message='Checkout is being prepared. Retry this payment link shortly.';return;}
      if(order.state!=='pending')throw new Error(APP_INTEGRITY_ERROR);
      // Resolve configuration before changing any saved intent.
      await this.loadOffers();
      if(!this.selectRetainedOrder(order))return;
      await this.openCheckout(checkout);
      this.discardContinuation(candidate);
    } catch(error) {
      if(!this.controller.signal.aborted && error instanceof Error && error.message===APP_INTEGRITY_ERROR)this.discardContinuation(candidate);
      throw error;
    }
  }
  /** @param {import('../core/paymentContinuation.js').PaymentContinuation} candidate */
  discardContinuation(candidate) {
    clearPaymentContinuation(candidate);
    if(this.continuation===candidate)this.continuation=null;
  }
  /** @param {import('../types.d.js').PaymentCheckout} checkout */
  async openCheckout(checkout) {
    this.closeCheckout();
    this.closeCheckout=await openPaddleCheckout(/** @type {import('../types.d.js').FundingOffers} */(this.offers),checkout,name=>{
      if(name==='checkout.completed') {if(this.busy)this.statusRequested=true;else void this.run(()=>this.checkStatus());}
      else {this.message='Checkout closed. You can resume this payment or check its status.';this.render();}
    },this.controller.signal);
    this.message='Checkout is open. Funds require server verification.';
  }
  /** @param {import('../types.d.js').FundingOrder} order */
  finish(order) {
    if(['created','pending'].includes(order.state))return false;
    if(this.intent?.order_id===order.id) {this.closeCheckout();this.saveIntent(null);}
    this.message=order.state==='failed'?'Payment failed. You can choose a funding amount again.':`Payment verified: ${order.state.replaceAll('_',' ')}. View its receipt in funding history.`;
    window.dispatchEvent(new CustomEvent(FUNDING_CHANGED_EVENT,{detail:{accountID:this.accountID}}));return true;
  }
  async checkStatus() {
    if(!this.intent?.order_id)throw new Error(APP_INTEGRITY_ERROR);
    for(let attempt=0;attempt<POLL_LIMIT;attempt++) {
      const order=await backend.fetchFundingOrder(this.accountID,this.intent.order_id,this.controller.signal);
      if(this.finish(order))return;
      this.message='Waiting for verified payment. Checkout alone does not add funds.';this.render();await nextPoll(this.controller.signal);
    }
    this.message='Payment is still pending. Check payment status again later.';
  }
  render() {
    const disabled=this.busy||this.storageFailure?'disabled':'';
    this.innerHTML=`<section class="prepaid-balance cw-details" aria-label="Add funds" aria-busy="${this.busy}">
      <h3>Add funds</h3><p>Minimum funding: $5.00. Spend your balance down to $0.00.</p>
      <p>Paddle handles payment. Taxes and the final payment total appear in checkout.</p>
      <p><a href="https://www.paddle.com/legal/buyer-terms" target="_blank" rel="noopener noreferrer">Paddle Buyer Terms</a> · <a href="https://www.paddle.com/legal/refund-policy" target="_blank" rel="noopener noreferrer">Paddle Refund Policy</a> · <a href="https://paddle.net" target="_blank" rel="noopener noreferrer">Payment and refund support</a></p>
      ${this.failure?`<p role="alert">${escapeHTML(this.failure)}</p>`:''}
      ${this.message?`<p role="status">${escapeHTML(this.message)}</p>`:''}
      ${this.continuation?`<button data-funding-action="link" ${disabled}>Retry payment link</button>`:''}
      ${this.intent?`<button data-funding-action="resume" ${disabled}>${this.intent.order_id?'Resume checkout':'Retry funding request'}</button>
        ${this.intent.order_id?`<button data-funding-action="status" ${disabled}>Check payment status</button>`:''}`:
        this.offers?this.offers.offers.map(offer=>`<button data-funding-action="create" data-offer="${escapeHTML(offer.code)}" ${disabled}>Add ${dollars(offer.funding_cents)}</button>`).join(' '):
        `<button data-funding-action="offers" ${disabled}>Choose funding amount</button>`}
    </section>`;
  }
}
customElements.define('funding-checkout',FundingCheckout);
