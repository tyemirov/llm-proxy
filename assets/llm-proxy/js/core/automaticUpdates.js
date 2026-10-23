// @ts-check
import {MANAGEMENT_AUTO_REFRESH_INTERVAL_MILLISECONDS} from '../constants.js?v=20260903f037';

/** @param {HTMLElement} host */
function isEditing(host) {
  const focus=document.activeElement;
  return Boolean(host.querySelector('dialog[open]')) ||
    (host.contains(focus) && (focus instanceof HTMLInputElement || focus instanceof HTMLTextAreaElement || focus instanceof HTMLSelectElement));
}
/** @param {HTMLElement} host @param {AbortSignal} signal @param {()=>Promise<void>} update */
export function startAutomaticUpdates(host, signal, update) {
  let pending=false;
  const tick=async()=>{
    if (pending || signal.aborted || document.hidden || isEditing(host)) return;
    pending=true;
    try { await update(); } finally { pending=false; }
  };
  const timer=window.setInterval(()=>void tick(),MANAGEMENT_AUTO_REFRESH_INTERVAL_MILLISECONDS);
  window.addEventListener('focus',()=>void tick(),{signal});
  document.addEventListener('visibilitychange',()=>void tick(),{signal});
  signal.addEventListener('abort',()=>window.clearInterval(timer),{once:true});
}

/**
 * @param {HTMLElement & {busy:boolean,controller:AbortController}} owner
 * @param {()=>Promise<void>} read
 * @param {()=>void} render
 * @param {(error:unknown)=>void} failed
 */
export async function updateManagementData(owner, read, render, failed) {
  if (owner.busy) return;
  const signal=owner.controller.signal;
  owner.busy=true;
  const focused=document.activeElement;
  const identity=focused instanceof HTMLButtonElement && owner.contains(focused) ?
    [...focused.attributes].filter(attribute=>attribute.name.startsWith('data-')).map(attribute=>`[${attribute.name}="${CSS.escape(attribute.value)}"]`).join('') : '';
  try { await read(); }
  catch(error) { if (!signal.aborted) failed(error); }
  finally {
    if (signal!==owner.controller.signal) return;
    owner.busy=false;
    if (!signal.aborted && !isEditing(owner)) {
      render();
      const replacement=identity ? owner.querySelector(`button${identity}`) : null;
      if (replacement instanceof HTMLButtonElement && !replacement.disabled) replacement.focus();
    }
  }
}

/**
 * @template T
 * @param {(cursor:string)=>Promise<{items:T[],next_cursor:string}>} read
 * @param {number} visibleCount
 * @returns {Promise<{items:T[],next_cursor:string}>}
 */
export async function reloadVisibleCollection(read, visibleCount) {
  /** @type {T[]} */ const items=[];
  let cursor='';
  do {
    const page=await read(cursor);
    items.push(...page.items);cursor=page.next_cursor;
  } while(cursor && items.length<visibleCount);
  return {items,next_cursor:cursor};
}
