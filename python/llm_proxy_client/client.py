"""Transport-only client for llm-proxy JSON POST text requests."""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Protocol, Sequence, cast

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
MESSAGE_ROLES = frozenset({"system", "user", "assistant"})


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

        return self._post_url("")

    def messages_post_url(self) -> str:
        """Return the authenticated v2 JSON POST URL for this config."""

        return self._post_url("v2")

    def _post_url(self, api_version: str) -> str:
        parsed_url = urllib.parse.urlparse(self.base_url.strip())
        request_path = parsed_url.path or "/"
        if api_version == "v2":
            request_path = v2_endpoint_path(request_path)
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
                request_path,
                parsed_url.params,
                urllib.parse.urlencode(preserved_items),
                "",
            )
        )


def v2_endpoint_path(base_path: str) -> str:
    """Return the v2 endpoint path for an optional base path prefix."""

    trimmed_path = base_path.strip().rstrip("/")
    if not trimmed_path:
        return "/v2"
    if trimmed_path == "/v2" or trimmed_path.endswith("/v2"):
        return trimmed_path
    return f"{trimmed_path}/v2"


@dataclass(frozen=True)
class ClientMessage:
    """Validated chat message; order is optional but all-or-none within one request."""

    role: str
    content: str
    order: int | None = None

    def __post_init__(self) -> None:
        if self.role.strip().lower() not in MESSAGE_ROLES:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported message role")
        if self.content == "":
            raise LLMProxyClientError("llm_proxy_client_invalid_request: empty message content")
        if self.order is not None and self.order < 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: message order must be non-negative")

    def body(self) -> dict[str, str | int]:
        """Return this message as a JSON-ready body item."""

        payload: dict[str, str | int] = {"role": self.role.strip().lower(), "content": self.content}
        if self.order is not None:
            payload["order"] = self.order
        return payload


@dataclass(frozen=True)
class ClientRequest:
    """Validated llm-proxy JSON POST request."""

    prompt: str = ""
    model: str = ""
    web_search: bool = False
    system_prompt: str = ""
    max_tokens: int | None = None
    messages: Sequence[ClientMessage] = ()

    def __post_init__(self) -> None:
        has_prompt = self.prompt != ""
        has_messages = len(self.messages) > 0
        if has_prompt and has_messages:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: choose prompt or messages")
        if not has_prompt and not has_messages:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: missing prompt")
        if has_messages:
            validate_messages(self.messages)
        if self.system_prompt.strip() and any(message.role.strip().lower() == "system" for message in self.messages):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: system_prompt conflicts with system message")
        if self.max_tokens is not None and self.max_tokens <= 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: max_tokens must be positive")

    def body(self) -> dict[str, Any]:
        """Return the JSON body payload for this request."""

        payload: dict[str, Any] = {
            "web_search": self.web_search,
        }
        if self.messages:
            payload["messages"] = [message.body() for message in self.ordered_messages()]
        else:
            payload["prompt"] = self.prompt
        if self.model.strip():
            payload["model"] = self.model.strip()
        if self.system_prompt.strip():
            payload["system_prompt"] = self.system_prompt.strip()
        if self.max_tokens is not None:
            payload["max_tokens"] = self.max_tokens
        return payload

    def ordered_messages(self) -> Sequence[ClientMessage]:
        """Return messages sorted by explicit order when provided."""

        return ordered_messages(self.messages)


@dataclass(frozen=True)
class ClientMessagesRequest:
    """Validated v2 messages-only JSON POST request."""

    messages: Sequence[ClientMessage]
    model: str = ""
    web_search: bool = False
    max_tokens: int | None = None

    def __post_init__(self) -> None:
        if len(self.messages) == 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: missing messages")
        validate_messages(self.messages)
        if self.max_tokens is not None and self.max_tokens <= 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: max_tokens must be positive")

    def body(self) -> dict[str, Any]:
        """Return the JSON body payload for this v2 request."""

        payload: dict[str, Any] = {
            "messages": [message.body() for message in ordered_messages(self.messages)],
            "web_search": self.web_search,
        }
        if self.model.strip():
            payload["model"] = self.model.strip()
        if self.max_tokens is not None:
            payload["max_tokens"] = self.max_tokens
        return payload


def validate_messages(messages: Sequence[ClientMessage]) -> None:
    """Validate shared message invariants."""

    if not any(message.role.strip().lower() == "user" for message in messages):
        raise LLMProxyClientError("llm_proxy_client_invalid_request: messages must include a user message")
    messages_with_order = [message for message in messages if message.order is not None]
    if messages_with_order and len(messages_with_order) != len(messages):
        raise LLMProxyClientError("llm_proxy_client_invalid_request: all messages must include order when order is used")
    order_values = [message.order for message in messages_with_order]
    if len(order_values) != len(set(order_values)):
        raise LLMProxyClientError("llm_proxy_client_invalid_request: duplicate message order")


def ordered_messages(messages: Sequence[ClientMessage]) -> Sequence[ClientMessage]:
    """Return messages sorted by explicit order when provided."""

    if any(message.order is not None for message in messages):
        return tuple(sorted(messages, key=lambda message: cast(int, message.order)))
    return messages


@dataclass(frozen=True)
class Client:
    """HTTP client for llm-proxy JSON POST text requests."""

    config: ClientConfig
    opener: ResponseOpener | None = None

    def post(self, request: ClientRequest) -> str:
        """Send a JSON POST request and return the response text."""

        return self._post_json(request.body(), self.config.post_url())

    def post_messages(self, request: ClientMessagesRequest) -> str:
        """Send a v2 messages-only JSON POST request and return the response text."""

        return self._post_json(request.body(), self.config.messages_post_url())

    def _post_json(self, request_payload: dict[str, Any], request_url: str) -> str:
        """Send a JSON POST request payload and return the response text."""

        request_body = json.dumps(request_payload, ensure_ascii=False).encode("utf-8")
        prepared_request = urllib.request.Request(
            request_url,
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
