"""System One public HTTP client contract."""
from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from llm_proxy_client import Client, ClientConfig, LLMProxyHTTPError


@pytest.mark.parametrize("content_type,encoding,raw_body,status,response", [
    ("application/json", "", b'{"model":"jev-1","state":"Evaluate","questions":{"one":{"type":"score","instructions":null,"criteria":["Only level"]}},"unknown":null}', 200, '{"model":"jev-1","answers":{"one":{"type":"score","score":0,"confidence":1,"legend":{"0":"Only level"},"probabilities":{"0":1},"unknown":null}},"usage":{"input_tokens":1,"output_tokens":1,"future":7},"unknown":null}'),
    ("multipart/form-data; boundary=image", "", b'--image\r\nContent-Disposition: form-data; name="true"; filename="a.png"\r\nContent-Type: image/png\r\n\r\nimage-bytes\r\n--image--\r\n', 200, '{"answers":{}}'),
    ("application/json", "gzip", b'\x1f\x8b\x08\x00\x01', 200, '{"answers":{}}'),
    ("application/json", "zstd", b'\x28\xb5\x2f\xfd\x01', 200, '{"answers":{}}'),
    ("application/json", "", b'{"model":"jev-1","questions":[]}', 400, '{"detail":{"message":"provider detail","unknown":null}}'),
])
def test_system_one_preserves_wire_contract(content_type: str, encoding: str, raw_body: bytes, status: int, response: str) -> None:
    received: list[tuple[str, str, dict[str, str], bytes]] = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:
            received.append((self.command, self.path, {name.lower(): value for name, value in self.headers.items()}, self.rfile.read(int(self.headers["Content-Length"]))))
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("X-Request-ID", "upstream-request")
            self.end_headers()
            self.wfile.write(response.encode())

        def log_message(self, format: str, *args: object) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        client = Client(ClientConfig(base_url=f"http://127.0.0.1:{server.server_port}/v2?provider=obsolete&key=old", secret="tenant-key", provider="other"))
        body = json.loads(raw_body) if not encoding and content_type == "application/json" else raw_body
        if status >= 300:
            with pytest.raises(LLMProxyHTTPError) as failure:
                client.post_system_one(body, content_type=content_type, content_encoding=encoding, headers={"TypeSafe-Danger-Zone-Images": "true"})
            assert failure.value.status_code == status
            assert failure.value.body == response
            assert failure.value.headers["x-request-id"] == "upstream-request"
        else:
            result = client.post_system_one(body, content_type=content_type, content_encoding=encoding, headers={"TypeSafe-Danger-Zone-Images": "true"})
            assert result.status_code == status
            assert result.body == response
            assert result.headers["x-request-id"] == "upstream-request"
        method, path, headers, wire = received[0]
        assert method == "POST" and path == "/v1/systemone"
        assert headers["authorization"] == "Bearer tenant-key"
        assert headers["content-type"] == content_type
        assert headers.get("content-encoding", "") == encoding
        assert headers["typesafe-danger-zone-images"] == "true"
        assert "idempotency-key" not in headers
        assert json.loads(wire) == json.loads(raw_body) if not encoding and content_type == "application/json" else wire == raw_body
    finally:
        server.shutdown()
        thread.join()
        server.server_close()
