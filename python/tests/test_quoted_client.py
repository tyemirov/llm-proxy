"""Quoted client protocol tests use a real local HTTP listener."""
from __future__ import annotations

import json
import threading
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Iterator

from llm_proxy_client import Client, ClientConfig, ClientMessage, ClientMessagesRequest, ClientStructuredOutput
from llm_proxy_client.quoted import FinancialQuoteInput, QuotedClient


@contextmanager
def quote_server() -> Iterator[tuple[str, list[dict[str, Any]]]]:
    calls: list[dict[str, Any]] = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:
            payload = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            calls.append({"path": self.path, "headers": dict(self.headers), "body": payload})
            response = {"id": "quote-" + "1" * 32, "revision": 1, "state": "reserved", "execution_kind": "native_v2", "processing_digest": payload["processing_digest"], "request_digest": "b" * 64, "request_id": "request-" + "2" * 32, "reservation_id": "request-" + "2" * 32, "pricing_revision": "prices-v1", "price_snapshot_digest": "c" * 64, "currency": "USD", "maximum_cents": "25", "expires_at": "2026-11-09T20:00:00Z", "created_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:00Z", "execution_id": ""}
            encoded = json.dumps(response).encode()
            self.send_response(201)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=lambda: server.serve_forever(poll_interval=0.02), daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", calls
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def test_quoted_creation_uses_explicit_frozen_native_intent() -> None:
    with quote_server() as (url, calls):
        client = QuotedClient(Client(ClientConfig(base_url=url, secret="tenant-key")))
        request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="Fictional document facts"),), model="exact-model", structured_output=ClientStructuredOutput(schema={"type": "object"}), idempotency_key="unused-native-header-key")
        intent = FinancialQuoteInput.native("a" * 64, "openai", request)
        quote = client.create_financial_quote("quote-key", intent)
        assert quote.maximum_cents == "25"
        assert quote.state == "reserved"
        assert len(calls) == 1
        assert calls[0]["path"] == "/model/v1/financial-quotes"
        assert calls[0]["headers"]["Authorization"] == "Bearer tenant-key"
        assert calls[0]["headers"]["Idempotency-Key"] == "quote-key"
        assert calls[0]["body"]["processing_digest"] == "a" * 64

from dataclasses import FrozenInstanceError
from typing import Callable

import pytest
from llm_proxy_client import ClientMediaOperationInput, LLMProxyClientError, LLMProxyHTTPError, LLMProxyTransportError


def snapshot() -> dict[str, Any]:
    return {"id": "quote-" + "1" * 32, "revision": 1, "state": "reserved", "execution_kind": "native_v2", "processing_digest": "a" * 64, "request_digest": "b" * 64, "request_id": "request-" + "2" * 32, "reservation_id": "request-" + "2" * 32, "pricing_revision": "prices-v1", "price_snapshot_digest": "c" * 64, "currency": "USD", "maximum_cents": "25", "expires_at": "2026-11-09T20:00:00Z", "created_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:00Z", "execution_id": ""}


def intent(model: str = "exact-model") -> FinancialQuoteInput:
    request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="Fictional facts"),), model=model, structured_output=ClientStructuredOutput(schema={"type": "object"}), idempotency_key="constructor-key")
    return FinancialQuoteInput.native("a" * 64, "openai", request)


def execution_headers() -> dict[str, str]:
    return {"X-LLM-Proxy-Quote-ID": "quote-" + "1" * 32, "X-LLM-Proxy-Processing-Digest": "a" * 64, "X-LLM-Proxy-Request-Digest": "b" * 64}


@contextmanager
def protocol_server(handler: Callable[[str, str, dict[str, str], dict[str, Any] | None], tuple[int, dict[str, str], Any]]) -> Iterator[tuple[QuotedClient, list[str]]]:
    calls: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def respond(self) -> None:
            calls.append(f"{self.command} {self.path}")
            raw = self.rfile.read(int(self.headers.get("Content-Length", "0")))
            body = json.loads(raw) if raw else None
            status, headers, payload = handler(self.command, self.path, dict(self.headers), body)
            encoded = json.dumps(payload).encode()
            self.send_response(status)
            for name, value in headers.items():
                self.send_header(name, value)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)

        do_POST = respond
        do_GET = respond
        do_PATCH = respond

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=lambda: server.serve_forever(poll_interval=0.02), daemon=True)
    thread.start()
    try:
        yield QuotedClient(Client(ClientConfig(base_url=f"http://127.0.0.1:{server.server_port}", secret="tenant-key"))), calls
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


