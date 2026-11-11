// @ts-check
const SDK_URL='https://cdn.paddle.com/paddle/v2/paddle.js';
const COMPLETED='checkout.completed',CLOSED='checkout.closed';
/** @typedef {{Environment:{set:(environment:string)=>void},Initialize:(options:{token:string,eventCallback:(event:unknown)=>void})=>void,Checkout:{open:(options:{transactionId:string,settings:{displayMode:string,allowLogout:boolean,showAddDiscounts:boolean}})=>void,close:()=>void}}} PaddleClient */
/** @type {Promise<PaddleClient>|null} */ let loaded=null;
/** @type {{token:string,environment:string}|null} */ let initialized=null;
/** @type {{transactionID:string,callback:(name:string)=>void}|null} */ let active=null;

/** @param {unknown} input */
function checkoutEvent(input) {
  const event=/** @type {{name?:unknown,data?:{transaction_id?:unknown}}|null} */(input);
  if (!event || (event.name!==COMPLETED && event.name!==CLOSED) || !active || event.data?.transaction_id!==active.transactionID) return;
  const callback=active.callback;
  if (event.name===CLOSED) active=null;
  callback(event.name);
}
/** @returns {Promise<PaddleClient>} */
function loadPaddle() {
  if (loaded) return loaded;
  loaded=new Promise((resolve,reject)=>{
    const script=document.createElement('script');script.src=SDK_URL;script.async=true;
    const timeout=window.setTimeout(()=>fail(),15000);
    const cleanup=()=>{window.clearTimeout(timeout);script.onload=null;script.onerror=null;};
    const fail=()=>{cleanup();script.remove();loaded=null;reject(new Error('Paddle checkout could not load. Try again.'));};
    script.onerror=fail;
    script.onload=()=>{
      cleanup();
      const client=/** @type {Window & {Paddle?:PaddleClient}} */(window).Paddle;
      if (!client || typeof client.Initialize!=='function' || typeof client.Environment?.set!=='function' || typeof client.Checkout?.open!=='function' || typeof client.Checkout?.close!=='function') {fail();return;}
      resolve(client);
    };
    document.head.append(script);
  });
  return loaded;
}
/** @param {import('../types.d.js').FundingOffers} config @param {import('../types.d.js').PaymentCheckout} checkout @param {(name:string)=>void} callback @param {AbortSignal} signal @returns {Promise<()=>void>} */
export async function openPaddleCheckout(config,checkout,callback,signal) {
  if (config.environment!==checkout.environment) throw new Error('The checkout environment changed. Reload this page.');
  const paddle=await loadPaddle();signal.throwIfAborted();
  if (initialized && (initialized.token!==config.client_token || initialized.environment!==config.environment)) throw new Error('The checkout configuration changed. Reload this page.');
  if (!initialized) {
    paddle.Environment.set(config.environment);
    paddle.Initialize({token:config.client_token,eventCallback:checkoutEvent});
    initialized={token:config.client_token,environment:config.environment};
  }
  if (active) throw new Error('Close the current checkout before opening another payment.');
  const subscription={transactionID:checkout.transaction_id,callback};active=subscription;
  const close=()=>{signal.removeEventListener('abort',close);if (active===subscription) {active=null;paddle.Checkout.close();}};
  signal.addEventListener('abort',close,{once:true});
  try {paddle.Checkout.open({transactionId:checkout.transaction_id,settings:{displayMode:'overlay',allowLogout:false,showAddDiscounts:false}});}
  catch(error) {close();throw error;}
  return close;
}
