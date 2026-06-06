"""Transport-only client for llm-proxy JSON POST text requests."""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Protocol, cast

ACCEPT_HEADER = "Accept"
CONTENT_TYPE_HEADER = "Content-Type"
FORMAT_QUERY_KEY = "format"
FORMAT_QUERY_VALUE_TEXT_PLAIN = "text/plain"
JSON_CONTENT_TYPE = "application/json; charset=utf-8"
KEY_QUERY_KEY = "key"
PROVIDER_QUERY_KEY = "provider"
POST_BODY_QUERY_KEYS = frozenset(
    {
        "messages",
        "model",
        "max_output_tokens",
        "max_tokens",
        "prompt",
        "system_prompt",
        "web_search",
    }
)


class LLMProxyClientError(ValueError):
    """Raised when llm-proxy client config or request input is invalid."""


class LLMProxyHTTPError(RuntimeError):
    """Raised when llm-proxy returns a non-success HTTP status."""

    def __init__(self, status_code: int, body: str, reason: str) -> None:
        super().__init__(f"llm_proxy_client_http_failure: status={status_code} reason={reason} body={body!r}")
        self.status_code = status_code
        self.body = body
        self.reason = reason


class LLMProxyTransportError(RuntimeError):
    """Raised when the HTTP transport cannot complete the request."""


class ResponseOpener(Protocol):
    """Callable that executes a prepared urllib request."""

    def __call__(self, request: urllib.request.Request, timeout: float) -> str:
        """Return decoded response text for the prepared request."""


@dataclass(frozen=True)
class ClientConfig:
    """Validated llm-proxy client configuration."""

    base_url: str
    secret: str
    provider: str = ""
    timeout_seconds: float = 120.0

    def __post_init__(self) -> None:
        if not self.base_url.strip():
            raise LLMProxyClientError("llm_proxy_client_invalid_config: missing base_url")
        parsed_url = urllib.parse.urlparse(self.base_url.strip())
        if parsed_url.scheme not in {"http", "https"}:
            raise LLMProxyClientError("llm_proxy_client_invalid_config: base_url must use http or https")
        if not parsed_url.netloc:
            raise LLMProxyClientError("llm_proxy_client_invalid_config: base_url must include host")
        if not self.secret.strip():
            raise LLMProxyClientError("llm_proxy_client_invalid_config: missing secret")
        if self.timeout_seconds <= 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_config: timeout_seconds must be positive")

    def post_url(self) -> str:
        """Return the authenticated JSON POST URL for this config."""

        parsed_url = urllib.parse.urlparse(self.base_url.strip())
        query_items = urllib.parse.parse_qsl(parsed_url.query, keep_blank_values=True)
        stripped_query_keys = set(POST_BODY_QUERY_KEYS)
        stripped_query_keys.update({KEY_QUERY_KEY, FORMAT_QUERY_KEY})
        if self.provider.strip():
            stripped_query_keys.add(PROVIDER_QUERY_KEY)
        preserved_items = [
            (query_key, query_value) for query_key, query_value in query_items if query_key not in stripped_query_keys
        ]
        preserved_items.extend(
            [
                (KEY_QUERY_KEY, self.secret.strip()),
                (FORMAT_QUERY_KEY, FORMAT_QUERY_VALUE_TEXT_PLAIN),
            ]
        )
        if self.provider.strip():
            preserved_items.append((PROVIDER_QUERY_KEY, self.provider.strip()))
        return urllib.parse.urlunparse(
            (
                parsed_url.scheme,
                parsed_url.netloc,
                parsed_url.path or "/",
                parsed_url.params,
                urllib.parse.urlencode(preserved_items),
                "",
            )
        )


@dataclass(frozen=True)
class ClientRequest:
    """Validated llm-proxy JSON POST request."""

    prompt: str
    model: str = ""
    web_search: bool = False
    system_prompt: str = ""
    max_tokens: int | None = None

    def __post_init__(self) -> None:
        if self.prompt == "":
            raise LLMProxyClientError("llm_proxy_client_invalid_request: missing prompt")
        if self.max_tokens is not None and self.max_tokens <= 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: max_tokens must be positive")

    def body(self) -> dict[str, Any]:
        """Return the JSON body payload for this request."""

        payload: dict[str, Any] = {
            "prompt": self.prompt,
            "web_search": self.web_search,
        }
        if self.model.strip():
            payload["model"] = self.model.strip()
        if self.system_prompt.strip():
            payload["system_prompt"] = self.system_prompt.strip()
        if self.max_tokens is not None:
            payload["max_tokens"] = self.max_tokens
        return payload


@dataclass(frozen=True)
class Client:
    """HTTP client for llm-proxy JSON POST text requests."""

    config: ClientConfig
    opener: ResponseOpener | None = None

    def post(self, request: ClientRequest) -> str:
        """Send a JSON POST request and return the response text."""

        request_body = json.dumps(request.body(), ensure_ascii=False).encode("utf-8")
        prepared_request = urllib.request.Request(
            self.config.post_url(),
            data=request_body,
            headers={
                ACCEPT_HEADER: FORMAT_QUERY_VALUE_TEXT_PLAIN,
                CONTENT_TYPE_HEADER: JSON_CONTENT_TYPE,
            },
            method="POST",
        )
        opener = self.opener or default_response_opener
        try:
            return opener(prepared_request, self.config.timeout_seconds)
        except urllib.error.HTTPError as error:
            body = error.read().decode("utf-8", errors="replace")
            raise LLMProxyHTTPError(error.code, body, str(error.reason)) from error
        except urllib.error.URLError as error:
            raise LLMProxyTransportError(f"llm_proxy_client_transport_failure: {error.reason}") from error
        except TimeoutError as error:
            raise LLMProxyTransportError(f"llm_proxy_client_transport_failure: {error}") from error


def default_response_opener(request: urllib.request.Request, timeout: float) -> str:
    """Execute a prepared urllib request and return decoded text."""

    with urllib.request.urlopen(request, timeout=timeout) as response:
        response_body = cast(bytes, response.read())
        return response_body.decode("utf-8")
