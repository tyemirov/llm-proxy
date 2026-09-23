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
    const login=await context.request.post(`${stack.tAuthOrigin}/auth/password/login`,{headers,data:{email:localManagementProfile.operatorEmail,password:localManagementProfile.operatorPassword}});
    expect(login.ok()).toBeTruthy();await route.fulfill({status:login.status(),contentType:'application/json',body:await login.body()});
  });
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
