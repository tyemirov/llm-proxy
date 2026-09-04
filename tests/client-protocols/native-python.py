"""Exercise the native Python client through the real proxy HTTP server."""
import json
import os
from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, ClientFunction, ClientFunctionCall, ClientToolChoice, LLMProxyClientError

client = Client(ClientConfig(os.environ['PROTOCOL_BASE_URL'], os.environ['PROTOCOL_TENANT_KEY'], provider='openai'))
tool = ClientFunction('read_file', {'type': 'object', 'properties': {'path': {'type': 'string'}}, 'required': ['path'], 'additionalProperties': False})
messages = [ClientMessage('user', 'Read the fixture.')]
first = json.loads(client.post_messages(ClientMessagesRequest(messages=messages, model='gpt-5.6', tools=[tool], tool_choice=ClientToolChoice('function', 'read_file'), parallel_tool_calls=False)))
assert first['type'] == 'tool_calls'
call = ClientFunctionCall(**first['tool_calls'][0])
messages.extend([ClientMessage('assistant', '', tool_calls=[call]), ClientMessage('tool', 'fixture read complete', tool_call_id=call.id)])
assert client.post_messages(ClientMessagesRequest(messages, 'gpt-5.6', tools=[tool])) == 'hello client'

failures = []
for assistant_role, tool_role, content in [
    ('Assistant', 'tool', 'fixture read complete'),
    ('assistant', ' TOOL ', 'fixture read complete'),
    ('assistant', ' TOOL ', ''),
    (' Assistant ', 'Tool', ''),
]:
    try:
        history = [
            ClientMessage(' User ', 'Read the fixture.'),
            ClientMessage(assistant_role, '', tool_calls=[call]),
            ClientMessage(tool_role, content, tool_call_id=call.id),
        ]
        request = ClientMessagesRequest(history, 'gpt-5.6', tools=[tool])
        assert [message['role'] for message in request.body()['messages']] == ['user', 'assistant', 'tool']
        assert client.post_messages(request) == 'hello client'
    except LLMProxyClientError as error:
        failures.append(f'{assistant_role!r}, {tool_role!r}, {content!r}: {error}')
assert not failures, '\n'.join(failures)

for history, expected_error in [
    ([ClientMessage(' User ', 'read', tool_calls=[call])], 'function calls require assistant role'),
    ([ClientMessage('user', 'read'), ClientMessage(' TOOL ', '', tool_call_id='unknown')], 'unmatched tool result'),
    ([ClientMessage('user', 'read'), ClientMessage(' Assistant ', '', tool_calls=[call])], 'missing tool result'),
]:
    try:
        ClientMessagesRequest(history, 'gpt-5.6', tools=[tool])
    except LLMProxyClientError as error:
        assert str(error) == expected_error
    else:
        raise AssertionError(f'invalid history accepted: {expected_error}')
