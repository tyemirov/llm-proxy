"""Exercise the native Python client through the real proxy HTTP server."""
import json
import os
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, ClientFunction, ClientFunctionCall, ClientToolChoice

client = Client(ClientConfig(os.environ['PROTOCOL_BASE_URL'], os.environ['PROTOCOL_TENANT_KEY'], provider='openai'))
tool = ClientFunction('read_file', {'type': 'object', 'properties': {'path': {'type': 'string'}}, 'required': ['path'], 'additionalProperties': False})
messages = [ClientMessage('user', 'Read the fixture.')]
first = json.loads(client.post_messages(ClientMessagesRequest(messages=messages, model='gpt-5.6', tools=[tool], tool_choice=ClientToolChoice('function', 'read_file'), parallel_tool_calls=False)))
assert first['type'] == 'tool_calls'
call = ClientFunctionCall(**first['tool_calls'][0])
messages.extend([ClientMessage('assistant', '', tool_calls=[call]), ClientMessage('tool', 'fixture read complete', tool_call_id=call.id)])
assert client.post_messages(ClientMessagesRequest(messages, 'gpt-5.6', tools=[tool])) == 'hello client'
