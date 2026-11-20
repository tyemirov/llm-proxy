// @ts-check
// Run synchronously before external scripts: Paddle.Initialize automatically opens _ptxn.
(()=>{
  const key='llm-proxy:payment-continuation';
  const url=new URL(window.location.href);
  const orders=url.searchParams.getAll('funding_order_id'),transactions=url.searchParams.getAll('_ptxn');
  if (!orders.length && !transactions.length) return;
  url.searchParams.delete('funding_order_id');url.searchParams.delete('_ptxn');
  history.replaceState(history.state,'',url.pathname+url.search+url.hash);
  const valid=orders.length===1 && transactions.length===1 && /^funding-[a-f0-9]{32}$/.test(orders[0]) && /^txn_[a-z0-9]{26}$/.test(transactions[0]);
  sessionStorage.setItem(key,JSON.stringify(valid?{order_id:orders[0],transaction_id:transactions[0]}:{invalid:true}));
})();