def test_lookup_release_and_original_snapshot_are_independent() -> None:
    value = snapshot()

    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        assert headers["Authorization"] == "Bearer tenant-key"
        if path == "/model/v1/financial-quotes":
            assert headers["Idempotency-Key"] == "original-key"
        if method == "PATCH":
            assert body == {"revision": value["revision"], "state": "released"}
            value["state"] = "released"
            value["revision"] = 2
        return 200, {}, value

    with protocol_server(handler) as (client, calls):
        created = client.create_financial_quote("original-key", intent())
        recovered = client.get_financial_quote("original-key")
        by_id = client.get_financial_quote_by_id(recovered.id)
        released = client.release_financial_quote(by_id)
        assert released.state == "released"
        assert created.state == "reserved"
        assert client.release_financial_quote(released).state == "released"
        assert len(calls) == 5
        with pytest.raises(FrozenInstanceError):
            released.state = "unknown"  # type: ignore[misc]


def test_native_pending_complete_and_no_second_execution_key() -> None:
    pending = True

    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        if not path.endswith("/executions"):
            return 201, {}, snapshot()
        assert "Idempotency-Key" not in headers
        if pending:
            return 202, execution_headers(), {"state": "dispatched", "proxy_request_id": "execution-one", "started_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:01Z", "elapsed_seconds": 1}
        return 200, execution_headers(), {"answer": "supported fictional claim"}

    with protocol_server(handler) as (client, calls):
        frozen = intent()
        quote = client.create_financial_quote("original-key", frozen)
        result = client.create_quoted_execution(quote, frozen)
        assert result.native is not None and result.native.state == "dispatched"
        assert result.pending_error() is not None
        pending = False
        result = client.get_quoted_execution(quote)
        assert result.native is not None and result.native.state == "succeeded"
        assert json.loads(result.native.output) == {"answer": "supported fictional claim"}
        assert result.pending_error() is None
        assert len(calls) == 3


@pytest.mark.parametrize("field,invalid", [("id", "quote-" + "9" * 32), ("revision", 0), ("revision", True), ("reservation_id", "wrong"), ("state", "unknown"), ("execution_kind", "text"), ("maximum_cents", 25), ("maximum_cents", "1.5"), ("maximum_cents", "01"), ("maximum_cents", "9223372036854775808"), ("maximum_cents", "9" * 5000), ("currency", "EUR"), ("execution_id", "unexpected"), ("updated_at", "2026-11-09T190000+00:00"), ("execution_id", None), ("processing_digest", "wrong")])
def test_reject_invalid_quote_evidence_without_retry(field: str, invalid: Any) -> None:
    value = snapshot()
    value[field] = invalid
    with protocol_server(lambda *_: (200, {}, value)) as (client, calls):
        with pytest.raises(LLMProxyTransportError):
            client.get_financial_quote_by_id("quote-" + "1" * 32)
        assert len(calls) == 1


@pytest.mark.parametrize("field", ["execution_id", "execution_kind", "maximum_cents"])
def test_reject_missing_required_quote_fields(field: str) -> None:
    value = snapshot()
    del value[field]
    with protocol_server(lambda *_: (200, {}, value)) as (client, calls):
        with pytest.raises(LLMProxyTransportError):
            client.get_financial_quote("original-key")
        assert len(calls) == 1


@pytest.mark.parametrize("header", list(execution_headers()))
def test_reject_cross_intent_execution_identity(header: str) -> None:
    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        if not path.endswith("/executions"):
            return 200, {}, snapshot()
        identity = execution_headers()
        identity[header] = "wrong"
        return 200, identity, {"answer": "untrusted"}

    with protocol_server(handler) as (client, calls):
        quote = client.get_financial_quote("original-key")
        with pytest.raises(LLMProxyTransportError):
            client.get_quoted_execution(quote)
        assert len(calls) == 2


@pytest.mark.parametrize("state", ["reserved", "consumed", "expired", "released"])
def test_read_states_do_not_infer_release_or_require_payload(state: str) -> None:
    value = snapshot()
    value["state"] = state
    if state == "consumed":
        value["execution_id"] = "execution-one"
    if state == "expired":
        value["updated_at"] = "2026-11-09T20:01:00Z"
    with protocol_server(lambda *_: (200, {}, value)) as (client, calls):
        quote = client.get_financial_quote("original-key")
        assert quote.state == state
        assert len(calls) == 1


def test_release_conflict_is_stable_and_never_confirms_release() -> None:
    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        if method == "GET":
            return 200, {}, snapshot()
        return 409, {}, {"error": {"code": "quote_conflict"}}

    with protocol_server(handler) as (client, calls):
        quote = client.get_financial_quote("original-key")
        with pytest.raises(LLMProxyHTTPError) as failure:
            client.release_financial_quote(quote)
        assert failure.value.status_code == 409
        assert failure.value.proxy_error_code == "quote_conflict"
        assert quote.state == "reserved"
        assert len(calls) == 2


