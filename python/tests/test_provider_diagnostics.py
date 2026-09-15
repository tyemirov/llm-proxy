"""Official diagnostic client behavior through a real HTTP listener."""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

import pytest

from llm_proxy_client import Client, ClientConfig, ClientProviderDiagnostics, LLMProxyClientError, LLMProxyHTTPError, LLMProxyTransportError


def test_provider_diagnostics_reads_tenant_metrics_and_rejects_invalid_responses() -> None:
    valid: dict[str, Any] = {
        "provider": "dictator", "scope": "tenant", "operations_total": 6,
        "queued": 1, "running": 1, "succeeded": 1, "failed": 1, "cancelled": 1, "uncertain": 1,
    }
    response = dict(valid)
    status = 200
    requests: list[tuple[str, str | None]] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            requests.append((self.path, self.headers.get("Authorization")))
            body = json.dumps(response).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        client = Client(ClientConfig(base_url=f"http://127.0.0.1:{server.server_port}/v2?key=obsolete", secret="tenant-secret"))
        result = client.get_provider_diagnostics("dictator")
        assert isinstance(result, ClientProviderDiagnostics)
        assert result == ClientProviderDiagnostics(**valid)
        assert requests == [("/model/v1/provider-diagnostics/dictator", "Bearer tenant-secret")]
        for provider in ("", "Dictator", " dictator", "dictator/other", "dictator?key=secret"):
            with pytest.raises(LLMProxyClientError):
                client.get_provider_diagnostics(provider)
        assert len(requests) == 1
        for field in valid:
            response = dict(valid)
            del response[field]
            with pytest.raises(LLMProxyTransportError):
                client.get_provider_diagnostics("dictator")
            response[field] = None
            with pytest.raises(LLMProxyTransportError):
                client.get_provider_diagnostics("dictator")
        for field, value in (
            ("provider", "other"), ("scope", "server"), ("operations_total", 7),
            *((field, value) for field in ("operations_total", "queued", "running", "succeeded", "failed", "cancelled", "uncertain") for value in (-1, 1.5, True, "1", 2**63)),
        ):
            response = dict(valid)
            response[field] = value
            with pytest.raises(LLMProxyTransportError):
                client.get_provider_diagnostics("dictator")
        response = {**valid, "native_handle": "private"}
        with pytest.raises(LLMProxyTransportError):
            client.get_provider_diagnostics("dictator")
        status = 403
        with pytest.raises(LLMProxyHTTPError) as failure:
            client.get_provider_diagnostics("dictator")
        assert failure.value.status_code == 403
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
