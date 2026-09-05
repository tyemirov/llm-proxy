"""Public contract tests for the llm_proxy_client package."""

from __future__ import annotations

import json
import os
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, fields
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import pytest
import yaml

from llm_proxy_client import (
    Client,
    ClientConfig,
    ClientMessage,
    ClientMessagesRequest,
    ClientStructuredOutput,
    LLMProxyClientError,
    LLMProxyHTTPError,
    LLMProxyModelProfileError,
    LLMProxyTransportError,
    audio_asset_attachment,
    image_asset_attachment,
    image_attachment,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
CANONICAL_OPENAPI_PATH = REPOSITORY_ROOT / "docs" / "openapi.yaml"


def test_media_attachment_constructors_serialize_inline_and_asset_union_variants() -> None:
    """Image and audio constructors emit exactly one attachment union variant."""

    inline_data = b"inline-image"
    inline = image_attachment(inline_data, " IMAGE/PNG ")
    asset = audio_asset_attachment(
        "ast_0123456789abcdef0123456789abcdef",
        "audio/wav",
    )
    message = ClientMessage(role="user", content="inspect", attachments=(inline, asset))
    body = message.body()
    assert body["attachments"][0] == {
        "type": "image",
        "mime_type": "image/png",
        "data": "aW5saW5lLWltYWdl",
    }
    assert body["attachments"][1] == {
        "type": "audio",
        "mime_type": "audio/wav",
        "asset_id": "ast_0123456789abcdef0123456789abcdef",
    }
    with pytest.raises(LLMProxyClientError, match="attachments require user role"):
        ClientMessage(role="assistant", content="invalid", attachments=(inline,))


def test_client_serializes_kimi_k3_image_and_reasoning_selection(running_server: RunningServer) -> None:
    """The Python client sends one canonical Kimi K3 image request."""

    client = Client(ClientConfig(base_url=running_server.url, secret="test-secret", provider="moonshot"))
    response = client.post_messages(
        ClientMessagesRequest(
            messages=(
                ClientMessage(
                    role="user",
                    content="Inspect.",
                    attachments=(image_attachment(b"kimi-image", "image/webp"),),
                ),
            ),
            model="kimi-k3",
            reasoning_effort="max",
        )
    )

    captured_request = CapturingHandler.captured_request
    assert response == "reviewed"
    assert urllib.parse.parse_qs(urllib.parse.urlparse(captured_request.path).query)["provider"] == ["moonshot"]
    assert captured_request.body is not None
    assert captured_request.body["model"] == "kimi-k3"
    assert captured_request.body["reasoning_effort"] == "max"
    assert captured_request.body["messages"][0]["attachments"][0]["mime_type"] == "image/webp"


def test_client_upload_asset_validates_exact_response_without_exposing_bytes() -> None:
    """The asset client sends exact bytes and validates the semantic record."""

    data = b"asset-image"

    def opener(request: urllib.request.Request) -> str:
        assert request.full_url.endswith("/model/v1/assets?key=sekret")
        assert request.data == data
        assert request.headers["Content-type"] == "image/png"
        assert set(request.headers) == {"Content-type"}
        return json.dumps(
            {
                "asset_id": "ast_0123456789abcdef0123456789abcdef",
                "mime_type": "image/png",
                "size_bytes": len(data),
                "state": "available",
                "created_at": "2026-08-11T10:00:00Z",
                "expires_at": "2026-08-13T10:00:00Z",
            }
        )

    client = Client(ClientConfig(base_url="https://proxy.example/v2", secret="sekret"), opener=opener)
    asset = client.upload_asset(data, " IMAGE/PNG ")
    assert asset.asset_id == "ast_0123456789abcdef0123456789abcdef"
    assert image_asset_attachment(asset.asset_id, asset.mime_type).body()["asset_id"] == asset.asset_id


@dataclass
class CapturedRequest:
    """Captured request data from the local test server."""

    method: str = ""
    path: str = ""
    accept: str = ""
    content_type: str = ""
    request_timeout: str = ""
    idempotency_key: str = ""
    body: dict[str, Any] | None = None


class CapturingHandler(BaseHTTPRequestHandler):
    """HTTP handler that captures the request and returns a configured body."""

    captured_request = CapturedRequest()
    captured_requests: list[CapturedRequest] = []
    response_status = 200
    response_body = "reviewed"
    response_delay_seconds = 0.0

    def do_POST(self) -> None:
        """Capture one POST request."""

        body_length = int(self.headers.get("Content-Length", "0"))
        raw_body = self.rfile.read(body_length).decode("utf-8")
        captured_request = CapturedRequest(
            method=self.command,
            path=self.path,
            accept=self.headers.get("Accept", ""),
            content_type=self.headers.get("Content-Type", ""),
            request_timeout=self.headers.get("X-LLM-Proxy-Request-Timeout-Seconds", ""),
            idempotency_key=self.headers.get("Idempotency-Key", ""),
            body=json.loads(raw_body),
        )
        type(self).captured_request = captured_request
        type(self).captured_requests.append(captured_request)
        if type(self).response_delay_seconds > 0:
            time.sleep(type(self).response_delay_seconds)
        self.send_response(type(self).response_status)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header(
            "X-LLM-Proxy-Request-Timeout-Seconds",
            captured_request.request_timeout or "360",
        )
        self.end_headers()
        try:
            self.wfile.write(type(self).response_body.encode("utf-8"))
        except BrokenPipeError:
            return

    def log_message(self, format_string: str, *arguments: object) -> None:
        """Suppress default stderr logging in tests."""


@dataclass(frozen=True)
class RunningServer:
    """Local HTTP server fixture data."""

    server: ThreadingHTTPServer
    thread: threading.Thread

    @property
    def url(self) -> str:
        """Return the local server base URL."""

        return f"http://127.0.0.1:{self.server.server_port}"

    def close(self) -> None:
        """Stop the local HTTP server."""

        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=1)


