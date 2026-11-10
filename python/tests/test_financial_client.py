"""Public SDK tests for retained hosted financial evidence."""

from __future__ import annotations

import copy
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Iterator

import pytest

from llm_proxy_client import Client, ClientConfig, LLMProxyClientError, LLMProxyTransportError


def financial_payload() -> dict[str, Any]:
    return {
        "request_id": "request-" + "a" * 32, "reservation_id": "request-" + "a" * 32,
        "execution_id": "execution-one", "execution_kind": "text_request", "request_digest": "b" * 64,
        "pricing_revision": "prices-one", "price_snapshot_digest": "c" * 64, "currency": "USD",
        "maximum_cents": "3", "reservation_state": "settled", "reservation_revision": 1,
        "execution_state": "completed", "state": "settled", "updated_at": "2026-11-09T12:00:00Z",
        "receipt": {"id": "request-" + "a" * 32, "revision": 1,
                    "exact_customer_charge": {"numerator": "12345678901234567890123456789", "denominator": "100000000000000000000000000000"},
                    "posted_cents": "0", "settled_at": "2026-11-09T12:00:00Z"},
    }


@pytest.fixture
def financial_http() -> Iterator[tuple[Client, dict[str, Any]]]:
    control: dict[str, Any] = {"payload": financial_payload(), "calls": 0}

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            control["calls"] += 1
            assert self.headers["Authorization"] == "Bearer application-key"
            assert "?" not in self.path
            assert self.path.startswith("/model/v1/financial-requests")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(control["payload"]).encode())

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        yield Client(ClientConfig(base_url=f"http://127.0.0.1:{server.server_port}", secret="application-key", provider="openai")), control
    finally:
        server.shutdown()
        thread.join()
        server.server_close()


@pytest.mark.parametrize("state", ["settled", "pending", "uncertain", "definitely_undispatched_released"])
def test_financial_client_http(financial_http: tuple[Client, dict[str, Any]], state: str) -> None:
    client, control = financial_http
    control["payload"]["state"] = state
    if state != "settled":
        control["payload"]["receipt"] = None
        control["payload"]["reservation_state"] = {"pending": "held", "uncertain": "reconciliation_required", "definitely_undispatched_released": "released"}[state]
    if state == "definitely_undispatched_released":
        control["payload"]["execution_state"] = "failed"
    result = client.get_financial_request("intent-one")
    assert result.state == state
    assert client.get_financial_request_by_id(result.request_id) == result
    if result.receipt:
        assert result.receipt.exact_customer_charge.numerator == "12345678901234567890123456789"
    assert control["calls"] == 2


@pytest.mark.parametrize("field,bad", [
    ("request_id", "other"), ("reservation_id", "other"), ("execution_id", ""),
    ("execution_kind", "other"), ("request_digest", "bad"), ("pricing_revision", ""),
    ("price_snapshot_digest", "bad"), ("currency", "EUR"), ("maximum_cents", "-1"),
    ("reservation_state", "other"), ("reservation_revision", True), ("reservation_revision", 0),
    ("execution_state", "other"), ("state", "other"), ("updated_at", "bad"), ("updated_at", "2026-99-99T12:00:00Z"),
    ("receipt", None),
])
def test_financial_client_rejects_invalid_evidence(financial_http: tuple[Client, dict[str, Any]], field: str, bad: Any) -> None:
    client, control = financial_http
    control["payload"][field] = bad
    with pytest.raises(LLMProxyTransportError):
        client.get_financial_request("intent-one")


@pytest.mark.parametrize("field,bad", [
    ("id", "other"), ("revision", 2), ("revision", True), ("posted_cents", "-1"),
    ("settled_at", "bad"), ("exact_customer_charge", {"numerator": "1", "denominator": "0"}),
    ("exact_customer_charge", {"numerator": "-1", "denominator": "1"}),
])
def test_financial_client_rejects_invalid_receipt(financial_http: tuple[Client, dict[str, Any]], field: str, bad: Any) -> None:
    client, control = financial_http
    control["payload"]["receipt"][field] = bad
    with pytest.raises(LLMProxyTransportError):
        client.get_financial_request("intent-one")


def test_financial_client_rejects_unknown_and_unsettled_receipts(financial_http: tuple[Client, dict[str, Any]]) -> None:
    client, control = financial_http
    valid = copy.deepcopy(control["payload"])
    for change in [{"extra": True}, {"state": "pending"}, {"receipt": {"extra": True}}]:
        control["payload"] = {**valid, **change}
        with pytest.raises(LLMProxyTransportError):
            client.get_financial_request("intent-one")
    for invalid in ["", "bad/key"]:
        with pytest.raises(LLMProxyClientError):
            client.get_financial_request(invalid)
        with pytest.raises(LLMProxyClientError):
            client.get_financial_request_by_id(invalid)


def test_financial_client_rejects_another_addressed_receipt(financial_http: tuple[Client, dict[str, Any]]) -> None:
    client, _ = financial_http
    with pytest.raises(LLMProxyTransportError):
        client.get_financial_request_by_id("request-" + "d" * 32)
