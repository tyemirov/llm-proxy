// @ts-check
import {expect,test} from '@playwright/test';
import {prepareManagementPage} from './authenticatedManagementPage.mjs';
import {localManagementProfile,startLocalManagementStack} from './localManagementStack.mjs';
import {startPaddleProtocolFixture} from './paddleProtocolFixture.mjs';

let stack,processor;
test.beforeAll(async()=>{
  processor=await startPaddleProtocolFixture();
  stack=await startLocalManagementStack('frontend',{payments:processor.config});
});
test.afterAll(async()=>{try{if(stack)await stack.stop();}finally{if(processor)await processor.stop();}});

test('funding history and receipts follow verified payments and refund holds through the normal runtime',async({page,context})=>{
  await page.route(`${stack.llmProxyOrigin}/api/management/billing-accounts`,async route=>{
    if(route.request().method()==='POST')await new Promise(resolve=>setTimeout(resolve,100));
    await route.continue();
  });
  await page.clock.install();
  const headers=await prepareManagementPage(page,context,stack);
  await page.goto(stack.frontendOrigin);
  await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
  await page.getByRole('button',{name:'Set up prepaid balance',exact:true}).click();
  const payments=page.getByRole('region',{name:'Funding history',exact:true});
  await expect(payments).toBeVisible();
  const billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
  const root=`${stack.llmProxyOrigin}/api/management/billing-accounts/${billing.billing_accounts[0].id}`;
  await page.clock.fastForward(30_001);
  await expect(payments).toContainText('No payments yet.');
  const created=await context.request.post(root+'/funding-orders',{headers:{...headers,'Idempotency-Key':'browser-funding-history'},data:{offer_code:'five'}});
  expect(created.status()).toBe(201);
  const order=await created.json();
  await expect.poll(async()=>processor.transactions.length).toBe(1);
  await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('pending');
  await page.clock.fastForward(30_001);
  const row=payments.locator(`[data-payment-order="${order.id}"]`);
  await expect(row).toContainText('Pending');
  await expect(row).toContainText('$5.00');
  await expect(row.getByRole('button',{name:'View receipt'})).toHaveCount(0);
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$0.00');
  const completed=processor.complete(order.id);
  // Browser return information cannot post funds.
  await page.goto(stack.frontendOrigin+'/?payment=success');
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$0.00');
  await processor.event(stack.llmProxyOrigin,'transaction.completed',completed);
  await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('paid');
  await page.clock.fastForward(30_001);
  await expect(row).toContainText('Paid');
  await row.getByRole('button',{name:'View receipt',exact:true}).click();
  const receipt=payments.getByRole('region',{name:'Payment receipt',exact:true});
  await expect(receipt).toContainText('INV-BROWSER-1');
  await expect(receipt).toContainText('Original payment: $5.50');
  await expect(receipt).toContainText('Original tax: $0.50');
  await expect(receipt).toContainText('Account credit: $5.00');
  await expect(receipt).not.toContainText('earnings');
  const pending=processor.refund(order.id,'pending_approval',1);
  await processor.event(stack.llmProxyOrigin,'adjustment.created',pending);
  await expect.poll(async()=>(await (await context.request.get(root+'/balance',{headers})).json()).available_cents).toBe('300');
  await row.getByRole('button',{name:'View receipt',exact:true}).click();
  await expect(receipt).toContainText('Pending credit reversal: $2.00');
  const funds=page.getByRole('region',{name:'Prepaid balance',exact:true});
  let releaseBalance, observeBalance;
  const balanceHeld=new Promise(resolve=>{releaseBalance=resolve;});
  const balanceStarted=new Promise(resolve=>{observeBalance=resolve;});
  const holdBalance=async route=>{observeBalance();await balanceHeld;await route.continue();};
  await page.route(root+'/balance',holdBalance);
  const historyButton=funds.getByRole('button',{name:'View financial history',exact:true});
  try {
    await page.clock.fastForward(30_001);
    await balanceStarted;
    await expect(historyButton).toBeDisabled();
    await expect(funds).toHaveAttribute('aria-busy','true');
    await funds.getByLabel('Limit in USD').fill('19.25');
  } finally {releaseBalance();}
  await expect(historyButton).toBeEnabled();
  await expect(funds).toHaveAttribute('aria-busy','false');
  await expect(funds.getByLabel('Limit in USD')).toHaveValue('19.25');
  await expect(funds.getByLabel('Limit in USD')).toBeFocused();
  await page.unroute(root+'/balance',holdBalance);
  await historyButton.click();
  await expect(funds).toContainText('payment-adjustment:');
  await expect(funds.getByRole('alert')).toHaveCount(0);
  const approved=processor.refund(order.id,'approved',2);
  await processor.event(stack.llmProxyOrigin,'adjustment.updated',approved);
  await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('partially_refunded');
  await page.clock.fastForward(30_001);
  await expect(row).toContainText('Partially refunded');
  await row.getByRole('button',{name:'View receipt',exact:true}).click();
  await expect(receipt).toContainText('Current payment: $3.30');
  await expect(receipt).toContainText('Credit reversed: $2.00');
  await expect(receipt).toContainText('Pending credit reversal: $0.00');
  for(const width of [1440,390,320]) {
    await page.setViewportSize({width,height:1000});
    await expect(receipt).toBeVisible();
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
  }
  await context.route('https://customer-portal.paddle.com/browser-fixture?*',route=>route.fulfill({body:'<h1>Controlled Paddle invoice portal</h1>',contentType:'text/html'}));
  const popupPromise=page.waitForEvent('popup');
  await payments.getByRole('button',{name:'Paddle invoices',exact:true}).click();
  const popup=await popupPromise;
  await expect(popup.getByRole('heading')).toHaveText('Controlled Paddle invoice portal');
  await popup.close();
  expect(processor.portalCalls).toBe(1);

  const receiptURL=root+`/funding-orders/${order.id}/receipt`;
  const verifiedReceipt=await (await context.request.get(receiptURL,{headers})).json();
  for(const invalid of [
    {...verifiedReceipt,gross_cents:550},
    {...verifiedReceipt,funding_order_id:'funding-'+'0'.repeat(32)},
    {...verifiedReceipt,currency:'EUR'},
  ]) {
    await page.route(receiptURL,route=>route.fulfill({json:invalid}));
    await row.getByRole('button',{name:'View receipt',exact:true}).click();
    await expect(payments.getByRole('alert')).toBeVisible();
    await expect(receipt).toHaveCount(0);
    await page.unroute(receiptURL);
  }
  await row.getByRole('button',{name:'View receipt',exact:true}).click();
  await expect(receipt).toContainText('INV-BROWSER-1');
  await page.route(root+'/payment-portal-sessions',route=>route.fulfill({status:503,json:{error:{code:'funding_unavailable',detail:'private processor diagnostic'}}}));
  const failedPopupPromise=page.waitForEvent('popup');
  await payments.getByRole('button',{name:'Paddle invoices',exact:true}).click();
  const failedPopup=await failedPopupPromise;
  await expect.poll(()=>failedPopup.isClosed()).toBeTruthy();
  await expect(payments.getByRole('alert')).toHaveText('Payments are unavailable. Try again later.');
  await expect(payments).not.toContainText('private processor diagnostic');
  await page.unroute(root+'/payment-portal-sessions');

  for(let sequence=0;sequence<51;sequence++) {
    const response=await context.request.post(root+'/funding-orders',{headers:{...headers,'Idempotency-Key':`browser-page-${sequence}`},data:{offer_code:'five'}});
    expect(response.status()).toBe(201);
  }
  await page.clock.fastForward(30_001);
  await expect(payments.locator('[data-payment-order]')).toHaveCount(50);
  await payments.getByRole('button',{name:'Load more payments',exact:true}).click();
  await expect(payments.locator('[data-payment-order]')).toHaveCount(52);
  await expect(payments.getByRole('button',{name:'Load more payments',exact:true})).toHaveCount(0);
  const ids=await payments.locator('[data-payment-order]').evaluateAll(elements=>elements.map(element=>element.getAttribute('data-payment-order')));
  expect(new Set(ids).size).toBe(52);
  await page.route(root+'/funding-orders?*',route=>route.fulfill({status:503,json:{error:{code:'funding_unavailable',detail:'private database diagnostic'}}}));
  await page.clock.fastForward(30_001);
  await expect(payments.getByRole('alert')).toHaveText('Payments are unavailable. Try again later.');
  await expect(payments.locator('[data-payment-order]')).toHaveCount(0);
  await expect(payments).not.toContainText('No payments yet.');
  await expect(payments).not.toContainText('private database diagnostic');
  await page.unroute(root+'/funding-orders?*');
  await page.clock.fastForward(30_001);
  await expect(payments.locator('[data-payment-order]')).toHaveCount(50);
  expect(processor.failures).toEqual([]);
});


