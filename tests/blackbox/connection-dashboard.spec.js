// @ts-check
import {expect,test} from '@playwright/test';
import {readFile,mkdir} from 'node:fs/promises';
import path from 'node:path';
import {assets,directory} from './sharedUIAssets.mjs';
import {localManagementProfile,startLocalManagementStack} from './localManagementStack.mjs';

let stack;
test.beforeAll(async()=>{stack=await startLocalManagementStack();});
test.afterAll(async()=>{if(stack)await stack.stop();});

test('account connection dashboard links Social Threader explicitly and preserves tenant defaults',async({page,context})=>{
 page.setDefaultTimeout(10000);
 const errors=[];page.on('pageerror',error=>errors.push(error.message));
 await page.route('**/api/management/connections{,?*}',route=>{
  if(route.request().method()!=='GET')return route.continue();
  const url=new URL(route.request().url());url.searchParams.set('limit','1');
  return route.continue({url:url.href});
 });
 for(const name of Object.keys(assets)){
  const body=await readFile(path.join(directory,name));
  await page.route(`https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/${name}*`,route=>route.fulfill({body,contentType:name.endsWith('.css')?'text/css':'application/javascript'}));
 }
 for(const [pattern,file] of [
  ['**/alpinejs@3.17.1/dist/module.esm.js','node_modules/alpinejs/dist/module.esm.js'],
  ['**/js-yaml@5.4.1/dist/browser/js-yaml.umd.min.js','node_modules/js-yaml/dist/browser/js-yaml.umd.min.js'],
  ['https://accounts.google.com/gsi/client','tests/blackbox/googleIdentityFixture.js']
 ]){const body=await readFile(file);await page.route(pattern,route=>route.fulfill({body,contentType:'application/javascript'}));}
 await page.route('https://loopaware.mprlab.com/**',route=>route.fulfill({body:'',contentType:'application/javascript'}));
 await page.route(`${stack.tAuthOrigin}/auth/google`,async route=>{
  const login=await context.request.post(`${stack.tAuthOrigin}/auth/password/login`,{headers:{Origin:stack.frontendOrigin,'X-TAuth-Tenant':localManagementProfile.tenantID},data:{email:localManagementProfile.operatorEmail,password:localManagementProfile.operatorPassword}});
  expect(login.ok()).toBeTruthy();
  await route.fulfill({status:login.status(),contentType:'application/json',body:await login.body()});
 });
 await page.setViewportSize({width:1440,height:1050});
 await page.goto(`${stack.frontendOrigin}/`);
 await page.getByRole('button',{name:'Sign in with Google',exact:true}).click();
 await expect(page).toHaveURL(`${stack.frontendOrigin}/app/`);
 const dashboard=page.locator('connection-dashboard');
 await expect(dashboard.getByRole('heading',{name:'Tenants → connections → models'})).toBeVisible();
 await expect(page.locator('settings-overlay')).not.toBeVisible();
 await context.grantPermissions(['clipboard-read','clipboard-write'],{origin:stack.frontendOrigin});
 await dashboard.getByRole('button',{name:'Copy MCP URL',exact:true}).click();
 expect(await page.evaluate(()=>navigator.clipboard.readText())).toBe(`${stack.llmProxyOrigin}/mcp`);
 await dashboard.getByRole('button',{name:'Create connection',exact:true}).click();
 let dialog=page.getByRole('dialog');
 await dialog.getByLabel('Connection name').fill('Production');
 await dialog.getByLabel('Provider',{exact:true}).selectOption('openai');
 await dialog.getByLabel('API key',{exact:false}).fill('sk-dashboard-key');
 const retryKeys=[];
 let loseCreationResponse=true;
 await page.route('**/api/management/connections',async route=>{
  if(route.request().method()!=='POST')return route.continue();
  retryKeys.push(route.request().headers()['idempotency-key']);
  if(loseCreationResponse){
   loseCreationResponse=false;
   const response=await route.fetch();
   expect(response.status()).toBe(201);
   return route.abort('failed');
  }
  return route.continue();
 });
 await dialog.getByRole('button',{name:'Create connection',exact:true}).click();
 await expect(dialog).toBeVisible();
 await expect(dialog.getByRole('alert')).toBeVisible();
 await expect(dialog.getByLabel('Connection name')).toHaveValue('Production');
 await dialog.getByRole('button',{name:'Create connection',exact:true}).click();
 await expect(dialog).not.toBeVisible();
 expect(retryKeys).toHaveLength(2);
 expect(retryKeys[0]).toBeTruthy();
 expect(retryKeys[1]).toBe(retryKeys[0]);
 await page.unroute('**/api/management/connections');
 await expect(dialog).not.toBeVisible();
 await expect(dashboard.locator('[data-connection-node]').filter({hasText:'Production'})).toContainText('Connected');
 await dashboard.getByRole('button',{name:'Create tenant',exact:true}).click();
 dialog=page.getByRole('dialog');
 await dialog.getByLabel('Tenant name').fill('Social Threader');
 await dialog.getByRole('button',{name:'Create tenant',exact:true}).click();
 await expect(dialog).not.toBeVisible();
 await expect(dashboard.locator('[data-tenant][aria-pressed=true]')).toContainText('Social Threader');
 await expect(dashboard.locator('[data-map]')).toContainText('Connect to choose models');
 const production=dashboard.locator('[data-connection-node]').filter({hasText:'Production'});
 await expect(production.getByRole('button',{name:'Connect',exact:true})).toBeVisible();
 await production.getByRole('button',{name:'Connect',exact:true}).click();
 await expect(production).toContainText('Connected');
 await expect(production).toContainText('Used by 2 tenants');
 await expect(dashboard.getByText('Default model',{exact:false})).toHaveCount(0);
 await expect(dashboard.locator('[data-model="gpt-4.1"] img[data-brand-id="gpt-4"]')).toBeVisible();
 await dashboard.locator('[data-model="gpt-4.1"]').click();
 await expect(dashboard.locator('.cw-wires path.preview')).toHaveCount(1);
 await dashboard.getByLabel('Tenant system prompt').fill('Keep answers concise.');
 await dashboard.getByRole('button',{name:'Save text default'}).click();
 await expect(dashboard.locator('[data-model="gpt-4.1"]')).toContainText('Default model');
 await dashboard.locator('[data-model="gpt-4.1"]').click();
 await expect(dashboard.locator('.cw-wires path.preview')).toHaveCount(0);
 await expect(dashboard.locator('[data-model="gpt-4.1"]')).not.toHaveClass(/preview/);
 await expect(page.locator('[x-ref="usageTenantSelector"] option:checked')).toHaveText('Social Threader');

 await dashboard.getByRole('button',{name:'Tenant details and API access'}).click();
 await dashboard.getByRole('button',{name:'API access',exact:true}).click();
 await page.getByRole('dialog').getByRole('button',{name:'Create API key',exact:true}).click();
 dialog=page.getByRole('dialog');
 const secret=await dialog.getByLabel('Tenant API key').inputValue();
 const example=await dialog.locator('pre').textContent();
 const route=example.match(/curl '([^']+)'/)[1];
 const payload=JSON.parse(example.match(/-d '([^']+)'/)[1]);
 expect(new URL(route).searchParams.get('key')).toBe('<generated-secret>');
 expect(example).not.toContain(secret);
 expect(payload).not.toHaveProperty('key');
 const requestURL=new URL(route);requestURL.searchParams.set('key',secret);
 const routed=await context.request.post(requestURL.href,{data:payload});
 expect(routed.status()).toBe(200);
 await expect(dialog.getByRole('button',{name:'Copy request example'})).toBeVisible();
 await dialog.getByRole('button',{name:'Close dialog',exact:true}).click();
 await production.getByRole('button',{name:'Production',exact:true}).click();
 await dashboard.getByRole('button',{name:'Detach from Social Threader'}).click();
 dialog=page.getByRole('dialog');await expect(dialog).toContainText('default routes will be cleared');
 await dialog.getByRole('button',{name:'Confirm',exact:true}).click();
 await expect(production).toContainText('Used by 1 tenant');
 await expect(production.getByRole('button',{name:'Connect',exact:true})).toBeVisible();
 await page.setViewportSize({width:390,height:844});
 await expect(dashboard.getByRole('button',{name:'Create tenant',exact:true})).toBeVisible();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 await mkdir('output/playwright',{recursive:true});
 await page.evaluate(()=>scrollTo(0,0));
 await page.screenshot({path:'output/playwright/p001-connections-mobile.png',fullPage:true});
 await page.setViewportSize({width:1440,height:1050});
 await page.evaluate(()=>scrollTo(0,0));
 await page.screenshot({path:'output/playwright/p001-connections-desktop.png',fullPage:true});
 const bounds=await production.getByRole('button',{name:'Connect',exact:true}).boundingBox();
 const cardBounds=await production.boundingBox();
 expect(bounds.x+bounds.width).toBeLessThanOrEqual(cardBounds.x+cardBounds.width);
 let creates=0;
 page.on('request',request=>{if(request.method()==='POST' && new URL(request.url()).pathname==='/api/management/connections')creates+=1;});
 let failAssignment=true;
 await page.route('**/api/management/tenants/*/connections/openai',route=>{
  if(failAssignment){failAssignment=false;return route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'assignment_unavailable'}})});}
  return route.continue();
 });
 await dashboard.getByRole('button',{name:'Create connection',exact:true}).click();
 dialog=page.getByRole('dialog');
 await dialog.getByLabel('Connection name').fill('New connection');
 await dialog.getByLabel('Provider',{exact:true}).selectOption('openai');
 await dialog.getByLabel('API key',{exact:false}).fill('sk-new-dashboard-key');
 await dialog.getByRole('button',{name:'Create connection',exact:true}).click();
 await expect(dialog).not.toBeVisible();
 await expect(dashboard.getByRole('alert')).toContainText('Connection created');
 const unassigned=dashboard.locator('[data-connection-node]').filter({hasText:'New connection'});
 await expect(unassigned).toContainText('Unassigned');
 await unassigned.getByRole('button',{name:'Connect',exact:true}).click();
 await expect(unassigned).toContainText('Connected');
 await expect(dashboard.locator('[data-connection-node]')).toHaveCount(2);
 expect(creates).toBe(1);
 expect(errors).toEqual([]);
});
