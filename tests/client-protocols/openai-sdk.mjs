// @ts-check
import assert from 'node:assert/strict';
import OpenAI, { toFile } from 'openai';
const client = new OpenAI({baseURL:process.env.PROTOCOL_BASE_URL,apiKey:process.env.PROTOCOL_TENANT_KEY,maxRetries:0});
const model = 'openai/gpt-5.6';
const parameters = {type:'object',properties:{path:{type:'string'}},required:['path'],additionalProperties:false};
const tools = [{type:'function',function:{name:'read_file',parameters}}];
const messages = [{role:'user',content:'Read the test file.'}];
const first = await client.chat.completions.create({model,messages,tools});
assert.equal(first.choices[0].finish_reason,'tool_calls');
const call=first.choices[0].message.tool_calls[0];
assert.equal(call.function.name,'read_file');
const second = await client.chat.completions.create({model,tools,messages:[...messages,first.choices[0].message,{role:'tool',tool_call_id:call.id,content:'fixture read complete'}]});
assert.equal(second.choices[0].message.content,'hello client');
const stream = await client.chat.completions.create({model,messages,stream:true,stream_options:{include_usage:true}});
let text='';let usage;
for await (const chunk of stream){text+=chunk.choices[0]?.delta.content??'';if(chunk.usage)usage=chunk.usage;}
assert.equal(text,'hello client');assert.equal(usage.total_tokens,13);
const responseTools=[{type:'function',name:'read_file',parameters}];
const response=await client.responses.create({model,input:'Read the test file.',tools:responseTools,store:false});
assert.equal(response.output[0].type,'function_call');
const next=await client.responses.create({model,store:false,tools:responseTools,input:[{role:'user',content:'Read the test file.'},...response.output,{type:'function_call_output',call_id:response.output[0].call_id,output:'fixture read complete'}]});
assert.equal(next.output_text,'hello client');
const events=await client.responses.create({model,input:'hello',stream:true,store:false});
let sequence=-1;let finished=false;
for await (const event of events){assert.equal(event.sequence_number,++sequence);if(event.type==='response.completed'){finished=true;assert.equal(event.response.output[0].content[0].text,'hello client');}}
assert.ok(finished);
const models=await client.models.list();assert.ok(models.data.some(record=>record.id===model));
const transcription=await client.audio.transcriptions.create({model:'openai/gpt-4o-transcribe',file:await toFile(Buffer.from('fixture audio'),'sample.wav')});
assert.equal(transcription.text,'fixture transcription');
await assert.rejects(client.responses.create({model,input:'hello',store:true}),error=>error.status===400&&error.error.code==='invalid_request');
console.log('OpenAI SDK text, tools, events, discovery, transcription, and errors passed.');