test('browser checkout retries one order and waits for verified funding after Paddle completion',async({page,context})=>{
  await page.clock.install();
  await prepareManagementPage(page,context,stack,localManagementProfile.secondOperatorEmail);
  let blockSDK=false;
  await page.route('https://cdn.paddle.com/paddle/v2/paddle.js',route=>blockSDK?route.abort('failed'):route.fulfill({path:'tests/blackbox/paddleBrowserFixture.js',contentType:'application/javascript'}));
  await page.goto(stack.frontendOrigin);
  await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
  await page.getByRole('button',{name:'Set up prepaid balance',exact:true}).click();
  const funding=page.getByRole('region',{name:'Add funds',exact:true});
  await expect(funding).toBeVisible();
  await funding.getByRole('button',{name:'Choose funding amount',exact:true}).click();
  await expect(funding).toContainText('Minimum funding: $5.00');
  const headers={Origin:stack.frontendOrigin,'X-Requested-With':'XMLHttpRequest','X-TAuth-Tenant':localManagementProfile.tenantID};
  const billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
  const root=`${stack.llmProxyOrigin}/api/management/billing-accounts/${billing.billing_accounts[0].id}`;
  let creations=0;const keys=[];
  await page.route(root+'/funding-orders',async route=>{
    if(route.request().method()!=='POST')return route.continue();
    keys.push(route.request().headers()['idempotency-key']);
    expect(route.request().postDataJSON()).toEqual({offer_code:'five'});
    const response=await route.fetch();
    if(++creations===1)return route.abort('failed');
    return route.fulfill({response});
  });
  await funding.getByRole('button',{name:'Add $5.00',exact:true}).click();
  await expect(funding.getByRole('alert')).toBeVisible();
  await page.reload();
  await funding.getByRole('button',{name:'Retry funding request',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  expect(keys).toHaveLength(2);expect(keys[0]).toBe(keys[1]);
  const orders=await allFundingOrders(context,root,headers);
  expect(orders).toHaveLength(1);
  const order=orders[0];
  expect(await page.evaluate(()=>window.paddleFixture.opened[0])).toEqual({transactionId:processor.transactions.find(item=>item.custom_data.funding_order_id===order.id).id,settings:{displayMode:'overlay',allowLogout:false,showAddDiscounts:false}});
  expect(await page.evaluate(()=>window.paddleFixture.token)).toBe('test_browserfixture');
  expect(await page.evaluate(()=>window.paddleFixture.environment)).toBe('sandbox');
  await expect(funding.getByRole('link',{name:'Paddle Refund Policy'})).toHaveAttribute('href','https://www.paddle.com/legal/refund-policy');
  await page.evaluate(()=>window.paddleFixture.emit({name:'checkout.completed',data:{transaction_id:'txn_'+'0'.repeat(26)}}));
  await expect(funding).not.toContainText('Waiting for verified payment');
  await page.getByRole('button',{name:'Complete controlled checkout',exact:true}).click();
  await page.evaluate(()=>window.paddleFixture.emit({name:'checkout.completed',data:{transaction_id:window.paddleFixture.opened[0].transactionId}}));
  await expect(funding).toContainText('Waiting for verified payment');
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$0.00');
  await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(order.id));
  await expect(funding).toContainText('Payment verified');
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$5.00');
  await expect(page.getByRole('region',{name:'Funding history',exact:true})).toContainText('Paid');
  await expect(funding.getByRole('alert')).toHaveCount(0);
  await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(order.id));
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$5.00');
  await page.reload();
  await funding.getByRole('button',{name:'Choose funding amount',exact:true}).click();
  await funding.getByRole('button',{name:'Add $5.00',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
  await expect(funding).toContainText('Checkout closed');
  await funding.getByRole('button',{name:'Resume checkout',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  expect(await page.evaluate(()=>window.paddleFixture.initializations)).toBe(1);
  const afterResume=await allFundingOrders(context,root,headers);
  expect(afterResume).toHaveLength(2);
  const retainedOrderIDs=afterResume.map(order=>order.id).sort();
  expect(retainedOrderIDs).toContain(order.id);
  await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
  for(const width of [1440,390,320]) {
    await page.setViewportSize({width,height:1000});
    await expect(funding.getByRole('button',{name:'Resume checkout',exact:true})).toBeVisible();
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
  }
  blockSDK=true;
  await page.reload();
  await funding.getByRole('button',{name:'Resume checkout',exact:true}).click();
  await expect(funding.getByRole('alert')).toBeVisible();
  blockSDK=false;
  await funding.getByRole('button',{name:'Resume checkout',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  await expect(funding.getByRole('alert')).toHaveCount(0);
  await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
  await page.evaluate(accountID=>sessionStorage.removeItem('llm-proxy:funding-intent:'+accountID),billing.billing_accounts[0].id);
  await page.reload();
  const history=page.getByRole('region',{name:'Funding history',exact:true});
  await page.clock.fastForward(30_001);
  await history.getByRole('button',{name:'Continue payment',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
  const retainedOrders=await allFundingOrders(context,root,headers);
  expect(retainedOrders.map(order=>order.id).sort()).toEqual(retainedOrderIDs);
  const unfinished=retainedOrders.find(item=>item.state==='pending');
  await processor.event(stack.llmProxyOrigin,'transaction.canceled',processor.cancel(unfinished.id));
  await funding.getByRole('button',{name:'Check payment status',exact:true}).click();
  await expect(funding).toContainText('Payment failed.');
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$5.00');
  await expect(history.locator(`[data-payment-order="${unfinished.id}"]`)).toContainText('Failed');
  await expect(history.getByRole('button',{name:'Continue payment',exact:true})).toHaveCount(0);
  expect(processor.failures).toEqual([]);
});


test('checkout link resumes the owned transaction after sign-in without creating another order',async({page,context})=>{
 const headers=await prepareManagementPage(page,context,stack);
 await page.route('https://cdn.paddle.com/paddle/v2/paddle.js',route=>route.fulfill({path:'tests/blackbox/paddleBrowserFixture.js',contentType:'application/javascript'}));
 await page.goto(stack.frontendOrigin);
 await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
 await expect(page).toHaveURL(/\/app\//);
 await expect.poll(async()=>(await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).status()).toBe(200);
 // Use the normal API to retain an order before the customer follows its checkout link.
 let billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
 if(!billing.billing_accounts.length) {
  await page.getByRole('button',{name:'Set up prepaid balance',exact:true}).click();
  billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
 }
 const root=`${stack.llmProxyOrigin}/api/management/billing-accounts/${billing.billing_accounts[0].id}`;
 const created=await context.request.post(root+'/funding-orders',{headers:{...headers,'Idempotency-Key':'checkout-link-resume'},data:{offer_code:'five'}});
 expect(created.status()).toBe(201);
 const order=await created.json();
 await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('pending');
 const transaction=processor.transactions.find(item=>item.custom_data.funding_order_id===order.id);
 const orderIDs=(await allFundingOrders(context,root,headers)).map(order=>order.id).sort();
 await context.clearCookies();
 await page.goto(`${stack.frontendOrigin}/?funding_order_id=${order.id}&_ptxn=${transaction.id}`);
 await expect(page).not.toHaveURL(/_ptxn=|funding_order_id=/);
 await expect(page.getByRole('button',{name:'Sign in with Google',exact:true})).toBeVisible();
 await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(await page.evaluate(()=>window.paddleFixture.opened.map(item=>item.transactionId))).toEqual([transaction.id]);
 expect((await allFundingOrders(context,root,headers)).map(order=>order.id).sort()).toEqual(orderIDs);
 await expect(page.locator('[data-funds-value="available_cents"]')).toBeVisible();
 await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();

 // An authenticated customer can follow the same transaction link directly.
 await page.goto(`${stack.frontendOrigin}/app/?funding_order_id=${order.id}&_ptxn=${transaction.id}&source=pilot`);
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(await page.evaluate(()=>window.paddleFixture.opened.map(item=>item.transactionId))).toEqual([transaction.id]);
 expect(await page.evaluate(()=>window.paddleFixture.initializedURL)).not.toMatch(/_ptxn=|funding_order_id=/);
 expect(new URL(page.url()).searchParams.get('source')).toBe('pilot');
 await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
 const balanceBefore=await (await context.request.get(root+'/balance',{headers})).json();
 for(const query of [
   `funding_order_id=${order.id}&_ptxn=txn_${'z'.repeat(26)}`,
   `funding_order_id=${order.id}&_ptxn=invalid`,
   `_ptxn=${transaction.id}`,
   `funding_order_id=${order.id}`,
   `funding_order_id=${order.id}&_ptxn=${transaction.id}&_ptxn=${transaction.id}`,
   `funding_order_id=funding-${'0'.repeat(32)}&_ptxn=${transaction.id}`,
 ]) {
   await page.goto(`${stack.frontendOrigin}/app/?${query}`);
   const funding=page.getByRole('region',{name:'Add funds',exact:true});
   await expect(funding).toBeVisible();
   await expect(funding).toHaveAttribute('aria-busy','false');
   await expect(page).not.toHaveURL(/_ptxn=|funding_order_id=/);
   expect(await page.evaluate(()=>window.paddleFixture?.opened || [])).toEqual([]);
   await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toHaveCount(0);
 }
 expect((await allFundingOrders(context,root,headers)).map(order=>order.id).sort()).toEqual(orderIDs);
 expect((await (await context.request.get(root+'/balance',{headers})).json()).available_cents).toBe(balanceBefore.available_cents);
 // Verified completed orders show their server status and never reopen checkout.
 await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(order.id));
 await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('paid');
 await page.goto(`${stack.frontendOrigin}/app/?funding_order_id=${order.id}&_ptxn=${transaction.id}`);
 await expect(page.getByRole('region',{name:'Add funds',exact:true})).toContainText('Payment verified: paid');
 expect(await page.evaluate(()=>window.paddleFixture?.opened || [])).toEqual([]);
 // A second authenticated account cannot resolve the original account's order.
 await context.clearCookies();
 await prepareManagementPage(page,context,stack,localManagementProfile.secondOperatorEmail);
 await page.goto(stack.frontendOrigin);
 await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
 await expect(page).toHaveURL(/\/app\//);
 await expect.poll(async()=>(await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).status()).toBe(200);
 const other=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
 if(!other.billing_accounts.length)await page.getByRole('button',{name:'Set up prepaid balance',exact:true}).click();
 await expect(page.getByRole('region',{name:'Add funds',exact:true})).toBeVisible();
 await page.goto(`${stack.frontendOrigin}/app/?funding_order_id=${order.id}&_ptxn=${transaction.id}`);
 const foreignFunding=page.getByRole('region',{name:'Add funds',exact:true});
 await expect(foreignFunding).toBeVisible();await expect(foreignFunding).toHaveAttribute('aria-busy','false');
 expect(await page.evaluate(()=>window.paddleFixture?.opened || [])).toEqual([]);
 await expect(page.locator(`[data-payment-order="${order.id}"]`)).toHaveCount(0);
 const foreignRoot=`${stack.llmProxyOrigin}/api/management/billing-accounts/${(await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json()).billing_accounts[0].id}`;
 expect((await context.request.get(foreignRoot+`/funding-orders/${order.id}`,{headers})).status()).toBe(404);
 expect((await context.request.get(foreignRoot+`/funding-orders/${order.id}/checkout`,{headers})).status()).toBe(404);
 expect(processor.failures).toEqual([]);
});

async function allFundingOrders(context,root,headers) {
 const orders=[],cursors=new Set();let cursor='';
 do {
  const url=new URL(root+'/funding-orders');url.searchParams.set('limit','50');
  if(cursor)url.searchParams.set('cursor',cursor);
  const response=await context.request.get(url.href,{headers});expect(response.status()).toBe(200);
  const page=await response.json();expect(Array.isArray(page.orders)).toBeTruthy();
  orders.push(...page.orders);
  cursor=page.next_cursor;expect(typeof cursor).toBe('string');
  if(cursor) {expect(cursors.has(cursor)).toBeFalsy();cursors.add(cursor);}
 } while(cursor);
 const ids=orders.map(order=>order.id);expect(new Set(ids).size).toBe(ids.length);
 return orders;
}

async function revealPaymentHistoryOrder(page,orderID) {
 const history=page.getByRole('region',{name:'Funding history',exact:true});
 const row=history.locator(`[data-payment-order="${orderID}"]`);
 await expect(page.locator('connection-dashboard')).toHaveAttribute('aria-busy','false');
 // These fixtures retain at least one order. Initial aria-busy=false precedes the first response.
 await expect(history.locator('[data-payment-order]').first()).toBeVisible();
 while(!await row.count()) {
  const more=history.getByRole('button',{name:'Load more payments',exact:true});
  if(!await more.count())break;
  await more.click();await expect(history).toHaveAttribute('aria-busy','false');
 }
 await expect(row).toBeVisible();return row;
}

async function checkoutReviewPage(page,context) {
 const headers=await prepareManagementPage(page,context,stack);
 await page.route('https://cdn.paddle.com/paddle/v2/paddle.js',route=>route.fulfill({path:'tests/blackbox/paddleBrowserFixture.js',contentType:'application/javascript'}));
 await page.goto(stack.frontendOrigin);
 await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
 await expect(page).toHaveURL(/\/app\//);
 await expect.poll(async()=>(await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).status()).toBe(200);
 let billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
 if(!billing.billing_accounts.length) {
  await page.getByRole('button',{name:'Set up prepaid balance',exact:true}).click();
  billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
 }
 const accountID=billing.billing_accounts[0].id;
 const root=`${stack.llmProxyOrigin}/api/management/billing-accounts/${accountID}`;
 const funding=page.getByRole('region',{name:'Add funds',exact:true});
 await expect(funding).toBeVisible();
 return {headers,root,accountID,funding};
}

async function reviewOrder(context,root,headers,key) {
 const response=await context.request.post(root+'/funding-orders',{headers:{...headers,'Idempotency-Key':key},data:{offer_code:'five'}});
 expect(response.status()).toBe(201);
 const order=await response.json();
 await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('pending');
 const transaction=processor.transactions.find(item=>item.custom_data.funding_order_id===order.id);
 return {order,transaction,url:`${stack.frontendOrigin}/app/?funding_order_id=${order.id}&_ptxn=${transaction.id}`};
}

test('checkout review blocks query auto-open when the early bootstrap cannot load',async({page,context})=>{
 const {headers,root,funding}=await checkoutReviewPage(page,context);
 const candidates=[await reviewOrder(context,root,headers,'bootstrap-load-failure'),await reviewOrder(context,root,headers,'bootstrap-history-before-target')].sort((left,right)=>left.order.id.localeCompare(right.order.id));
 const retained=candidates[1];
 let releaseHistory;const historyGate=new Promise(resolve=>{releaseHistory=resolve;});
 const delayedFirstPage=async route=>{
  const url=new URL(route.request().url());
  if(route.request().method()!=='GET' || url.searchParams.has('cursor'))return route.continue();
  url.searchParams.set('limit','1');
  const response=await route.fetch({url:url.href});await historyGate;return route.fulfill({response});
 };
 await page.route(root+'/funding-orders?*',delayedFirstPage);
 const foreign='txn_'+'z'.repeat(26);
 await page.route('**/paymentContinuationBootstrap.js?*',route=>route.abort('failed'));
 await page.goto(`${stack.frontendOrigin}/app/?funding_order_id=${retained.order.id}&_ptxn=${foreign}`);
 await expect(funding).toBeVisible();
 const revealing=revealPaymentHistoryOrder(page,retained.order.id);
 try {
  await expect(page.locator('connection-dashboard')).toHaveAttribute('aria-busy','false');
  await expect(page.getByRole('region',{name:'Funding history',exact:true}).locator('[data-payment-order]')).toHaveCount(0);
 } finally {releaseHistory();}
 const row=await revealing;
 await page.unroute(root+'/funding-orders?*',delayedFirstPage);
 await row.getByRole('button',{name:'Continue payment',exact:true}).click();
 await expect(funding).toHaveAttribute('aria-busy','false');
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toHaveCount(0);
 expect(await page.evaluate(()=>window.paddleFixture?.opened || [])).toEqual([]);
 await expect(funding.getByRole('alert')).toBeVisible();
 const balance=await (await context.request.get(root+'/balance',{headers})).json();
 expect(balance.pending_cents).toBe('0');
});

test('checkout review preserves an uncertain funding key across other paid and pending links',async({page,context})=>{
 const {headers,root,accountID,funding}=await checkoutReviewPage(page,context);
 let loseResponse=true;const keys=[];
 await page.route(root+'/funding-orders',async route=>{
  if(route.request().method()!=='POST')return route.continue();
  keys.push(route.request().headers()['idempotency-key']);
  const response=await route.fetch();
  if(loseResponse){loseResponse=false;return route.abort('failed');}
  return route.fulfill({response});
 });
 await funding.getByRole('button',{name:'Choose funding amount',exact:true}).click();
 await funding.getByRole('button',{name:'Add $5.00',exact:true}).click();
 await expect(funding.getByRole('alert')).toBeVisible();
 const original=await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID);
 expect(original.order_id).toBeNull();
 const candidates=[await reviewOrder(context,root,headers,'other-paid-link'),await reviewOrder(context,root,headers,'other-pending-link')].sort((left,right)=>left.order.id.localeCompare(right.order.id));
 const [pending,paid]=candidates;
 await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(paid.order.id));
 await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${paid.order.id}`,{headers})).json()).state).toBe('paid');
 // Retain real pagination and order ownership while forcing the paid order beyond the first history page.
 const historyPage=async route=>{
  if(route.request().method()!=='GET')return route.continue();
  const url=new URL(route.request().url());
  if(url.searchParams.has('cursor'))return route.continue();
  url.searchParams.set('limit','1');
  const response=await route.fetch({url:url.href});return route.fulfill({response});
 };
 await page.route(root+'/funding-orders?*',historyPage);
 const orderIDs=(await allFundingOrders(context,root,headers)).map(order=>order.id).sort();
 await page.goto(paid.url);
 await expect(funding).toContainText('Payment verified: paid');
 expect(await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID)).toEqual(original);
 await expect(page.locator(`[data-payment-order="${paid.order.id}"]`)).toHaveCount(0);
 await expect(page.getByRole('region',{name:'Funding history',exact:true}).getByRole('button',{name:'Load more payments',exact:true})).toBeVisible();
 await page.unroute(root+'/funding-orders?*',historyPage);
 await page.goto(pending.url);
 await expect(funding).toHaveAttribute('aria-busy','false');
 expect(await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID)).toEqual(original);
 await expect(funding.getByRole('button',{name:'Retry payment link',exact:true})).toBeVisible();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toHaveCount(0);
 await funding.getByRole('button',{name:'Retry funding request',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(keys).toEqual([original.key,original.key]);
 const originalOrder=await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID);
 await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
 await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(originalOrder.order_id));
 await funding.getByRole('button',{name:'Check payment status',exact:true}).click();
 await expect(funding).toContainText('Payment verified');
 await funding.getByRole('button',{name:'Retry payment link',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(await page.evaluate(()=>window.paddleFixture.opened.at(-1).transactionId)).toBe(pending.transaction.id);
 const linked=await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID);
 await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
 await page.goto(pending.url);
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID)).toEqual(linked);
 expect((await allFundingOrders(context,root,headers)).map(order=>order.id).sort()).toEqual(orderIDs);
});

test('checkout review retains the payment link and offers retry after temporary API failures',async({page,context})=>{
 const {headers,root,accountID,funding}=await checkoutReviewPage(page,context);
 const retained=await reviewOrder(context,root,headers,'temporary-link-api-failure');
 const orderIDs=(await allFundingOrders(context,root,headers)).map(order=>order.id).sort();
 for(const endpoint of [`/funding-orders/${retained.order.id}`,`/funding-orders/${retained.order.id}/checkout`,'/funding-offers']) {
  let unavailable=true;
  const routeHandler=route=>unavailable?route.fulfill({status:503,json:{error:{code:'funding_unavailable',detail:'controlled failure'}}}):route.continue();
  await page.route(root+endpoint,routeHandler);
  await page.goto(retained.url);
  await expect(funding.getByRole('alert')).toBeVisible();
  expect(await page.evaluate(()=>JSON.parse(sessionStorage.getItem('llm-proxy:payment-continuation')))).toEqual({order_id:retained.order.id,transaction_id:retained.transaction.id});
  await expect(funding.getByRole('button',{name:'Retry payment link',exact:true})).toBeVisible();
  unavailable=false;
  await funding.getByRole('button',{name:'Retry payment link',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  expect(await page.evaluate(()=>sessionStorage.getItem('llm-proxy:payment-continuation'))).toBeNull();
  expect(await page.evaluate(()=>window.paddleFixture.opened.at(-1).transactionId)).toBe(retained.transaction.id);
  await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
  await page.unroute(root+endpoint,routeHandler);
  await page.evaluate(id=>sessionStorage.removeItem('llm-proxy:funding-intent:'+id),accountID);
 }
 expect((await allFundingOrders(context,root,headers)).map(order=>order.id).sort()).toEqual(orderIDs);
});

test('checkout review terminal link leaves an unrelated active checkout and its intent intact',async({page,context})=>{
 const {headers,root,accountID,funding}=await checkoutReviewPage(page,context);
 await funding.getByRole('button',{name:'Choose funding amount',exact:true}).click();
 await funding.getByRole('button',{name:'Add $5.00',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 const original=await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID);
 await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
 const other=await reviewOrder(context,root,headers,'active-checkout-terminal-link');
 await page.goto(other.url);
 await expect(funding.getByRole('button',{name:'Retry payment link',exact:true})).toBeVisible();
 await expect(funding).toHaveAttribute('aria-busy','false');
 await funding.getByRole('button',{name:'Resume checkout',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 const active=await page.evaluate(()=>window.paddleFixture.opened.at(-1).transactionId);
 await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(other.order.id));
 await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${other.order.id}`,{headers})).json()).state).toBe('paid');
 // Exercise the public retry control while the independently owned SDK overlay is active.
 await funding.getByRole('button',{name:'Retry payment link',exact:true}).dispatchEvent('click');
 await expect(funding).toHaveAttribute('aria-busy','false');
 await expect(funding).toContainText('Payment verified');
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(await page.evaluate(()=>window.paddleFixture.opened.at(-1).transactionId)).toBe(active);
 expect(await page.evaluate(id=>JSON.parse(sessionStorage.getItem('llm-proxy:funding-intent:'+id)),accountID)).toEqual(original);
 await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
});

test('checkout review retains a preparing order when delivery commits between order and checkout reads',async({page,context})=>{
 const {headers,root,funding}=await checkoutReviewPage(page,context);
 const retained=await reviewOrder(context,root,headers,'checkout-delivery-read-race');
 const orderIDs=(await allFundingOrders(context,root,headers)).map(order=>order.id).sort();
 let staleSnapshot=true;
 await page.route(root+`/funding-orders/${retained.order.id}`,async route=>{
  const response=await route.fetch();
  if(!staleSnapshot)return route.fulfill({response});
  return route.fulfill({response,json:{...await response.json(),state:'created'}});
 });
 // The owned checkout endpoint already has the retained transaction while the order read is stale.
 await page.goto(retained.url);
 await expect(funding).toHaveAttribute('aria-busy','false');
 expect(await page.evaluate(()=>JSON.parse(sessionStorage.getItem('llm-proxy:payment-continuation')))).toEqual({order_id:retained.order.id,transaction_id:retained.transaction.id});
 await expect(funding.getByRole('button',{name:'Retry payment link',exact:true})).toBeVisible();
 expect(await page.evaluate(()=>window.paddleFixture?.opened || [])).toEqual([]);
 staleSnapshot=false;
 await funding.getByRole('button',{name:'Retry payment link',exact:true}).click();
 await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
 expect(await page.evaluate(()=>window.paddleFixture.opened.at(-1).transactionId)).toBe(retained.transaction.id);
 expect((await allFundingOrders(context,root,headers)).map(order=>order.id).sort()).toEqual(orderIDs);
});