def test_frozen_input_detaches_schema_and_rejects_changed_consume_intent() -> None:
    schema: dict[str, Any] = {"type": "object"}
    request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="Fictional facts"),), model="exact-model", structured_output=ClientStructuredOutput(schema=schema), idempotency_key="constructor-key")
    frozen = FinancialQuoteInput.native("a" * 64, "openai", request)
    schema["type"] = "array"
    body = frozen.body()
    assert body["intent"]["request"]["structured_output"]["schema"]["type"] == "object"
    body["intent"]["provider"] = "changed"
    assert frozen.body()["intent"]["provider"] == "openai"
    with protocol_server(lambda *_: (201, {}, snapshot())) as (client, calls):
        quote = client.create_financial_quote("original-key", frozen)
        with pytest.raises(LLMProxyClientError):
            client.create_quoted_execution(quote, intent("different-model"))
        assert len(calls) == 1


def test_unsupported_media_and_timeout_reject_at_constructor() -> None:
    with pytest.raises(LLMProxyClientError):
        FinancialQuoteInput.media("a" * 64, ClientMediaOperationInput(capability="video.generate", provider="openai", model="exact-model", input={}, controls={}))
    request = ClientMessagesRequest(messages=(ClientMessage(role="user", content="Fictional facts"),), model="exact-model", structured_output=ClientStructuredOutput(schema={"type": "object"}), idempotency_key="constructor-key", request_timeout_seconds=30)
    with pytest.raises(LLMProxyClientError):
        FinancialQuoteInput.native("a" * 64, "openai", request)


def test_image_execution_and_restart_read_use_the_same_quote() -> None:
    value = snapshot()
    value["execution_kind"] = "media_operation"
    operation_id = "mop_" + "4" * 32
    operation: dict[str, Any] = {"operation_id": operation_id, "capability": "image.generate", "provider": "openai", "model": "exact-image", "catalog_revision": "catalog-v1", "state": "queued", "cancellation_state": "not_requested", "outputs": [], "cost": {"available": False, "reason": "not_settled"}, "accepted_at": "2026-11-09T19:00:00Z", "updated_at": "2026-11-09T19:00:00Z", "deadline_at": "2026-11-09T20:00:00Z"}

    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        if not path.endswith("/executions"):
            if method == "GET":
                value["state"] = "consumed"
                value["execution_id"] = operation_id
            return 200, {}, value
        assert "Idempotency-Key" not in headers
        return 202 if method == "POST" else 200, execution_headers(), operation

    with protocol_server(handler) as (client, calls):
        frozen = FinancialQuoteInput.media("a" * 64, ClientMediaOperationInput(capability="image.generate", provider="openai", model="exact-image", input={"prompt": "Fictional diagram"}, controls={}))
        quote = client.create_financial_quote("image-quote", frozen)
        result = client.create_quoted_execution(quote, frozen)
        assert result.media is not None and result.media.operation_id == operation_id
        restored = client.get_financial_quote_by_id(quote.id)
        result = client.get_quoted_execution(restored)
        assert result.media is not None and result.media.operation_id == operation_id
        assert len(calls) == 4
        operation["operation_id"] = "mop_" + "5" * 32
        with pytest.raises(LLMProxyTransportError):
            client.get_quoted_execution(restored)


def test_other_processing_and_payload_expiry_never_imply_release() -> None:
    value = snapshot()

    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        if path.endswith("/executions"):
            return 410, {}, {"error": {"code": "hosted_result_expired"}}
        if method == "POST":
            value["processing_digest"] = "d" * 64
        return 200, {}, value

    with protocol_server(handler) as (client, calls):
        with pytest.raises(LLMProxyTransportError):
            client.create_financial_quote("key", intent())
        value["processing_digest"] = "a" * 64
        value["state"] = "consumed"
        value["execution_id"] = "execution-one"
        quote = client.get_financial_quote("key")
        with pytest.raises(LLMProxyHTTPError) as failure:
            client.get_quoted_execution(quote)
        assert failure.value.status_code == 410
        assert failure.value.proxy_error_code == "hosted_result_expired"
        assert quote.state == "consumed"
        assert len(calls) == 3


def test_release_cannot_change_state_at_same_revision() -> None:
    value = snapshot()

    def handler(method: str, path: str, headers: dict[str, str], body: dict[str, Any] | None) -> tuple[int, dict[str, str], Any]:
        if method == "PATCH":
            value["state"] = "released"
        return 200, {}, value

    with protocol_server(handler) as (client, calls):
        quote = client.get_financial_quote("key")
        with pytest.raises(LLMProxyTransportError):
            client.release_financial_quote(quote)
        assert len(calls) == 2
