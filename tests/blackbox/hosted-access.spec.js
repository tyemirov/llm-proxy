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
  const operator=await browser.newContext();
  try {
    const login=await operator.request.post(`${stack.tAuthOrigin}/auth/password/login`,{headers,data:{email:localManagementProfile.secondOperatorEmail,password:localManagementProfile.operatorPassword}});
    expect(login.ok()).toBeTruthy();
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
    let failObservations=false, invalidMeasurement=false;
    await page.route(root+'**',async route=>{
      const url=new URL(route.request().url()),cursor=url.searchParams.get('cursor');
      let body;
      if(url.pathname.endsWith('/requests'))body={requests:[cursor?serviceRequest:retained],next_cursor:cursor?'':requestID};
      else if(url.pathname.endsWith('/'+requestID))body=retained;
      else if(url.pathname.endsWith('/attempts'))body={attempts:[{id:cursor?secondAttempt:attemptID,number:cursor?2:1,state:'observed',dispatched_at:timestamp,observed_at:timestamp,created_at:timestamp,updated_at:timestamp}],next_cursor:cursor?'':attemptID};
      else if(url.pathname.endsWith('/observations')) {
        if(failObservations){await route.fulfill({status:500,json:{error:{code:'usage_journal_unavailable',detail:'private storage diagnostic'}}});return;}
        body={observations:[{id:cursor?secondObservation:observationID,attempt_id:attemptID,quantities:cursor?[{dimension:'output_tokens',unit:'token',value:'1.25'}]:[{dimension:'input_tokens',unit:'token',value:invalidMeasurement?9007199254740992:'9007199254740993'},{dimension:'output_tokens',unit:'token',value:'0'},{dimension:'cache_read_tokens',unit:'token',unknown_reason:'not_reported',included_in:'input_tokens'}],completeness:cursor?'complete':'unknown',outcome:'continue',observed_at:timestamp,created_at:timestamp}],next_cursor:cursor?'':observationID};
      } else if(url.pathname.endsWith('/reconciliation-cases'))body={cases:[cursor?{id:secondCase,reason:'execution_result_unknown',state:'resolved',resolved_at:timestamp,created_at:timestamp}:{id:caseID,reason:'usage_unknown',state:'open',created_at:timestamp}],next_cursor:cursor?'':caseID};
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
    for(const kind of ['attempts','observations','cases'])await details.getByRole('button',{name:'Load more '+kind,exact:true}).click();
    await expect(details).toContainText('Attempt 2');
    await expect(details).toContainText('1.25 token');
    await expect(details).toContainText('Resolved');
    await expect(details).toContainText('Execution result unknown');
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
    await journalView.locator(`[data-journal-request="${requestID}"]`).click();
    await expect(details).toContainText('9007199254740993');
  } finally { await operator.close(); }
});