@pytest.fixture()
def running_server() -> RunningServer:
    """Start a local HTTP server for client contract tests."""

    CapturingHandler.captured_request = CapturedRequest()
    CapturingHandler.captured_requests = []
    CapturingHandler.response_status = 200
    CapturingHandler.response_body = "reviewed"
    CapturingHandler.response_delay_seconds = 0.0
    server = ThreadingHTTPServer(("127.0.0.1", 0), CapturingHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    running = RunningServer(server=server, thread=thread)
    try:
        yield running
    finally:
        running.close()


def read_model_profile(profile_path: str) -> str:
    """Read one application-owned model profile as UTF-8 text."""

    return Path(profile_path).read_text(encoding="utf-8")


def replace_model_profile(profile_path: Path, profile_document: str) -> None:
    """Atomically replace one application-owned profile document."""

    replacement_path = profile_path.with_name("next-model.json")
    replacement_path.write_text(profile_document, encoding="utf-8")
    os.replace(replacement_path, profile_path)


def canonical_openapi_document() -> dict[str, Any]:
    """Load the sole committed HTTP contract."""

    document = yaml.safe_load(CANONICAL_OPENAPI_PATH.read_text(encoding="utf-8"))
    assert isinstance(document, dict)
    assert document["openapi"] == "3.1.0"
    return document


def resolve_openapi_reference(document: dict[str, Any], value: dict[str, Any]) -> dict[str, Any]:
    """Resolve one local OpenAPI reference."""

    reference = value.get("$ref")
    if reference is None:
        return value
    assert isinstance(reference, str) and reference.startswith("#/")
    resolved: Any = document
    for encoded_segment in reference.removeprefix("#/").split("/"):
        segment = encoded_segment.replace("~1", "/").replace("~0", "~")
        assert isinstance(resolved, dict)
        resolved = resolved[segment]
    assert isinstance(resolved, dict)
    return resolved


def canonical_v2_operation(document: dict[str, Any]) -> dict[str, Any]:
    """Return the canonical v2 POST operation."""

    operation = document["paths"]["/v2"]["post"]
    assert isinstance(operation, dict)
    return operation


def assert_python_v2_request_conforms_to_openapi(captured_request: CapturedRequest) -> None:
    """Prove the Python package's real serialized request uses the canonical v2 shape."""

    document = canonical_openapi_document()
    operation = canonical_v2_operation(document)
    request_body = resolve_openapi_reference(document, operation["requestBody"])
    media_contract = request_body["content"]["application/json"]
    request_schema = resolve_openapi_reference(document, media_contract["schema"])
    contract_body_fields = set(request_schema["properties"])
    client_body_fields = {
        client_field.name
        for client_field in fields(ClientMessagesRequest)
        if client_field.name != "request_timeout_seconds"
    }
    assert client_body_fields == contract_body_fields
    assert captured_request.body is not None
    assert set(captured_request.body).issubset(contract_body_fields)
    assert set(request_schema["required"]).issubset(captured_request.body)
    assert request_schema["additionalProperties"] is False
    assert isinstance(captured_request.body["web_search"], bool)

    declared_query_fields: set[str] = set()
    for raw_parameter in operation["parameters"]:
        parameter = resolve_openapi_reference(document, raw_parameter)
        if parameter["in"] == "query":
            declared_query_fields.add(parameter["name"])
    parsed_path = urllib.parse.urlparse(captured_request.path)
    actual_query_fields = set(urllib.parse.parse_qs(parsed_path.query))
    assert {"key", "provider", "format"}.issubset(declared_query_fields)
    assert contract_body_fields.isdisjoint(actual_query_fields)


def canonical_v2_response_statuses() -> set[int]:
    """Return every explicitly documented v2 response status."""

    responses = canonical_v2_operation(canonical_openapi_document())["responses"]
    return {int(status_code) for status_code in responses}


def test_canonical_openapi_declares_web_search_query_as_boolean() -> None:
    """The query contract uses one canonical boolean instead of textual aliases."""

    document = canonical_openapi_document()
    web_search_schema = document["components"]["parameters"]["WebSearch"]["schema"]

    assert web_search_schema == {"type": "boolean"}


def test_client_posts_v2_body_and_preserves_non_body_query(running_server: RunningServer) -> None:
    """The public client sends v2 messages in the body and auth in query."""

    client = Client(
        ClientConfig(
            base_url=(
                f"{running_server.url}/review?"
                "prompt=old&model=old&max_tokens=9&reasoning_effort=old&web_search=true&provider=gemini&keep=1"
            ),
            secret="test-secret",
        )
    )

    response_text = client.post_messages(
        ClientMessagesRequest(
            messages=(ClientMessage(role="user", content="Проверить текст"),),
            model="gpt-5.5",
            request_timeout_seconds=27,
        )
    )

    captured_request = CapturingHandler.captured_request
    parsed_path = urllib.parse.urlparse(captured_request.path)
    query_values = urllib.parse.parse_qs(parsed_path.query)
    assert response_text == "reviewed"
    assert captured_request.method == "POST"
    assert captured_request.accept == "text/plain"
    assert captured_request.content_type == "application/json; charset=utf-8"
    assert captured_request.request_timeout == "27"
    assert parsed_path.path == "/review/v2"
    assert query_values["key"] == ["test-secret"]
    assert query_values["format"] == ["text/plain"]
    assert query_values["provider"] == ["gemini"]
    assert query_values["keep"] == ["1"]
    for stripped_query_key in ("prompt", "model", "max_tokens", "reasoning_effort", "web_search"):
        assert stripped_query_key not in query_values
    assert captured_request.body == {
        "messages": [{"role": "user", "content": "Проверить текст"}],
        "web_search": False,
        "model": "gpt-5.5",
    }
    assert_python_v2_request_conforms_to_openapi(captured_request)


def test_client_omits_model_when_request_uses_provider_default(running_server: RunningServer) -> None:
    """Blank request model is omitted while the selected provider stays in the URL."""

    client = Client(
        ClientConfig(
            base_url=f"{running_server.url}/review?provider=gemini&model=stale&keep=1",
            secret="test-secret",
        )
    )

    response_text = client.post_messages(
        ClientMessagesRequest(messages=(ClientMessage(role="user", content="Use provider default"),))
    )

    captured_request = CapturingHandler.captured_request
    parsed_path = urllib.parse.urlparse(captured_request.path)
    query_values = urllib.parse.parse_qs(parsed_path.query)
    assert response_text == "reviewed"
    assert parsed_path.path == "/review/v2"
    assert query_values["provider"] == ["gemini"]
    assert "model" not in query_values
    assert captured_request.request_timeout == ""
    assert captured_request.body == {
        "messages": [{"role": "user", "content": "Use provider default"}],
        "web_search": False,
    }


def test_client_reloads_atomically_replaced_model_profile(running_server: RunningServer, tmp_path: Path) -> None:
    """One client reads the profile that exists at each outbound request."""

    profile_path = tmp_path / "current-model.json"
    replace_model_profile(profile_path, '{"provider":"gemini","model":"gemini-3.5-flash"}')
    client = Client(
        ClientConfig(
            base_url=running_server.url,
            secret="test-secret",
            model_profile_path=str(profile_path),
            model_profile_reader=read_model_profile,
        )
    )
    request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="Use my selected model"),))

    assert client.post_messages(request) == "reviewed"
    replace_model_profile(profile_path, '{"provider":"openai","model":"gpt-5-mini"}')
    assert client.post_messages(request) == "reviewed"

    assert [
        (
            urllib.parse.parse_qs(urllib.parse.urlparse(captured_request.path).query)["provider"],
            captured_request.body,
        )
        for captured_request in CapturingHandler.captured_requests
    ] == [
        (
            ["gemini"],
            {
                "messages": [{"role": "user", "content": "Use my selected model"}],
                "web_search": False,
                "model": "gemini-3.5-flash",
            },
        ),
        (
            ["openai"],
            {
                "messages": [{"role": "user", "content": "Use my selected model"}],
                "web_search": False,
                "model": "gpt-5-mini",
            },
        ),
    ]


