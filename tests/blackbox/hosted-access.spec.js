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
  await expect(page.getByLabel('API key',{exact:false})).toHaveCount(0);
  const base=`${stack.llmProxyOrigin}/api/management`;
  const account=await (await context.request.get(`${base}/account`,{headers})).json();
  const billing=await (await context.request.get(`${base}/billing-accounts`,{headers})).json();
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
    const card=hosted.locator('[data-hosted-grant]');
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
  } finally { await operator.close(); }
});
