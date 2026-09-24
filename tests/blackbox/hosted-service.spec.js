// @ts-check
import {expect,test} from '@playwright/test';
import {prepareManagementPage} from './authenticatedManagementPage.mjs';
import {localManagementProfile,startLocalManagementStack} from './localManagementStack.mjs';
import {startPaddleProtocolFixture} from './paddleProtocolFixture.mjs';

let stack,processor;
test.beforeAll(async()=>{
  processor=await startPaddleProtocolFixture();
  stack=await startLocalManagementStack('frontend',{
    adminEmails:[localManagementProfile.secondOperatorEmail],payments:processor.config,
    hosted:{offerings:[{provider:'openai',model:'gpt-4.1',operation:'text',maximum_attempts:1,conditions:{billing_mode:'standard',service_tier:'standard'}}]},
    configureCatalog(catalog) {
      const offering=catalog.providers.find(provider=>provider.id==='openai').offerings.find(offering=>offering.model==='gpt-4.1');
      offering.output_token_limit=10;
      offering.limits=[{id:'input_tokens',value:10,unit:'tokens'},{id:'output_tokens',value:10,unit:'tokens'}];
      offering.prices=[{operation:'text',available:true,source:'https://example.com/controlled-rates',last_verified:'2026-09-23',rates:['input_tokens','output_tokens','prompt_cache_read_tokens'].map(component=>({component,currency:'USD',rate:'2000',unit:'USD/1M_tokens',conditions:{billing_mode:'standard',service_tier:'standard',effective_from:'2026-09-01T00:00:00Z',...(component==='prompt_cache_read_tokens'?{cache_class:'read'}:{})}}))}];
    },
    responses:{id:'funded-browser-result',status:'completed',output_text:'Funded browser answer',usage:{input_tokens:2,output_tokens:3,total_tokens:5,input_tokens_details:{cached_tokens:0},output_tokens_details:{reasoning_tokens:0}}},
  });
});
test.afterAll(async()=>{try{if(stack)await stack.stop();}finally{if(processor)await processor.stop();}});