def test_client_rejects_invalid_or_competing_model_profiles_before_http(
    running_server: RunningServer, tmp_path: Path
) -> None:
    """Invalid profiles and a pinned request never reuse a prior profile or call the proxy."""

    profile_path = tmp_path / "current-model.json"
    valid_profile = '{"provider":"gemini","model":"gemini-3.5-flash"}'
    replace_model_profile(profile_path, valid_profile)
    client = Client(
        ClientConfig(
            base_url=running_server.url,
            secret="test-secret",
            model_profile_path=str(profile_path),
            model_profile_reader=read_model_profile,
        )
    )
    request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="Keep the profile current"),))

    assert client.post_messages(request) == "reviewed"
    assert len(CapturingHandler.captured_requests) == 1
    invalid_profiles = [
        ('{"provider":"gemini"', "decode model_profile"),
        ('{"provider":"gemini"}', "missing model"),
        ('{"provider":"qwencloud","model":"qwen3.8-max-preview"}', "provider is retired"),
        ('{"provider":"gemini","model":"gemini-3.5-flash","secret":"forbidden"}', "unsupported field"),
        ('{"provider":"gemini","provider":"openai","model":"gpt-5-mini"}', "duplicate field"),
    ]
    for invalid_profile, expected_error in invalid_profiles:
        replace_model_profile(profile_path, invalid_profile)
        with pytest.raises(LLMProxyModelProfileError, match=expected_error):
            client.post_messages(request)
        assert len(CapturingHandler.captured_requests) == 1
        replace_model_profile(profile_path, valid_profile)

    profile_path.unlink()
    with pytest.raises(LLMProxyModelProfileError, match="read model_profile"):
        client.post_messages(request)
    assert len(CapturingHandler.captured_requests) == 1
    replace_model_profile(profile_path, valid_profile)

    pinned_request = ClientMessagesRequest(
        messages=(ClientMessage(role="user", content="Do not compete"),), model="gpt-5-mini"
    )
    with pytest.raises(LLMProxyModelProfileError, match="request model conflicts"):
        client.post_messages(pinned_request)
    assert len(CapturingHandler.captured_requests) == 1


