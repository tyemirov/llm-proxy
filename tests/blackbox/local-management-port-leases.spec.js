// @ts-check
import {expect,test} from '@playwright/test';
import http from 'node:http';
import {startLocalManagementStack} from './localManagementStack.mjs';

function observePortLeases(){
 const originalCreateServer=http.createServer;
 const originalFetch=globalThis.fetch;
 const leases=[];
 const health=[];
 http.createServer=function(...argumentsList){
  const server=originalCreateServer.apply(this,argumentsList);
  if(argumentsList.length===0){
   const observed={server,port:0};
   server.once('listening',()=>{const address=server.address();if(address&&typeof address!=='string')observed.port=address.port;});
   leases.push(observed);
  }
  return server;
 };
 globalThis.fetch=async function(input,options){
  const url=new URL(typeof input==='string'?input:input instanceof URL?input.href:input.url);
  if(url.pathname==='/auth/session'||url.pathname==='/config-ui.yaml')health.push({path:url.pathname,held:leases.map(lease=>lease.server.listening)});
  return originalFetch(input,options);
 };
 return {leases,health,restore(){http.createServer=originalCreateServer;globalThis.fetch=originalFetch;},originalCreateServer};
}

test('local management startup holds distinct leases until each child launch',async()=>{
 const observed=observePortLeases();
 let stack;
 let prepared=false;
 try {
  stack=await startLocalManagementStack('frontend',{configureCatalog(){
   prepared=true;
   expect(observed.leases).toHaveLength(2);
   expect(observed.leases.map(lease=>lease.server.listening)).toEqual([true,true]);
   expect(observed.leases[0].port).not.toBe(observed.leases[1].port);
  }});
  expect(prepared).toBe(true);
  const authHealth=observed.health.filter(item=>item.path==='/auth/session');
  const proxyHealth=observed.health.filter(item=>item.path==='/config-ui.yaml');
  expect(authHealth.length).toBeGreaterThan(0);
  expect(proxyHealth.length).toBeGreaterThan(0);
  for(const item of authHealth)expect(item.held).toEqual([false,true]);
  for(const item of proxyHealth)expect(item.held).toEqual([false,false]);
 } finally {
  try {if(stack)await stack.stop();}
  finally {observed.restore();}
 }
});

test('local management configuration failure releases every unlaunched lease',async()=>{
 const observed=observePortLeases();
 try {
  await expect(startLocalManagementStack('frontend',{configureCatalog(){throw new Error('controlled_configuration_failure');}})).rejects.toThrow('controlled_configuration_failure');
  expect(observed.leases).toHaveLength(2);
  expect(observed.leases.map(lease=>lease.server.listening)).toEqual([false,false]);
  for(const lease of observed.leases){
   const probe=observed.originalCreateServer();
   try {await new Promise((resolve,reject)=>{probe.once('error',reject);probe.listen(lease.port,resolve);});}
   finally {if(probe.listening)await new Promise((resolve,reject)=>probe.close(error=>error?reject(error):resolve(undefined)));}
  }
 } finally {observed.restore();}
});
