// @ts-check
import {expect, test} from '@playwright/test';
import {readFile} from 'node:fs/promises';
import path from 'node:path';
import {assets, directory} from './sharedUIAssets.mjs';
import {localManagementProfile, startLocalManagementStack} from './localManagementStack.mjs';

let stack;
test.beforeAll(async()=>{stack=await startLocalManagementStack('frontend', {adminEmails:[localManagementProfile.secondOperatorEmail]});});
test.afterAll(async()=>{if(stack)await stack.stop();});

test('hosted onboarding creates tenant access without provider credentials at desktop and phone widths', async({page, context, browser})=>{
  for(const name of Object.keys(assets)) {
    const body=await readFile(path.join(directory,name));
    await page.route(`https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/${name}*`,route=>route.fulfill({body,contentType:name.endsWith('.css')?'text/css':'application/javascript'}));
  }
  for(const [pattern,file] of [
    ['**/alpinejs@3.17.1/dist/module.esm.js','node_modules/alpinejs/dist/module.esm.js'],
    ['**/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js','node_modules/js-yaml/dist/browser/js-yaml.umd.min.js'],
    ['https://accounts.google.com/gsi/client','tests/blackbox/googleIdentityFixture.js'],
  ]) {
    const body=await readFile(file);
    await page.route(pattern,route=>route.fulfill({body,contentType:'application/javascript'}));
  }
  await page.route('https://loopaware.mprlab.com/**',route=>route.fulfill({body:'',contentType:'application/javascript'}));
  const headers={Origin:stack.frontendOrigin,'X-Requested-With':'XMLHttpRequest','X-TAuth-Tenant':localManagementProfile.tenantID};
  await page.route(`${stack.tAuthOrigin}/auth/google`,async route=>{
    const login=await context.request.post(`${stack.tAuthOrigin}/auth/password/login`,{headers,data:{email:localManagementProfile.operatorEmail,password:localManagementProfile.operatorPassword}});
    expect(login.ok()).toBeTruthy();
    await route.fulfill({status:login.status(),contentType:'application/json',body:await login.body()});
  });
  await page.goto(stack.frontendOrigin);
  await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
  const dashboard=page.locator('connection-dashboard');
  const hosted=dashboard.getByRole('region',{name:'Hosted access',exact:true});
  await expect(hosted).toBeVisible();
  await expect(hosted.getByRole('button',{name:'Create billing account'})).toBeVisible();
  await hosted.getByRole('button',{name:'Create billing account'}).click();
  await expect(hosted).toContainText('USD billing account');
  const funds=dashboard.getByRole('region',{name:'Prepaid balance',exact:true});
  await expect(funds).toBeVisible();
  await expect(funds.locator('[data-funds-value="available_cents"]')).toHaveText('$0.00');
  await expect(funds).toContainText('No funds available for hosted requests.');
  await expect(funds).toContainText('Minimum funding: $5.00');
  const tenantLimit=funds.getByRole('region',{name:'Tenant spending limit',exact:true});
  await expect(tenantLimit).toContainText('No tenant limit');
  await tenantLimit.getByLabel('Limit in USD').fill('12.34');
  await tenantLimit.getByRole('button',{name:'Save limit',exact:true}).click();
  await expect(tenantLimit).toContainText('Remaining allowance: $12.34');
  await page.reload();
  await expect(tenantLimit.getByLabel('Limit in USD')).toHaveValue('12.34');
  await tenantLimit.getByLabel('Limit in USD').fill('0.001');
  await tenantLimit.getByRole('button',{name:'Save limit',exact:true}).click();
  await expect(tenantLimit.getByRole('alert')).toContainText('Use a nonnegative USD amount with at most two decimal places.');
  await tenantLimit.getByRole('button',{name:'Remove limit',exact:true}).click();
  await expect(tenantLimit).toContainText('No tenant limit');
  await funds.getByRole('button',{name:'View financial history',exact:true}).click();
  await expect(funds).toContainText('No reservations yet.');
  await expect(funds).toContainText('No ledger entries yet.');
  const journalView=dashboard.getByRole('region',{name:'Usage journal',exact:true});
  await expect(journalView).toBeVisible();
  await journalView.getByRole('button',{name:'Load usage journal',exact:true}).click();
  await expect(journalView).toContainText('No hosted requests yet.');
  await expect(page.getByLabel('API key',{exact:false})).toHaveCount(0);
  const base=`${stack.llmProxyOrigin}/api/management`;
  const account=await (await context.request.get(`${base}/account`,{headers})).json();
  const billing=await (await context.request.get(`${base}/billing-accounts`,{headers})).json();
  const journal=await page.evaluate(async billingID=>{
    const client=await import('/assets/llm-proxy/js/core/backendClient.js');
    return client.fetchJournalRequests(billingID);
  },billing.billing_accounts[0].id);
  expect(journal).toEqual({requests:[],next_cursor:''});
  const tenant=account.tenants[0].id;
  const limitURL=`${base}/billing-accounts/${billing.billing_accounts[0].id}/tenant-limits/${tenant}`;
  const priorLimit=await (await context.request.get(limitURL,{headers})).json();
  const otherTab=await context.request.put(limitURL,{headers,data:{limit_cents:'456',revision:priorLimit.revision}});
  expect(otherTab.status()).toBe(200);
  await tenantLimit.getByLabel('Limit in USD').fill('7.89');
  await tenantLimit.getByRole('button',{name:'Save limit',exact:true}).click();
  await expect(tenantLimit.getByRole('alert')).toContainText('The limit changed in another session.');
  await expect(tenantLimit.getByLabel('Limit in USD')).toHaveValue('4.56');
  const foreignOrigin=await context.request.put(limitURL,{headers:{...headers,Origin:'https://foreign.example'},data:{limit_cents:null,revision:priorLimit.revision+1}});
  expect(foreignOrigin.status()).toBe(403);
  await tenantLimit.getByRole('button',{name:'Remove limit',exact:true}).click();
  await expect(tenantLimit).toContainText('No tenant limit');
  const operator=await browser.newContext();
  try {
    const login=await operator.request.post(`${stack.tAuthOrigin}/auth/password/login`,{headers,data:{email:localManagementProfile.secondOperatorEmail,password:localManagementProfile.operatorPassword}});
    expect(login.ok()).toBeTruthy();
    const foreignAccount=await operator.request.get(limitURL,{headers});
    expect(foreignAccount.status()).toBe(404);
    const provision=async(path, data)=>{
      const response=await operator.request.post(base+path,{headers:{...headers,'Idempotency-Key':crypto.randomUUID()},data});
      expect(response.status(),await response.text()).toBe(201);
      return response.json();
    };
    const platform=await provision('/platform-connections',{name:'Browser hosted',provider:'openai',fields:{api_key:'sk-private-platform-browser'}});
    const catalog=await (await context.request.get(`${stack.llmProxyOrigin}/api/public/capabilities`)).json();
    const grant=await provision('/hosted-access-grants',{billing_account_id:billing.billing_accounts[0].id,tenant_id:tenant,platform_connection_id:platform.id,catalog_revision:catalog.revision,offerings:[{model:'gpt-4.1',operations:['text']}],reason:'Browser acceptance'});
    await hosted.getByRole('button',{name:'Refresh hosted access'}).click();
    const card=hosted.locator(`[data-hosted-grant="${grant.id}"]`);
    await expect(card).toContainText('gpt-4.1');
    await card.getByRole('button',{name:'Use hosted access'}).click();
    await expect(card).toContainText('Assigned');
    await expect(hosted).toContainText('Hosted execution is not available yet.');
    await dashboard.getByRole('button',{name:'API access',exact:true}).click();
    await page.getByRole('dialog').getByRole('button',{name:'Create API key',exact:true}).click();
    await expect(page.getByRole('dialog').getByLabel('Tenant API key')).toHaveValue(/^llmp_/);
    await page.getByRole('button',{name:'Close dialog',exact:true}).click();
    for (const width of [1440,390,320]) {
      await page.setViewportSize({width,height:1000});
      await expect(card.getByRole('button',{name:'Detach hosted access'})).toBeVisible();
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
    }
    const suspension=await operator.request.patch(`${base}/hosted-access-grants/${grant.id}`,{headers,data:{revision:1,state:'suspended',reason:'Browser suspension'}});
    expect(suspension.status()).toBe(200);
    await page.reload();
    await expect(card).toContainText('Suspended');
    await expect(card).toContainText('Assigned');
    await card.getByRole('button',{name:'Detach hosted access'}).click();
    await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();
    await expect(card).not.toContainText('Assigned');
    await expect(card.getByRole('button',{name:'Use hosted access'})).toHaveCount(0);
    await expect(dashboard).not.toContainText(platform.id);
    await expect(dashboard).not.toContainText('sk-private-platform-browser');
    const servicePlatform=await provision('/platform-connections',{name:'Browser hosted services',provider:'elevenlabs',fields:{api_key:'local-eleven-key'}});
    const serviceGrant=await provision('/hosted-access-grants',{billing_account_id:billing.billing_accounts[0].id,tenant_id:tenant,platform_connection_id:servicePlatform.id,catalog_revision:catalog.revision,offerings:[{operations:['audio_alignment','pronunciation_dictionary_creation']}],reason:'Service browser acceptance'});
    await hosted.getByRole('button',{name:'Refresh hosted access'}).click();
    const serviceCard=hosted.locator(`[data-hosted-grant="${serviceGrant.id}"]`);
    await expect(serviceCard).toContainText('Provider services');
    await expect(serviceCard).toContainText('audio_alignment');
    await serviceCard.getByRole('button',{name:'Use hosted access'}).click();
    for(const width of [1440,390,320]) {
      await page.setViewportSize({width,height:1000});
      await expect(serviceCard).toContainText('Assigned');
      await expect(serviceCard.getByRole('button',{name:'Detach hosted access'})).toBeVisible();
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
    }
    await expect(dashboard).not.toContainText(servicePlatform.id);
    await expect(dashboard).not.toContainText('local-eleven-key');
    const requestID='request-'+ '1'.repeat(32), secondID='request-'+ '2'.repeat(32);
    const attemptID='attempt-'+ '1'.repeat(32), secondAttempt='attempt-'+ '2'.repeat(32);
    const observationID='observation-'+ '1'.repeat(32), secondObservation='observation-'+ '2'.repeat(32);
    const caseID='case-'+ '1'.repeat(32), secondCase='case-'+ '2'.repeat(32);
    const timestamp='2026-09-22T12:00:00Z';
    const retained={id:requestID,tenant_id:tenant,grant_id:grant.id,grant_revision:1,provider:'openai',model:'gpt-4.1',operation:'text',catalog_revision:catalog.revision,execution_kind:'text_request',execution_id:'retained-text',state:'completed',usage_state:'unknown',created_at:timestamp,updated_at:timestamp};
    const serviceRequest={...retained,id:secondID,grant_id:serviceGrant.id,provider:'elevenlabs',operation:'audio_alignment',execution_kind:'media_operation',execution_id:'retained-media',state:'uncertain'};
    delete serviceRequest.model;
    const root=`${base}/billing-accounts/${billing.billing_accounts[0].id}/requests`;
    let failObservations=false, invalidMeasurement=false, financialReason='usage_unknown', summaryState='unresolved';
    let summaryCharge={numerator:'13',denominator:'1000'};
    let summaryCredits={numerator:'1',denominator:'1000'}, summaryNet={numerator:'3',denominator:'250'}, summaryProvider={numerator:'1',denominator:'100'};
    await page.route(root+'**',async route=>{
      const url=new URL(route.request().url()),cursor=url.searchParams.get('cursor');
      let body;
      if(url.pathname.endsWith('/requests'))body={requests:[cursor?serviceRequest:retained],next_cursor:cursor?'':requestID};
      else if(url.pathname.endsWith('/'+requestID))body=retained;
      else if(url.pathname.endsWith('/charge-summary'))body={request_id:requestID,state:summaryState,attempt_count:1,charge_count:1,provider_cost:summaryState==='pending'?null:summaryProvider,customer_charge:summaryState==='rated'?summaryCharge:null,customer_credits:summaryState==='rated'?summaryCredits:null,net_customer_charge:summaryState==='rated'?summaryNet:null};
      else if(url.pathname.endsWith('/attempts'))body={attempts:[{id:cursor?secondAttempt:attemptID,number:cursor?2:1,state:'observed',dispatched_at:timestamp,observed_at:timestamp,created_at:timestamp,updated_at:timestamp}],next_cursor:cursor?'':attemptID};
      else if(url.pathname.endsWith('/observations')) {
        if(failObservations){await route.fulfill({status:500,json:{error:{code:'usage_journal_unavailable',detail:'private storage diagnostic'}}});return;}
        body={observations:[{id:cursor?secondObservation:observationID,attempt_id:attemptID,quantities:cursor?[{dimension:'output_tokens',unit:'token',value:'1.25'}]:[{dimension:'input_tokens',unit:'token',value:invalidMeasurement?9007199254740992:'9007199254740993'},{dimension:'output_tokens',unit:'token',value:'0'},{dimension:'cache_read_tokens',unit:'token',unknown_reason:'not_reported',included_in:'input_tokens'}],completeness:cursor?'complete':'unknown',outcome:'continue',observed_at:timestamp,created_at:timestamp}],next_cursor:cursor?'':observationID};
      } else if(url.pathname.endsWith('/reconciliation-cases'))body={cases:[cursor?{id:secondCase,reason:'execution_result_unknown',state:'resolved',resolved_at:timestamp,created_at:timestamp}:{id:caseID,reason:financialReason,state:'open',created_at:timestamp}],next_cursor:cursor?'':caseID};
      else throw new Error('Unexpected journal request: '+url.pathname);
      await route.fulfill({json:body});
    });
    await journalView.getByRole('button',{name:'Load usage journal',exact:true}).click();
    await journalView.getByRole('button',{name:'Load more requests',exact:true}).click();
    await expect(journalView.locator('[data-journal-request]')).toHaveCount(2);
    await expect(journalView).toContainText('audio alignment');
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();
    const details=journalView.getByRole('region',{name:'Request evidence',exact:true});
    await expect(details).toContainText('9007199254740993');
    await expect(details).toContainText('Unknown (not reported)');
    await expect(details).toContainText('Included in input tokens');
    await expect(details).toContainText('0 token');
    await expect(details.getByRole('region',{name:'Request charges',exact:true})).toContainText('Charge unresolved');
    for(const kind of ['attempts','observations','cases'])await details.getByRole('button',{name:'Load more '+kind,exact:true}).click();
    await expect(details).toContainText('Attempt 2');
    await expect(details).toContainText('1.25 token');
    await expect(details).toContainText('Resolved');
    await expect(details).toContainText('Execution result unknown');
    for(const reason of ['usage_unresolved','policy_unresolved','limit_unresolved','platform_exposure']) {
      financialReason=reason;
      await journalView.locator(`[data-journal-request="${requestID}"]`).click();
      const label=reason.charAt(0).toUpperCase()+reason.slice(1).replaceAll('_',' ');
      await expect(details).toContainText(label);
    }
    for(const width of [1440,390,320]){
      await page.setViewportSize({width,height:1000});
      await expect(details).toBeVisible();
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
    }
    failObservations=true;
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();
    await expect(journalView.getByRole('alert')).toBeVisible();
    await expect(journalView).not.toContainText('private storage diagnostic');
    await expect(journalView.getByRole('region',{name:'Request evidence',exact:true})).toHaveCount(0);
    failObservations=false;
    invalidMeasurement=true;
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();
    await expect(journalView.getByRole('alert')).toBeVisible();
    await expect(journalView.getByRole('region',{name:'Request evidence',exact:true})).toHaveCount(0);
    invalidMeasurement=false;
    summaryState='rated';
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();
    await expect(details).toContainText('9007199254740993');
    const totals=details.getByRole('region',{name:'Request charges',exact:true});
    await expect(totals).toContainText('Customer charge: $0.013');
    await expect(totals).toContainText('Credits: $0.001');
    await expect(totals).toContainText('Net charge: $0.012');
    summaryState='pending';
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();
    await expect(totals).toContainText('Charge pending');
    await expect(totals).not.toContainText('$0.00');
    summaryState='rated';
    for(const [numerator,denominator,display] of [
      ['9007199254740993','100','$90071992547409.93'],
      ['1','3','≈$0.333333'],
      ['1','10000000','Less than $0.000001'],
    ]) {
      summaryCharge={numerator,denominator};
      summaryCredits={numerator:'0',denominator:'1'};summaryNet=summaryCharge;
      let costNumerator=BigInt(numerator)*10n,costDenominator=BigInt(denominator)*13n;
      let left=costNumerator,right=costDenominator;
      while(right){const next=left%right;left=right;right=next;}
      summaryProvider={numerator:String(costNumerator/left),denominator:String(costDenominator/left)};
      await journalView.locator(`[data-journal-request="${requestID}"]`).click();
      await expect(totals).toContainText('Customer charge: '+display);
      await expect(totals.locator(`span[title="Exact: ${numerator}/${denominator} USD"]`).first()).toBeVisible();
    }
    summaryCharge={numerator:'13',denominator:'1000'};
    summaryCredits={numerator:'1',denominator:'1000'};summaryNet={numerator:'3',denominator:'250'};summaryProvider={numerator:'1',denominator:'100'};
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();

    const chargeID='charge-'+ '1'.repeat(32), secondCharge='charge-'+ '2'.repeat(32);
    let invalidCharge=false, failedCharges=false;
    await page.route(`${base}/billing-accounts/${billing.billing_accounts[0].id}/charges?*`,async route=>{
      if(failedCharges)return route.fulfill({status:500,json:{error:{code:'usage_journal_unavailable',detail:'private charge diagnostic'}}});
      const more=new URL(route.request().url()).searchParams.has('cursor');
      const gross={numerator:invalidCharge?13:'13',denominator:'1000'}, providerCost={numerator:'1',denominator:'100'};
      const rating={state:'resolved',lines:[{dimension:'input_tokens',component:'input_tokens',quantity:'10',quantity_unit:'token',provider_rate:'1000',customer_rate:{numerator:'1300',denominator:'1'},rate_unit:'USD/1M_tokens',conditions:{},provider_cost:providerCost,customer_charge:gross}],provider_cost:providerCost,customer_charge:gross,minimum_adjustment:{numerator:'0',denominator:'1'},unresolved_dimensions:[]};
      const charge={id:more?secondCharge:chargeID,request_id:more?secondID:requestID,attempt_id:more?secondAttempt:attemptID,observation_id:more?secondObservation:observationID,price_snapshot_id:'price-'+ '1'.repeat(32),state:more?'usage_unresolved':'rated',rating:more?{state:'unresolved',lines:[],provider_cost:null,customer_charge:null,minimum_adjustment:null,unresolved_dimensions:['input_tokens']}:rating,customer_charge:more?null:gross,net_customer_charge:more?null:{numerator:'3',denominator:'250'},customer_adjustments:more?[]:[{id:'credit-'+ '1'.repeat(32),credit:{numerator:'1',denominator:'1000'},reason:'Approved correction',created_at:timestamp}],created_at:timestamp};
      return route.fulfill({json:{charges:[charge],next_cursor:more?'':chargeID}});
    });
    await journalView.getByRole('button',{name:'Load charges',exact:true}).click();
    const charges=journalView.getByRole('region',{name:'Itemized charges',exact:true});
    await expect(charges).toContainText('10 token');
    await expect(charges).toContainText('Provider cost: $0.01');
    await expect(charges).toContainText('Customer charge: $0.013');
    await expect(charges).toContainText('Approved correction');
    await expect(charges).toContainText('Net charge: $0.012');
    await charges.getByRole('button',{name:'Load more charges',exact:true}).click();
    await expect(charges.locator('[data-charge-id]')).toHaveCount(2);
    await expect(charges.locator(`[data-charge-id="${secondCharge}"]`)).toContainText('Charge unresolved');
    await expect(charges.locator(`[data-charge-id="${secondCharge}"]`)).not.toContainText('$0.00');
    for(const width of [1440,390,320]){
      await page.setViewportSize({width,height:1000});
      await expect(charges).toBeVisible();
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
    }
    for(const invalid of ['numeric amount','failed read']) {
      invalidCharge=invalid==='numeric amount';failedCharges=invalid==='failed read';
      await journalView.getByRole('button',{name:'Refresh charges',exact:true}).click();
      await expect(journalView.getByRole('alert')).toBeVisible();
      await expect(charges.locator('[data-charge-id]')).toHaveCount(0);
      await expect(journalView).not.toContainText('private charge diagnostic');
    }
    invalidCharge=false;failedCharges=false;
    await journalView.getByRole('button',{name:'Refresh charges',exact:true}).click();
    await expect(charges).toContainText('Net charge: $0.012');

    const fundsRoot=`${base}/billing-accounts/${billing.billing_accounts[0].id}`;
    let fundsFailure=false, invalidFunds=false;
    await page.route(fundsRoot+'/balance',async route=>{
      if(fundsFailure)return route.fulfill({status:500,json:{error:{code:'billing_account_store_failed',detail:'private financial diagnostic'}}});
      return route.fulfill({json:{currency:'USD',state:'active',posted_cents:'9007199254740993',available_cents:invalidFunds?9007199254740493:'9007199254740493',reserved_cents:'500',spent_cents:'123',pending_cents:'200',unsettled_fraction:{numerator:'91',denominator:'25000'}}});
    });
    await page.route(fundsRoot+'/reservations?*',route=>{
      const more=new URL(route.request().url()).searchParams.has('cursor');
      return route.fulfill({json:{reservations:[{id:more?secondID:requestID,currency:'USD',maximum_cents:more?'200':'300',state:more?'reconciliation_required':'held',revision:1,created_at:timestamp,updated_at:timestamp}],next_cursor:more?'':requestID}});
    });
    await page.route(fundsRoot+'/ledger-entries?*',route=>{
      const more=new URL(route.request().url()).searchParams.has('cursor');
      const entryID=more?'22222222-2222-4222-8222-222222222222':'11111111-1111-4111-8111-111111111111';
      return route.fulfill({json:{entries:[{id:entryID,currency:'USD',type:more?'spend':'grant',amount_cents:more?'-123':'9007199254740993',reservation_id:null,refund_of_entry_id:null,created_at:timestamp}],next_cursor:more?'':entryID}});
    });
    await funds.getByRole('button',{name:'Refresh balance',exact:true}).click();
    await expect(funds.locator('[data-funds-value="available_cents"]')).toHaveText('$90071992547404.93');
    await expect(funds.locator('[data-funds-value="reserved_cents"]')).toHaveText('$5.00');
    await expect(funds.locator('[data-funds-value="pending_cents"]')).toHaveText('$2.00');
    await expect(funds.locator('[data-funds-value="spent_cents"]')).toHaveText('$1.23');
    await expect(funds).toContainText('91/25000 USD');
    await funds.getByRole('button',{name:'View financial history',exact:true}).click();
    await funds.getByRole('button',{name:'Load more reservations',exact:true}).click();
    await funds.getByRole('button',{name:'Load more ledger entries',exact:true}).click();
    await expect(funds.locator('[data-funds-reservation]')).toHaveCount(2);
    await expect(funds.locator('[data-funds-entry]')).toHaveCount(2);
    await expect(funds).toContainText('-$1.23');
    await expect(funds).toContainText('Reconciliation required');
    for(const width of [1440,390,320]) {
      await page.setViewportSize({width,height:1000});
      await expect(funds).toBeVisible();
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
    }
    for(const failure of ['read','contract']) {
      fundsFailure=failure==='read';invalidFunds=failure==='contract';
      await funds.getByRole('button',{name:'Refresh balance',exact:true}).click();
      await expect(funds.getByRole('alert')).toBeVisible();
      await expect(funds.locator('[data-funds-value]')).toHaveCount(0);
      await expect(funds).not.toContainText('private financial diagnostic');
    }
    fundsFailure=false;invalidFunds=false;
    await funds.getByRole('button',{name:'Refresh balance',exact:true}).click();
    await expect(funds.locator('[data-funds-value="available_cents"]')).toHaveText('$90071992547404.93');
  } finally { await operator.close(); }
});