def test_client_normalizes_model_profile_reader_failures_before_http(running_server: RunningServer) -> None:
    """Application reader failures are typed and never reach the proxy."""

    model_profile_path = "/profiles/current-model.json"

    def failing_model_profile_reader(profile_path: str) -> str:
        """Simulate an application profile-storage failure."""

        raise ValueError(f"application storage rejected {profile_path!r}")

    client = Client(
        ClientConfig(
            base_url=running_server.url,
            secret="test-secret",
            model_profile_path=model_profile_path,
            model_profile_reader=failing_model_profile_reader,
        )
    )

    with pytest.raises(LLMProxyModelProfileError, match="read model_profile") as error_info:
        client.post_messages(ClientMessagesRequest(messages=(ClientMessage(role="user", content="Keep it typed"),)))

    assert f"path={model_profile_path!r}" in str(error_info.value)
    assert "application storage rejected" in str(error_info.value)
    assert CapturingHandler.captured_requests == []


def test_client_sends_unknown_model_profile_pair_to_proxy(running_server: RunningServer, tmp_path: Path) -> None:
    """The client leaves exact provider/model validation to the proxy boundary."""

    profile_path = tmp_path / "current-model.json"
    replace_model_profile(profile_path, '{"provider":"unknown","model":"unknown-model"}')
    CapturingHandler.response_status = 400
    CapturingHandler.response_body = "unknown provider/model pair"
    client = Client(
        ClientConfig(
            base_url=running_server.url,
            secret="test-secret",
            model_profile_path=str(profile_path),
            model_profile_reader=read_model_profile,
        )
    )

    with pytest.raises(LLMProxyHTTPError) as error_info:
        client.post_messages(ClientMessagesRequest(messages=(ClientMessage(role="user", content="Route this pair"),)))

    parsed_path = urllib.parse.urlparse(CapturingHandler.captured_request.path)
    assert error_info.value.status_code == 400
    assert error_info.value.status_code in canonical_v2_response_statuses()
    assert urllib.parse.parse_qs(parsed_path.query)["provider"] == ["unknown"]
    assert CapturingHandler.captured_request.body is not None
    assert CapturingHandler.captured_request.body["model"] == "unknown-model"


