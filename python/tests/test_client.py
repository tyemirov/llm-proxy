"""Public contract tests for the llm_proxy_client package."""

from __future__ import annotations

import json
import os
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import pytest

from llm_proxy_client import (
    Client,
    ClientConfig,
    ClientMessagesRequest,
    ClientMessage,
    LLMProxyClientError,
    LLMProxyHTTPError,
    LLMProxyModelProfileError,
    LLMProxyTransportError,
)


@dataclass
class CapturedRequest:
    """Captured request data from the local test server."""

    method: str = ""
    path: str = ""
    accept: str = ""
    content_type: str = ""
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
            body=json.loads(raw_body),
        )
        type(self).captured_request = captured_request
        type(self).captured_requests.append(captured_request)
        if type(self).response_delay_seconds > 0:
            time.sleep(type(self).response_delay_seconds)
        self.send_response(type(self).response_status)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
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


def test_client_posts_v2_body_and_preserves_non_body_query(running_server: RunningServer) -> None:
    """The public client sends v2 messages in the body and auth in query."""

    client = Client(
        ClientConfig(
            base_url=(
                f"{running_server.url}/review?"
                "prompt=old&model=old&max_tokens=9&web_search=true&provider=gemini&keep=1"
            ),
            secret="test-secret",
        )
    )

    response_text = client.post_messages(
        ClientMessagesRequest(
            messages=(ClientMessage(role="user", content="Проверить текст"),),
            model="gpt-5.5",
        )
    )

    captured_request = CapturingHandler.captured_request
    parsed_path = urllib.parse.urlparse(captured_request.path)
    query_values = urllib.parse.parse_qs(parsed_path.query)
    assert response_text == "reviewed"
    assert captured_request.method == "POST"
    assert captured_request.accept == "text/plain"
    assert captured_request.content_type == "application/json; charset=utf-8"
    assert parsed_path.path == "/review/v2"
    assert query_values["key"] == ["test-secret"]
    assert query_values["format"] == ["text/plain"]
    assert query_values["provider"] == ["gemini"]
    assert query_values["keep"] == ["1"]
    for stripped_query_key in ("prompt", "model", "max_tokens", "web_search"):
        assert stripped_query_key not in query_values
    assert captured_request.body == {
        "messages": [{"role": "user", "content": "Проверить текст"}],
        "web_search": False,
        "model": "gpt-5.5",
    }


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
    assert captured_request.body == {
        "messages": [{"role": "user", "content": "Use provider default"}],
        "web_search": False,
    }


def test_client_reloads_atomically_replaced_model_profile(running_server: RunningServer, tmp_path: Path) -> None:
    """One client reads the profile that exists at each outbound request."""

    profile_path = tmp_path / "current-model.json"
    replace_model_profile(profile_path, '{"provider":"gemini","model":"gemini-2.5-flash"}')
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
                "model": "gemini-2.5-flash",
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
    valid_profile = '{"provider":"gemini","model":"gemini-2.5-flash"}'
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
        ('{"provider":"gemini","model":"gemini-2.5-flash","secret":"forbidden"}', "unsupported field"),
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
    }


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


@pytest.mark.parametrize(
    ("config_kwargs", "expected_error"),
    [
        ({"base_url": "", "secret": "sekret"}, "missing base_url"),
        ({"base_url": "ftp://example.test", "secret": "sekret"}, "base_url must use http or https"),
        ({"base_url": "http://", "secret": "sekret"}, "base_url must include host"),
        ({"base_url": "http://example.test", "secret": ""}, "missing secret"),
        ({"base_url": "http://example.test", "secret": "sekret", "timeout_seconds": 0}, "timeout_seconds must be positive"),
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
    ],
)
def test_messages_request_validation_errors(request_kwargs: dict[str, object], expected_error: str) -> None:
    """Invalid v2 request input fails at the package boundary."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientMessagesRequest(**request_kwargs)


@pytest.mark.parametrize(
    ("message_kwargs", "expected_error"),
    [
        ({"role": "tool", "content": "tool result"}, "unsupported message role"),
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
            timeout_seconds=12,
        )
    )

    with pytest.raises(LLMProxyHTTPError) as error_info:
        client.post_messages(
            ClientMessagesRequest(
                messages=(ClientMessage(role="user", content="prompt"),),
                model="gpt-5-mini",
            )
        )

    assert error_info.value.status_code == 502
    assert error_info.value.body == "upstream failed"
    assert error_info.value.request_context == "provider=gemini model=gpt-5-mini timeout_seconds=12"
    assert "provider=gemini model=gpt-5-mini timeout_seconds=12" in str(error_info.value)


def test_transport_error_is_typed() -> None:
    """Transport errors are surfaced separately from HTTP status errors."""

    def failing_opener(request: urllib.request.Request, timeout: float) -> str:
        raise urllib.error.URLError("network unavailable")

    client = Client(
        ClientConfig(
            base_url="http://example.test/?provider=gemini",
            secret="test-secret",
            timeout_seconds=9,
        ),
        opener=failing_opener,
    )

    with pytest.raises(
        LLMProxyTransportError,
        match="provider=gemini model=gpt-5-mini timeout_seconds=9.*network unavailable",
    ):
        client.post_messages(
            ClientMessagesRequest(
                messages=(ClientMessage(role="user", content="prompt"),),
                model="gpt-5-mini",
            )
        )


def test_read_timeout_is_typed_transport_error(running_server: RunningServer) -> None:
    """Socket read timeouts are surfaced through the transport-error contract."""

    CapturingHandler.response_delay_seconds = 0.3
    client = Client(
        ClientConfig(
            base_url=running_server.url,
            secret="test-secret",
            timeout_seconds=0.05,
        )
    )

    with pytest.raises(LLMProxyTransportError, match="provider=omitted model=omitted timeout_seconds=0.05.*timed out"):
        client.post_messages(ClientMessagesRequest(messages=(ClientMessage(role="user", content="prompt"),)))


def test_ssl_failure_is_typed_transport_error() -> None:
    """Raw socket and SSL style failures are surfaced through the transport-error contract."""

    def failing_opener(request: urllib.request.Request, timeout: float) -> str:
        raise OSError("record layer failure")

    client = Client(
        ClientConfig(
            base_url="http://example.test/?provider=openai",
            secret="test-secret",
            timeout_seconds=240,
        ),
        opener=failing_opener,
    )

    with pytest.raises(
        LLMProxyTransportError,
        match="provider=openai model=gpt-5.5 timeout_seconds=240.*record layer failure",
    ):
        client.post_messages(
            ClientMessagesRequest(
                messages=(ClientMessage(role="user", content="prompt"),),
                model="gpt-5.5",
            )
        )
