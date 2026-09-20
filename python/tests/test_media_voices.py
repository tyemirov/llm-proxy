"""Public voice pagination and metadata through the HTTP client."""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

import pytest

from llm_proxy_client import Client, ClientConfig, ClientMediaVoiceQuery, LLMProxyClientError, LLMProxyTransportError


VOICE_ID = "voi_0123456789abcdef0123456789abcdef"
VOICE = {
    "voice_id": VOICE_ID, "provider": "elevenlabs", "mode": "preset", "language": None,
    "display_name": "Reader", "default": False, "sample_rates": [], "default_sample_rate": None,
    "description": "Narrator", "category": "cloned", "labels": {"accent": "american"},
    "high_quality_base_model_ids": ["eleven_v3"],
    "verified_languages": [{"language": "en", "model_id": "eleven_v3", "accent": "american", "locale": "en-US", "preview": f"/model/v1/voices/{VOICE_ID}/previews/1"}],
    "preview": f"/model/v1/voices/{VOICE_ID}/previews/0",
}
PAGE = {"voices": [VOICE], "has_more": True, "next_cursor": "opaque", "total_count": 2}


def test_voice_query_preserves_source_fields_and_page() -> None:
    queries = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            queries.append(parse_qs(urlsplit(self.path).query))
            assert self.headers["Authorization"] == "Bearer secret"
            body = json.dumps(PAGE).encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *_args: object) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        client = Client(ClientConfig(base_url=f"http://127.0.0.1:{server.server_port}", secret="secret"))
        page = client.get_media_voices(ClientMediaVoiceQuery(provider="elevenlabs", search="reader", page_size=1, sort="name", sort_direction="asc", voice_type="personal", category="cloned", include_total_count=True))
        voice = page.voices[0]
        assert voice.language is None and voice.default_sample_rate is None and voice.sample_rates == ()
        assert voice.labels == {"accent": "american"} and voice.high_quality_base_model_ids == ("eleven_v3",)
        assert voice.verified_languages[0].locale == "en-US"
        assert voice.preview == f"/model/v1/voices/{VOICE_ID}/previews/0"
        assert page.has_more and page.total_count == 2 and page.next_cursor == "opaque"
        client.get_media_voices(ClientMediaVoiceQuery(provider="elevenlabs", cursor=page.next_cursor))
        assert queries == [{"provider": ["elevenlabs"], "search": ["reader"], "page_size": ["1"], "sort": ["name"], "sort_direction": ["asc"], "voice_type": ["personal"], "category": ["cloned"], "include_total_count": ["true"]}, {"provider": ["elevenlabs"], "cursor": ["opaque"]}]
    finally:
        server.shutdown()
        server.server_close()
        worker.join()


@pytest.mark.parametrize("arguments", [{"provider": "xai", "search": "\ud800"}, {"provider": " XAI "}, {"provider": "xai", "page_size": 0}, {"provider": "xai", "page_size": True}, {"provider": "xai", "cursor": "opaque", "search": "new"}, {"provider": "xai", "sort": "unknown"}, {"provider": "xai", "include_total_count": 1}])
def test_voice_query_rejects_invalid_inputs(arguments: dict) -> None:
    with pytest.raises(LLMProxyClientError):
        ClientMediaVoiceQuery(**arguments)


@pytest.mark.parametrize("changes", [{"voices": None}, {"has_more": None}, {"total_count": True}, {"total_count": -1}, {"next_cursor": None}, {"next_cursor": " "}, {"voices": [VOICE, VOICE]}, {"voices": [{**VOICE, "provider": "other"}]}, {"voices": [{**VOICE, "preview": "https://native.invalid"}]}, {"voices": [{**VOICE, "verified_languages": [{"language": "en"}]}]}, {"voices": [{**VOICE, "default_sample_rate": 24000}]}, {"voices": [{**VOICE, "labels": []}]}])
def test_voice_client_rejects_malformed_pages(changes: dict) -> None:
    client = Client(ClientConfig(base_url="https://gateway.example", secret="secret"), opener=lambda *_args, **_kwargs: json.dumps({**PAGE, **changes}))
    with pytest.raises(LLMProxyTransportError):
        client.get_media_voices(ClientMediaVoiceQuery(provider="elevenlabs"))