def test_client_overrides_provider_and_sends_optional_v2_body_fields(running_server: RunningServer) -> None:
    """Explicit provider config overrides a provider already present in the URL."""

    client = Client(
        ClientConfig(
            base_url=f"{running_server.url}/?provider=openai&keep=1",
            secret="test-secret",
            provider="deepseek",
        )
    )

    response_text = client.post_messages(
        ClientMessagesRequest(
            messages=(
                ClientMessage(role="system", content="Be terse."),
                ClientMessage(role="user", content="Summarize"),
            ),
            web_search=True,
            max_tokens=42,
            reasoning_effort="high",
        )
    )

    captured_request = CapturingHandler.captured_request
    parsed_path = urllib.parse.urlparse(captured_request.path)
    query_values = urllib.parse.parse_qs(parsed_path.query)
    assert response_text == "reviewed"
    assert parsed_path.path == "/v2"
    assert query_values["provider"] == ["deepseek"]
    assert query_values["keep"] == ["1"]
    assert captured_request.body == {
        "messages": [
            {"role": "system", "content": "Be terse."},
            {"role": "user", "content": "Summarize"},
        ],
        "web_search": True,
        "max_tokens": 42,
        "reasoning_effort": "high",
    }
    assert_python_v2_request_conforms_to_openapi(captured_request)


def test_client_posts_v2_messages_body(running_server: RunningServer) -> None:
    """The public client can send v2 messages-only requests."""

    client = Client(ClientConfig(base_url=running_server.url, secret="test-secret"))

    response_text = client.post_messages(
        ClientMessagesRequest(
            messages=(
                ClientMessage(role="assistant", content="Hi.", order=2),
                ClientMessage(role="user", content="Hello", order=1),
            ),
            model="deepseek-v4-flash",
            web_search=True,
        )
    )

    captured_request = CapturingHandler.captured_request
    parsed_path = urllib.parse.urlparse(captured_request.path)
    query_values = urllib.parse.parse_qs(parsed_path.query)
    assert response_text == "reviewed"
    assert parsed_path.path == "/v2"
    assert query_values["key"] == ["test-secret"]
    assert captured_request.body == {
        "messages": [
            {"role": "user", "content": "Hello", "order": 1},
            {"role": "assistant", "content": "Hi.", "order": 2},
        ],
        "web_search": True,
        "model": "deepseek-v4-flash",
    }