test('a customer funds hosted access and reads the real settled charge',async({page,context,browser})=>{
  const headers=await prepareManagementPage(page,context,stack);
  await page.route('https://cdn.paddle.com/paddle/v2/paddle.js',route=>route.fulfill({path:'tests/blackbox/paddleBrowserFixture.js',contentType:'application/javascript'}));
  await page.goto(stack.frontendOrigin);
  await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
  await page.getByRole('button',{name:'Create billing account',exact:true}).click();
  const base=stack.llmProxyOrigin+'/api/management';
  const account=await (await context.request.get(base+'/account',{headers})).json();
  const billing=await (await context.request.get(base+'/billing-accounts',{headers})).json();
  const accountID=billing.billing_accounts[0].id,tenantID=account.tenants[0].id,root=base+'/billing-accounts/'+accountID;
  const operator=await browser.newContext();
  try {
    const login=await operator.request.post(stack.tAuthOrigin+'/auth/password/login',{headers,data:{email:localManagementProfile.secondOperatorEmail,password:localManagementProfile.operatorPassword}});
    expect(login.ok()).toBeTruthy();
    const provision=async(path,data)=>{
      const response=await operator.request.post(base+path,{headers:{...headers,'Idempotency-Key':crypto.randomUUID()},data});
      expect(response.status(),await response.text()).toBe(201);return response.json();
    };
    const platform=await provision('/platform-connections',{name:'Controlled hosted account',provider:'openai',fields:{api_key:'sk-private-hosted-service'}});
    const catalog=await (await context.request.get(stack.llmProxyOrigin+'/api/public/capabilities')).json();
    const grant=await provision('/hosted-access-grants',{billing_account_id:accountID,tenant_id:tenantID,platform_connection_id:platform.id,catalog_revision:catalog.revision,offerings:[{model:'gpt-4.1',operations:['text']}],reason:'Controlled complete service acceptance'});
    const hosted=page.getByRole('region',{name:'Hosted access',exact:true});
    await hosted.getByRole('button',{name:'Refresh hosted access',exact:true}).click();
    const card=hosted.locator(`[data-hosted-grant="${grant.id}"]`);
    await card.getByRole('button',{name:'Use hosted access',exact:true}).click();
    await expect(card).toContainText('Assigned');
    await expect(card.getByLabel('Hosted text model',{exact:true})).toBeVisible();
    await card.getByLabel('Hosted text model', {exact:true}).selectOption('gpt-4.1');
    await card.getByRole('button',{name:'Save hosted text default',exact:true}).click();
    await expect(card).toContainText('Saved text default: gpt-4.1');
    await page.reload();
    await expect(card.getByLabel('Hosted text model',{exact:true})).toHaveValue('gpt-4.1');
    await expect(card).toContainText('Saved text default: gpt-4.1');
    await page.getByRole('button',{name:'API access',exact:true}).click();
    await page.getByRole('dialog').getByRole('button',{name:'Create API key',exact:true}).click();
    const key=await page.getByRole('dialog').getByLabel('Tenant API key').inputValue();
    await expect(page.getByRole('dialog')).toContainText('Idempotency-Key');
    await page.getByRole('button',{name:'Close dialog',exact:true}).click();
    const callsBefore=stack.providerRequests.length;
    const request=async(idempotencyKey='funded-browser-request')=>context.request.post(stack.llmProxyOrigin+'/v1/responses',{headers:{Authorization:'Bearer '+key,'Idempotency-Key':idempotencyKey},data:{model:'openai/gpt-4.1',input:'Hello',max_output_tokens:10}});
    const denied=await request();
    expect(denied.status(),await denied.text()).toBe(402);
    expect(stack.providerRequests).toHaveLength(callsBefore);
    const funding=page.getByRole('region',{name:'Add funds',exact:true});
    await funding.getByRole('button',{name:'Choose funding amount',exact:true}).click();
    await funding.getByRole('button',{name:'Add $5.00',exact:true}).click();
    await expect(page.getByRole('dialog',{name:'Controlled Paddle checkout'})).toBeVisible();
    const orders=await (await context.request.get(root+'/funding-orders',{headers})).json();
    expect(orders.orders).toHaveLength(1);
    await page.getByRole('button',{name:'Complete controlled checkout',exact:true}).click();
    await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$0.00');
    await processor.event(stack.llmProxyOrigin,'transaction.completed',processor.complete(orders.orders[0].id));
    await expect(funding).toContainText('Payment verified');
    await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$5.00');
    for(let count=0;count<2;count++) {
      const result=await request();expect(result.status(),`${await result.text()}\n${stack.serviceOutput()}`).toBe(200);
      expect(await result.json()).toMatchObject({output:[{type:'message',content:[{type:'output_text',text:'Funded browser answer'}]}]});
    }
    expect(stack.providerRequests).toHaveLength(callsBefore+1);
    expect(stack.providerRequests.at(-1).authorization).toBe('Bearer sk-private-hosted-service');
    await expect.poll(async()=>(await (await context.request.get(root+'/balance',{headers})).json()).posted_cents).toBe('499');
    const balance=await (await context.request.get(root+'/balance',{headers})).json();
    expect(balance).toMatchObject({posted_cents:'499',available_cents:'499',reserved_cents:'0',unsettled_fraction:{numerator:'3',denominator:'1000'}});
    await page.getByRole('region',{name:'Prepaid balance',exact:true}).getByRole('button',{name:'Refresh balance',exact:true}).click();
    await expect(page.locator('[data-funds-value="available_cents"]')).toHaveText('$4.99');
    const journal=page.getByRole('region',{name:'Usage journal',exact:true});
    await journal.getByRole('button',{name:'Load usage journal',exact:true}).click();
    await expect(journal.locator('[data-journal-request]')).toHaveCount(1);
    await journal.getByRole('button',{name:'View request',exact:true}).click();
    const summary=journal.getByRole('region',{name:'Request charges',exact:true});
    await expect(summary).toContainText('Provider cost: $0.01');
    await expect(summary).toContainText('Customer charge: $0.013');
    await expect(summary).toContainText('Net charge: $0.013');
    await journal.getByRole('button',{name:'Load charges',exact:true}).click();
    const charges=journal.getByRole('region',{name:'Itemized charges',exact:true});
    await expect(charges.locator('[data-charge-id]')).toHaveCount(1);
    await expect(charges).toContainText('Customer charge: $0.013');
    const history=page.getByRole('region',{name:'Funding history',exact:true});
    await history.getByRole('button',{name:'Refresh payments',exact:true}).click();
    await history.getByRole('button',{name:'View receipt',exact:true}).click();
    await expect(history).toContainText('Account credit: $5.00');
    const chargeID=await charges.locator('[data-charge-id]').getAttribute('data-charge-id');
    const foreign=await operator.request.get(root+'/charges/'+chargeID,{headers});
    expect(foreign.status()).toBe(404);
    const suspension=await operator.request.patch(base+'/hosted-access-grants/'+grant.id,{headers,data:{revision:1,state:'suspended',reason:'Stop new hosted work'}});
    expect(suspension.status(),await suspension.text()).toBe(200);
    const suspendedProfile=await context.request.get(base+'/tenants/'+tenantID,{headers});
    expect(suspendedProfile.status(),await suspendedProfile.text()).toBe(200);
    expect(await suspendedProfile.json()).toMatchObject({tenant:{defaults:{provider:'openai',model:'gpt-4.1'}}});
    const suspendedRequest=await request('suspended-browser-request');
    expect(suspendedRequest.status(),await suspendedRequest.text()).toBe(403);
    expect(stack.providerRequests).toHaveLength(callsBefore+1);
    for(const width of [1440,390,320]) {
      await page.setViewportSize({width,height:1000});
      await expect(summary).toBeVisible();await expect(charges).toBeVisible();
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
    }
    expect(processor.failures).toEqual([]);
  } finally {await operator.close();}
});
