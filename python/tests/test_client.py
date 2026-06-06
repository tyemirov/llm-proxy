"""Public contract tests for the llm_proxy_client package."""

from __future__ import annotations

import json
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

import pytest

from llm_proxy_client import (
    Client,
    ClientConfig,
    ClientRequest,
    LLMProxyClientError,
    LLMProxyHTTPError,
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
    response_status = 200
    response_body = "reviewed"
    response_delay_seconds = 0.0

    def do_POST(self) -> None:
        """Capture one POST request."""

        body_length = int(self.headers.get("Content-Length", "0"))
        raw_body = self.rfile.read(body_length).decode("utf-8")
        type(self).captured_request = CapturedRequest(
            method=self.command,
            path=self.path,
            accept=self.headers.get("Accept", ""),
            content_type=self.headers.get("Content-Type", ""),
            body=json.loads(raw_body),
        )
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


def test_client_posts_json_body_and_preserves_non_body_query(running_server: RunningServer) -> None:
    """The public client sends prompt fields in the body and auth in query."""

    client = Client(
        ClientConfig(
            base_url=(
                f"{running_server.url}/review?"
                "prompt=old&model=old&max_tokens=9&web_search=true&provider=gemini&keep=1"
            ),
            secret="test-secret",
        )
    )

    response_text = client.post(ClientRequest(prompt="Проверить текст", model="gpt-5.5"))

    captured_request = CapturingHandler.captured_request
    parsed_path = urllib.parse.urlparse(captured_request.path)
    query_values = urllib.parse.parse_qs(parsed_path.query)
    assert response_text == "reviewed"
    assert captured_request.method == "POST"
    assert captured_request.accept == "text/plain"
    assert captured_request.content_type == "application/json; charset=utf-8"
    assert parsed_path.path == "/review"
    assert query_values["key"] == ["test-secret"]
    assert query_values["format"] == ["text/plain"]
    assert query_values["provider"] == ["gemini"]
    assert query_values["keep"] == ["1"]
    for stripped_query_key in ("prompt", "model", "max_tokens", "web_search"):
        assert stripped_query_key not in query_values
    assert captured_request.body == {
        "prompt": "Проверить текст",
        "web_search": False,
        "model": "gpt-5.5",
    }


def test_client_overrides_provider_and_sends_optional_body_fields(running_server: RunningServer) -> None:
    """Explicit provider config overrides a provider already present in the URL."""

    client = Client(
        ClientConfig(
            base_url=f"{running_server.url}/?provider=openai&keep=1",
            secret="test-secret",
            provider="deepseek",
        )
    )

    response_text = client.post(
        ClientRequest(
            prompt="Summarize",
            web_search=True,
            system_prompt="Be terse.",
            max_tokens=42,
        )
    )

    query_values = urllib.parse.parse_qs(urllib.parse.urlparse(CapturingHandler.captured_request.path).query)
    assert response_text == "reviewed"
    assert query_values["provider"] == ["deepseek"]
    assert query_values["keep"] == ["1"]
    assert CapturingHandler.captured_request.body == {
        "prompt": "Summarize",
        "web_search": True,
        "system_prompt": "Be terse.",
        "max_tokens": 42,
    }


@pytest.mark.parametrize(
    ("config_kwargs", "expected_error"),
    [
        ({"base_url": "", "secret": "sekret"}, "missing base_url"),
        ({"base_url": "ftp://example.test", "secret": "sekret"}, "base_url must use http or https"),
        ({"base_url": "http://", "secret": "sekret"}, "base_url must include host"),
        ({"base_url": "http://example.test", "secret": ""}, "missing secret"),
        ({"base_url": "http://example.test", "secret": "sekret", "timeout_seconds": 0}, "timeout_seconds must be positive"),
    ],
)
def test_config_validation_errors(config_kwargs: dict[str, object], expected_error: str) -> None:
    """Invalid config fails at the package boundary."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientConfig(**config_kwargs)


@pytest.mark.parametrize(
    ("request_kwargs", "expected_error"),
    [
        ({"prompt": ""}, "missing prompt"),
        ({"prompt": "prompt", "max_tokens": 0}, "max_tokens must be positive"),
    ],
)
def test_request_validation_errors(request_kwargs: dict[str, object], expected_error: str) -> None:
    """Invalid request input fails at the package boundary."""

    with pytest.raises(LLMProxyClientError, match=expected_error):
        ClientRequest(**request_kwargs)


def test_http_error_exposes_status_and_body(running_server: RunningServer) -> None:
    """Non-success HTTP responses are typed errors with status and body."""

    CapturingHandler.response_status = 502
    CapturingHandler.response_body = "upstream failed"
    client = Client(ClientConfig(base_url=running_server.url, secret="test-secret"))

    with pytest.raises(LLMProxyHTTPError) as error_info:
        client.post(ClientRequest(prompt="prompt"))

    assert error_info.value.status_code == 502
    assert error_info.value.body == "upstream failed"


def test_transport_error_is_typed() -> None:
    """Transport errors are surfaced separately from HTTP status errors."""

    def failing_opener(request: urllib.request.Request, timeout: float) -> str:
        raise urllib.error.URLError("network unavailable")

    client = Client(ClientConfig(base_url="http://example.test", secret="test-secret"), opener=failing_opener)

    with pytest.raises(LLMProxyTransportError, match="network unavailable"):
        client.post(ClientRequest(prompt="prompt"))


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

    with pytest.raises(LLMProxyTransportError, match="timed out"):
        client.post(ClientRequest(prompt="prompt"))