def test_client_posts_schema_constrained_request_with_idempotency_header(running_server: RunningServer) -> None:
    """The public client binds one caller schema to one idempotency key."""

    structured_output = ClientStructuredOutput(
        schema={
            "type": "object",
            "additionalProperties": False,
            "required": ["decision"],
            "properties": {"decision": {"type": "string", "enum": ["pass", "return"]}},
        },
        idempotency_key="review:story-1",
    )
    client = Client(ClientConfig(base_url=running_server.url, secret="test-secret", provider="openai"))

    response_text = client.post_messages(
        ClientMessagesRequest(
            messages=(ClientMessage(role="user", content="Review"),),
            model="gpt-5.5",
            structured_output=structured_output,
        )
    )

    captured_request = CapturingHandler.captured_request
    assert response_text == "reviewed"
    assert captured_request.idempotency_key == "review:story-1"
    assert captured_request.body is not None
    assert captured_request.body["structured_output"] == {"schema": structured_output.schema}
    assert_python_v2_request_conforms_to_openapi(captured_request)


@pytest.mark.parametrize(
    ("config_kwargs", "expected_error"),
    [
        ({"base_url": "", "secret": "sekret"}, "missing base_url"),
        ({"base_url": "ftp://example.test", "secret": "sekret"}, "base_url must use http or https"),
        ({"base_url": "http://", "secret": "sekret"}, "base_url must include host"),
        ({"base_url": "http://example.test", "secret": ""}, "missing secret"),
        (
            {"base_url": "http://example.test", "secret": "sekret", "model_profile_path": "/profiles/user.json"},
            "model_profile_path requires model_profile_reader",
        ),
        (
            {
                "base_url": "http://example.test",
                "secret": "sekret",
                "model_profile_reader": read_model_profile,
            },
            "model_profile_reader requires model_profile_path",
        ),
        (
            {
                "base_url": "http://example.test",
                "secret": "sekret",
                "model_profile_path": "/profiles/user.json",
                "model_profile_reader": "not-a-reader",
            },
            "model_profile_reader must be callable",
        ),
        (
            {
                "base_url": "http://example.test",
                "secret": "sekret",
                "provider": "gemini",
                "model_profile_path": "/profiles/user.json",
                "model_profile_reader": read_model_profile,
            },
            "model_profile_path conflicts with provider",
        ),
        (
            {
                "base_url": "http://example.test?provider=gemini",
                "secret": "sekret",
                "model_profile_path": "/profiles/user.json",
                "model_profile_reader": read_model_profile,
            },
            "base_url provider query",
        ),
        (
            {
                "base_url": "http://example.test?model=gpt-5-mini",
                "secret": "sekret",
                "model_profile_path": "/profiles/user.json",
                "model_profile_reader": read_model_profile,
            },
            "base_url model query",
        ),
    ],
)
def test_config_validation_errors(config_kwargs: dict[str, object], expected_error: str) -> None:
    """Invalid config fails at the package boundary."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientConfig(**config_kwargs)


@pytest.mark.parametrize(
    ("request_kwargs", "expected_error"),
    [
        ({"messages": ()}, "missing messages"),
        (
            {
                "messages": (
                    ClientMessage(role="user", content="prompt", order=1),
                    ClientMessage(role="assistant", content="answer"),
                ),
            },
            "all messages must include order",
        ),
        (
            {
                "messages": (
                    ClientMessage(role="user", content="prompt", order=1),
                    ClientMessage(role="assistant", content="answer", order=1),
                ),
            },
            "duplicate message order",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "max_tokens": 0},
            "max_tokens must be positive",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "reasoning_effort": ""},
            "reasoning_effort must be nonblank",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "web_search": "true"},
            "web_search must be a boolean",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "web_search": 1},
            "web_search must be a boolean",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "request_timeout_seconds": 0},
            "request_timeout_seconds must be a positive whole number",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "request_timeout_seconds": 1.5},
            "request_timeout_seconds must be a positive whole number",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "request_timeout_seconds": True},
            "request_timeout_seconds must be a positive whole number",
        ),
        (
            {"messages": (ClientMessage(role="user", content="prompt"),), "structured_output": "invalid"},
            "invalid structured output",
        ),
        (
            {
                "messages": (ClientMessage(role="user", content="prompt"),),
                "web_search": True,
                "structured_output": ClientStructuredOutput(schema={}, idempotency_key="request-1"),
            },
            "structured output conflicts with web_search",
        ),
    ],
)
def test_messages_request_validation_errors(request_kwargs: dict[str, object], expected_error: str) -> None:
    """Invalid v2 request input fails at the package boundary."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientMessagesRequest(**request_kwargs)


