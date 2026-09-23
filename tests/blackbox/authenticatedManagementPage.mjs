// @ts-check
import {expect} from '@playwright/test';
import path from 'node:path';
import {assets,directory} from './sharedUIAssets.mjs';
import {localManagementProfile} from './localManagementStack.mjs';

export async function prepareManagementPage(page,context,stack,email=localManagementProfile.operatorEmail) {
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
