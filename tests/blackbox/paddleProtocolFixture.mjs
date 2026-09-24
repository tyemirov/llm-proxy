// @ts-check
import http from 'node:http';
import {createHmac} from 'node:crypto';

const priceID='pri_01hv8x2axb33yr5y238zfwcn5p';
const itemID='txnitm_01hv8x2axb33yr5y238zfwcn5p';
const apiKey='browser-paddle-fixture-key';
const webhookSecret='browser-paddle-fixture-secret';
const timestamp='2026-09-23T12:00:00Z';

/** Controlled Paddle HTTP boundary; the application and its financial database stay real. */
export async function startPaddleProtocolFixture() {
  const transactions=[],adjustments=[];
  const customers=new Map();
  let eventSequence=0,portalCalls=0;
  const failures=[];
  const server=http.createServer(async(request,response)=>{
    try {
      if(request.headers.authorization!==`Bearer ${apiKey}`)throw new Error('Paddle authorization absent');
      const url=new URL(request.url,'http://localhost');
      response.setHeader('Content-Type','application/json');
      const send=data=>response.end(JSON.stringify({data,meta:{pagination:{has_more:false}}}));
      if(request.method==='GET' && url.pathname===`/prices/${priceID}`) {
        return send({id:priceID,billing_cycle:null,unit_price:{amount:'500',currency_code:'USD'}});
      }
      if(request.method==='GET' && url.pathname==='/customers') {
        if(!url.searchParams.get('email'))throw new Error('Missing customer email');
        const email=url.searchParams.get('email');
        if(!customers.has(email))customers.set(email,`ctm_${String(customers.size+1).padStart(26,'0')}`);
        return send([{id:customers.get(email)}]);
      }
      if(request.method==='POST' && url.pathname==='/transactions') {
        const chunks=[];for await(const chunk of request)chunks.push(chunk);
        const input=JSON.parse(Buffer.concat(chunks).toString());
        if(![...customers.values()].includes(input.customer_id) || input.items.length!==1 || input.items[0].price_id!==priceID || input.items[0].quantity!==1 || !input.custom_data.funding_order_id || !input.custom_data.billing_account_id)throw new Error('Unbound transaction');
        const transaction={id:`txn_${String(transactions.length+1).padStart(26,'0')}`,status:'ready',currency_code:'USD',collection_mode:'automatic',customer_id:input.customer_id,custom_data:input.custom_data,items:[{quantity:1,price:{id:priceID,unit_price:{amount:'500',currency_code:'USD'}}}]};
        transactions.push(transaction);return send({id:transaction.id});
      }
      if(request.method==='GET' && url.pathname==='/transactions')return send(transactions.filter(item=>item.customer_id===url.searchParams.get('customer_id')));
      if(request.method==='GET' && url.pathname==='/adjustments')return send(adjustments.filter(item=>item.transaction_id===url.searchParams.get('transaction_id')));
      const transaction=transactions.find(item=>url.pathname===`/transactions/${item.id}`);
      if(request.method==='GET' && transaction)return send(transaction);
      if(request.method==='POST' && [...customers.values()].some(id=>url.pathname===`/customers/${id}/portal-sessions`)) {
        portalCalls++;return send({urls:{general:{overview:`https://customer-portal.paddle.com/browser-fixture?token=${portalCalls}`}}});
      }
      throw new Error(`Unexpected Paddle request: ${request.method} ${url.pathname}`);
    } catch(error) {failures.push(String(error));response.statusCode=500;response.end('{}');}
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  const origin=`http://127.0.0.1:${server.address().port}`;
  const totals=(principal)=>({subtotal:String(principal),tax:String(principal/10),total:String(principal*1.1),grand_total:String(principal*1.1),grand_total_tax:String(principal/10),fee:'30',retained_fee:'0',earnings:String(principal-30),currency_code:'USD'});
  return {
    config:{environment:'sandbox',client_token:'test_browserfixture',processor_account_id:'browser-processor',supplier_id:'browser-supplier',api_key:apiKey,api_base_url:origin,webhook_secret:webhookSecret,offers:[{code:'five',price_id:priceID,funding_cents:500}]},
    transactions,failures,
    get portalCalls(){return portalCalls;},
    cancel(orderID) {
      const transaction=transactions.find(item=>item.custom_data.funding_order_id===orderID);
      Object.assign(transaction,{status:'canceled',updated_at:'2026-09-23T12:05:00Z'});
      return structuredClone(transaction);
    },
    complete(orderID) {
      const transaction=transactions.find(item=>item.custom_data.funding_order_id===orderID);
      if(!transaction)throw new Error('Checkout transaction is absent');
      Object.assign(transaction,{status:'completed',completed_at:timestamp,updated_at:'2026-09-23T12:00:01Z',invoice_number:'INV-BROWSER-1',payments:[{payment_attempt_id:'browser-payment',amount:'550',status:'captured',captured_at:timestamp}],details:{
        adjusted_totals:totals(500),
        totals:{...totals(500),discount:'0',credit:'0',credit_to_balance:'0',balance:'0'},
        payout_totals:{...totals(500),discount:'0',credit:'0',credit_to_balance:'0',balance:'0'},
        line_items:[{id:itemID,price_id:priceID,quantity:1,totals:{subtotal:'500',discount:'0',tax:'50',total:'550'}}],
      }});
      return structuredClone(transaction);
    },
    refund(orderID,status,revision) {
      const transaction=transactions.find(item=>item.custom_data.funding_order_id===orderID);
      const updated=`2026-09-23T12:${String(revision).padStart(2,'0')}:00Z`;
      const adjustment={id:'adj_00000000000000000000000001',action:'refund',type:'partial',status,transaction_id:transaction.id,customer_id:transaction.customer_id,currency_code:'USD',created_at:'2026-09-23T12:01:00Z',updated_at:updated,items:[{id:'adjitm_browser',item_id:itemID,type:'partial',amount:'200',totals:{subtotal:'200',tax:'20',total:'220'}}],totals:{subtotal:'200',tax:'20',total:'220',fee:'0',retained_fee:'0',earnings:'200',currency_code:'USD'}};
      transaction.updated_at=updated;transaction.details.adjusted_totals=totals(status==='approved'?300:500);
      adjustments.splice(0,adjustments.length,adjustment);
      return structuredClone(adjustment);
    },
    async event(serviceOrigin,eventType,data) {
      const payload=JSON.stringify({event_id:`evt_${String(++eventSequence).padStart(26,'0')}`,event_type:eventType,occurred_at:timestamp,data});
      const ts=Math.floor(Date.now()/1000);
      const signature=createHmac('sha256',webhookSecret).update(`${ts}:${payload}`).digest('hex');
      const response=await fetch(`${serviceOrigin}/api/payments/paddle/events`,{method:'POST',headers:{'Content-Type':'application/json','Paddle-Signature':`ts=${ts};h1=${signature}`},body:payload});
      if(response.status!==200)throw new Error(`Webhook failed: ${response.status} ${await response.text()}`);
    },
    async stop(){await new Promise((resolve,reject)=>server.close(error=>error?reject(error):resolve()));},
  };
}