@pytest.mark.parametrize(
    ("schema", "idempotency_key", "expected_error"),
    [
        ([], "request-1", "schema must be an object"),
        ({"const": float("nan")}, "request-1", "schema must be valid JSON"),
        ({}, " bad", "invalid idempotency key"),
    ],
)
def test_structured_output_validation_errors(
    schema: object, idempotency_key: str, expected_error: str
) -> None:
    """Invalid structured-output input fails before HTTP work."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientStructuredOutput(schema=schema, idempotency_key=idempotency_key)  # type: ignore[arg-type]


@pytest.mark.parametrize(
    ("message_kwargs", "expected_error"),
    [
        ({"role": "function", "content": "tool result"}, "unsupported message role"),
        ({"role": "user", "content": ""}, "empty message content"),
        ({"role": "user", "content": "prompt", "order": -1}, "message order must be non-negative"),
    ],
)
def test_message_validation_errors(message_kwargs: dict[str, object], expected_error: str) -> None:
    """Invalid message input fails at the package boundary."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientMessage(**message_kwargs)


def test_http_error_exposes_status_and_body(running_server: RunningServer) -> None:
    """Non-success HTTP responses are typed errors with status and body."""

    CapturingHandler.response_status = 502
    CapturingHandler.response_body = "upstream failed"
    client = Client(
        ClientConfig(
            base_url=f"{running_server.url}/?provider=gemini",
            secret="test-secret",
        )
    )

    with pytest.raises(LLMProxyHTTPError) as error_info:
        client.post_messages(
            ClientMessagesRequest(
                messages=(ClientMessage(role="user", content="prompt"),),
                model="gpt-5-mini",
                request_timeout_seconds=12,
            )
        )

    assert error_info.value.status_code == 502
    assert error_info.value.body == "upstream failed"
    assert error_info.value.request_context == "provider=gemini model=gpt-5-mini request_timeout_seconds=12"
    assert "provider=gemini model=gpt-5-mini request_timeout_seconds=12" in str(error_info.value)


def test_transport_error_is_typed() -> None:
    """Transport errors are surfaced separately from HTTP status errors."""

    def failing_opener(request: urllib.request.Request) -> str:
        raise urllib.error.URLError("network unavailable")

    client = Client(
        ClientConfig(
            base_url="http://example.test/?provider=gemini",
            secret="test-secret",
        ),
        opener=failing_opener,
    )

    with pytest.raises(
        LLMProxyTransportError,
        match="provider=gemini model=gpt-5-mini request_timeout_seconds=9.*network unavailable",
    ):
        client.post_messages(
            ClientMessagesRequest(
                messages=(ClientMessage(role="user", content="prompt"),),
                model="gpt-5-mini",
                request_timeout_seconds=9,
            )
        )


def test_transport_owned_timeout_is_typed_transport_error() -> None:
    """An injected transport may still enforce its independently owned cancellation policy."""

    def timing_out_opener(request: urllib.request.Request) -> str:
        raise TimeoutError("transport timed out")

    client = Client(
        ClientConfig(base_url="http://example.test", secret="test-secret"),
        opener=timing_out_opener,
    )

    with pytest.raises(
        LLMProxyTransportError,
        match="provider=omitted model=omitted request_timeout_seconds=omitted.*transport timed out",
    ):
        client.post_messages(ClientMessagesRequest(messages=(ClientMessage(role="user", content="prompt"),)))


def test_ssl_failure_is_typed_transport_error() -> None:
    """Raw socket and SSL style failures are surfaced through the transport-error contract."""

    def failing_opener(request: urllib.request.Request) -> str:
        raise OSError("record layer failure")

    client = Client(
        ClientConfig(
            base_url="http://example.test/?provider=openai",
            secret="test-secret",
        ),
        opener=failing_opener,
    )

    with pytest.raises(
        LLMProxyTransportError,
        match="provider=openai model=gpt-5.5 request_timeout_seconds=240.*record layer failure",
    ):
        client.post_messages(
            ClientMessagesRequest(
                messages=(ClientMessage(role="user", content="prompt"),),
                model="gpt-5.5",
                request_timeout_seconds=240,
            )
        )
