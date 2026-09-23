// @ts-check
import {expect,test} from '@playwright/test';
import path from 'node:path';
import {assets,directory} from './sharedUIAssets.mjs';
import {localManagementProfile,startLocalManagementStack} from './localManagementStack.mjs';
import {startPaddleProtocolFixture} from './paddleProtocolFixture.mjs';

let stack,processor;
test.beforeAll(async()=>{
  processor=await startPaddleProtocolFixture();
  stack=await startLocalManagementStack('frontend',{payments:processor.config});
});
test.afterAll(async()=>{try{if(stack)await stack.stop();}finally{if(processor)await processor.stop();}});

test('funding history and receipts follow verified payments and refund holds through the normal runtime',async({page,context})=>{
  const headers=await preparePaymentPage(page,context);
  await page.goto(stack.frontendOrigin);
  await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
  await page.getByRole('button',{name:'Create billing account',exact:true}).click();
  const billing=await (await context.request.get(`${stack.llmProxyOrigin}/api/management/billing-accounts`,{headers})).json();
  const root=`${stack.llmProxyOrigin}/api/management/billing-accounts/${billing.billing_accounts[0].id}`;
  const payments=page.getByRole('region',{name:'Funding history',exact:true});
  await expect(payments).toBeVisible();
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
  await expect(payments).toContainText('No payments yet.');
  const created=await context.request.post(root+'/funding-orders',{headers:{...headers,'Idempotency-Key':'browser-funding-history'},data:{offer_code:'five'}});
  expect(created.status()).toBe(201);
  const order=await created.json();
  await expect.poll(async()=>processor.transactions.length).toBe(1);
  await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('pending');
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
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
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
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
  await funds.getByRole('button',{name:'Refresh balance',exact:true}).click();
  await funds.getByRole('button',{name:'View financial history',exact:true}).click();
  await expect(funds).toContainText('payment-adjustment:');
  await expect(funds.getByRole('alert')).toHaveCount(0);
  const approved=processor.refund(order.id,'approved',2);
  await processor.event(stack.llmProxyOrigin,'adjustment.updated',approved);
  await expect.poll(async()=>(await (await context.request.get(root+`/funding-orders/${order.id}`,{headers})).json()).state).toBe('partially_refunded');
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
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
  await payments.getByRole('button',{name:'Open Paddle invoices',exact:true}).click();
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
  await payments.getByRole('button',{name:'Open Paddle invoices',exact:true}).click();
  const failedPopup=await failedPopupPromise;
  await expect.poll(()=>failedPopup.isClosed()).toBeTruthy();
  await expect(payments.getByRole('alert')).toHaveText('Payments are unavailable. Try again later.');
  await expect(payments).not.toContainText('private processor diagnostic');
  await page.unroute(root+'/payment-portal-sessions');

  for(let sequence=0;sequence<51;sequence++) {
    const response=await context.request.post(root+'/funding-orders',{headers:{...headers,'Idempotency-Key':`browser-page-${sequence}`},data:{offer_code:'five'}});
    expect(response.status()).toBe(201);
  }
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
  await expect(payments.locator('[data-payment-order]')).toHaveCount(50);
  await payments.getByRole('button',{name:'Load more payments',exact:true}).click();
  await expect(payments.locator('[data-payment-order]')).toHaveCount(52);
  await expect(payments.getByRole('button',{name:'Load more payments',exact:true})).toHaveCount(0);
  const ids=await payments.locator('[data-payment-order]').evaluateAll(elements=>elements.map(element=>element.getAttribute('data-payment-order')));
  expect(new Set(ids).size).toBe(52);
  await page.route(root+'/funding-orders?*',route=>route.fulfill({status:503,json:{error:{code:'funding_unavailable',detail:'private database diagnostic'}}}));
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
  await expect(payments.getByRole('alert')).toHaveText('Payments are unavailable. Try again later.');
  await expect(payments.locator('[data-payment-order]')).toHaveCount(0);
  await expect(payments).not.toContainText('No payments yet.');
  await expect(payments).not.toContainText('private database diagnostic');
  await page.unroute(root+'/funding-orders?*');
  await payments.getByRole('button',{name:'Refresh payments',exact:true}).click();
  await expect(payments.locator('[data-payment-order]')).toHaveCount(50);
  expect(processor.failures).toEqual([]);
});

async function preparePaymentPage(page,context,email=localManagementProfile.operatorEmail) {
  for(const name of Object.keys(assets)) {
    await page.route(`https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/${name}*`,route=>route.fulfill({path:path.join(directory,name),contentType:name.endsWith('.css')?'text/css':'application/javascript'}));
  }
  for(const [pattern,file] of [
    ['**/alpinejs@3.17.1/dist/module.esm.js','node_modules/alpinejs/dist/module.esm.js'],
    ['**/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js','node_modules/js-yaml/dist/browser/js-yaml.umd.min.js'],
    ['https://accounts.google.com/gsi/client','tests/blackbox/googleIdentityFixture.js'],
  ])await page.route(pattern,route=>route.fulfill({path:file,contentType:'application/javascript'}));
  await page.route('https://loopaware.mprlab.com/**',route=>route.fulfill({body:'',contentType:'application/javascript'}));
  const headers={Origin:stack.frontendOrigin,'X-Requested-With':'XMLHttpRequest','X-TAuth-Tenant':localManagementProfile.tenantID};
  await page.route(`${stack.tAuthOrigin}/auth/google`,async route=>{
    const login=await context.request.post(`${stack.tAuthOrigin}/auth/password/login`,{headers,data:{email:email,password:localManagementProfile.operatorPassword}});
    expect(login.ok()).toBeTruthy();await route.fulfill({status:login.status(),contentType:'application/json',body:await login.body()});
  });
  return headers;
}

test('browser checkout retries one order and waits for verified funding after Paddle completion',async({page,context})=>{
  await preparePaymentPage(page,context,localManagementProfile.secondOperatorEmail);
  let blockSDK=false;
  await page.route('https://cdn.paddle.com/paddle/v2/paddle.js',route=>blockSDK?route.abort('failed'):route.fulfill({path:'tests/blackbox/paddleBrowserFixture.js',contentType:'application/javascript'}));
  await page.goto(stack.frontendOrigin);
  await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
  await page.getByRole('button',{name:'Create billing account',exact:true}).click();
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
  const orders=await (await context.request.get(root+'/funding-orders',{headers})).json();
  expect(orders.orders).toHaveLength(1);
  const order=orders.orders[0];
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
  const afterResume=await (await context.request.get(root+'/funding-orders',{headers})).json();
  expect(afterResume.orders).toHaveLength(2);
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
  await history.getByRole('button',{name:'Refresh payments',exact:true}).click();
  await history.getByRole('button',{name:'Continue payment',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
  await page.getByRole('button',{name:'Close controlled checkout',exact:true}).click();
  const retainedOrders=(await (await context.request.get(root+'/funding-orders',{headers})).json()).orders;
  expect(retainedOrders).toHaveLength(2);
  const unfinished=retainedOrders.find(item=>item.state==='pending');
  await processor.event(stack.llmProxyOrigin,'transaction.canceled',processor.cancel(unfinished.id));
  await funding.getByRole('button',{name:'Check payment status',exact:true}).click();
  await expect(funding).toContainText('Payment failed.');
  await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$5.00');
  await expect(history.locator(`[data-payment-order="${unfinished.id}"]`)).toContainText('Failed');
  await expect(history.getByRole('button',{name:'Continue payment',exact:true})).toHaveCount(0);
  expect(processor.failures).toEqual([]);
});
