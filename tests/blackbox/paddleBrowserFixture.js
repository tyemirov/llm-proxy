// Controlled external SDK boundary. This fixture cannot credit application funds.
window.paddleFixture={opened:[],initializations:0,token:'',environment:''};
let callback;
window.Paddle={
  Environment:{set(environment){window.paddleFixture.environment=environment;}},
  Initialize(options){
    if(++window.paddleFixture.initializations!==1)throw new Error('Paddle initialized twice');
    window.paddleFixture.token=options.token;callback=options.eventCallback;window.paddleFixture.emit=callback;
  },
  Checkout:{
    open(options){
      window.paddleFixture.opened.push(options);
      const dialog=document.createElement('dialog');dialog.setAttribute('aria-label','Controlled Paddle checkout');
      dialog.innerHTML='<button>Complete controlled checkout</button><button>Close controlled checkout</button>';
      dialog.querySelectorAll('button').forEach((button,index)=>button.addEventListener('click',()=>{
        callback({name:index===0?'checkout.completed':'checkout.closed',data:{transaction_id:options.transactionId}});
        dialog.close();dialog.remove();
      }));
      document.body.append(dialog);dialog.showModal();
    },
    close(){document.querySelector('dialog[aria-label="Controlled Paddle checkout"]')?.remove();},
  },
};
