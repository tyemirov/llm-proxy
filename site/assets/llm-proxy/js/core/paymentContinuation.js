// @ts-check
import {APP_INTEGRITY_ERROR} from '../constants.js?v=20261119paymentfix';
const KEY='llm-proxy:payment-continuation';
/** @typedef {{order_id:string,transaction_id:string}} PaymentContinuation */
/** Read an untrusted continuation after the account is authenticated.
 * @returns {PaymentContinuation|null}
 */
export function readPaymentContinuation() {
  const encoded=sessionStorage.getItem(KEY);
  if(encoded===null)return null;
  const value=JSON.parse(encoded);
  if(!value || typeof value.order_id!=='string' || !/^funding-[a-f0-9]{32}$/.test(value.order_id) ||
    typeof value.transaction_id!=='string' || !/^txn_[a-z0-9]{26}$/.test(value.transaction_id))throw new Error(APP_INTEGRITY_ERROR);
  return value;
}

/** Remove only the handled candidate; preserve a newer link and aborted attempts.
 * @param {PaymentContinuation} [candidate]
 */
export function clearPaymentContinuation(candidate) {
  if(candidate && sessionStorage.getItem(KEY)!==JSON.stringify(candidate))return;
  sessionStorage.removeItem(KEY);
}
