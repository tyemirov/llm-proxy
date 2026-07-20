"""Transport-only client for llm-proxy v2 JSON POST text requests."""

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
MODEL_PROFILE_MODEL_KEY = "model"
MODEL_PROFILE_SUBJECT = "model_profile"
MODEL_PROFILE_FIELDS = frozenset({PROVIDER_QUERY_KEY, MODEL_PROFILE_MODEL_KEY})
POST_BODY_QUERY_KEYS = frozenset(
    {
        "messages",
        MODEL_PROFILE_MODEL_KEY,
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


class LLMProxyModelProfileError(LLMProxyClientError):
    """Raised when a configured JSON model-profile document is invalid."""


class LLMProxyHTTPError(RuntimeError):
    """Raised when llm-proxy returns a non-success HTTP status."""

    def __init__(self, status_code: int, body: str, reason: str, request_context: str) -> None:
        super().__init__(
            f"llm_proxy_client_http_failure: status={status_code} reason={reason} "
            f"{request_context} body={body!r}"
        )
        self.status_code = status_code
        self.body = body
        self.reason = reason
        self.request_context = request_context


class LLMProxyTransportError(RuntimeError):
    """Raised when the HTTP transport cannot complete the request."""


class ResponseOpener(Protocol):
    """Callable that executes a prepared urllib request."""

    def __call__(self, request: urllib.request.Request, timeout: float) -> str:
        """Return decoded response text for the prepared request."""


class ModelProfileReader(Protocol):
    """Callable that reads a current JSON model-profile document."""

    def __call__(self, path: str) -> str:
        """Return the model-profile document at the configured path."""


@dataclass(frozen=True)
class _JSONModelProfileObject:
    pairs: tuple[tuple[str, Any], ...]


@dataclass(frozen=True)
class _ModelProfile:
    provider: str
    model: str


@dataclass(frozen=True)
class ClientConfig:
    """Validated llm-proxy client configuration."""

    base_url: str
    secret: str
    provider: str = ""
    timeout_seconds: float = 390.0
    model_profile_path: str = ""
    model_profile_reader: ModelProfileReader | None = None

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

        model_profile_path = self.model_profile_path.strip()
        if not model_profile_path and self.model_profile_reader is not None:
            raise LLMProxyClientError(
                "llm_proxy_client_invalid_config: model_profile_reader requires model_profile_path"
            )
        if model_profile_path:
            if self.model_profile_reader is None:
                raise LLMProxyClientError(
                    "llm_proxy_client_invalid_config: model_profile_path requires model_profile_reader"
                )
            if not callable(self.model_profile_reader):
                raise LLMProxyClientError(
                    "llm_proxy_client_invalid_config: model_profile_reader must be callable"
                )
            if self.provider.strip():
                raise LLMProxyClientError("llm_proxy_client_invalid_config: model_profile_path conflicts with provider")
            query_keys = {query_key for query_key, _ in urllib.parse.parse_qsl(parsed_url.query, keep_blank_values=True)}
            if PROVIDER_QUERY_KEY in query_keys:
                raise LLMProxyClientError(
                    "llm_proxy_client_invalid_config: model_profile_path conflicts with base_url provider query"
                )
            if MODEL_PROFILE_MODEL_KEY in query_keys:
                raise LLMProxyClientError(
                    "llm_proxy_client_invalid_config: model_profile_path conflicts with base_url model query"
                )

    def messages_post_url(self) -> str:
        """Return the authenticated v2 JSON POST URL for this config."""

        provider = self.provider.strip()
        if self.model_profile_path.strip():
            provider = self._current_model_profile().provider
        return self._messages_post_url_for_provider(provider)

    def _current_model_profile(self) -> _ModelProfile:
        """Read and validate the current model-profile document."""

        model_profile_path = self.model_profile_path.strip()
        model_profile_reader = cast(ModelProfileReader, self.model_profile_reader)
        try:
            model_profile_document = model_profile_reader(model_profile_path)
        except (OSError, UnicodeError) as error:
            raise LLMProxyModelProfileError(
                f"llm_proxy_client_invalid_model_profile: read {MODEL_PROFILE_SUBJECT} "
                f"path={model_profile_path!r}: {error}"
            ) from error
        return _decode_model_profile(model_profile_path, model_profile_document)

    def _messages_post_url_for_provider(self, provider: str) -> str:
        """Return the authenticated v2 JSON POST URL for one validated provider override."""

        parsed_url = urllib.parse.urlparse(self.base_url.strip())
        request_path = parsed_url.path or "/"
        request_path = v2_endpoint_path(request_path)
        query_items = urllib.parse.parse_qsl(parsed_url.query, keep_blank_values=True)
        stripped_query_keys = set(POST_BODY_QUERY_KEYS)
        stripped_query_keys.update({KEY_QUERY_KEY, FORMAT_QUERY_KEY})
        if provider.strip():
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
        if provider.strip():
            preserved_items.append((PROVIDER_QUERY_KEY, provider.strip()))
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


def _decode_model_profile(model_profile_path: str, model_profile_document: str) -> _ModelProfile:
    """Decode one exact provider/model JSON document from a configured path."""

    if not isinstance(model_profile_document, str):
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: read {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: reader must return text"
        )
    try:
        decoded_document = json.loads(model_profile_document, object_pairs_hook=_json_model_profile_object)
    except json.JSONDecodeError as error:
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: decode {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: {error}"
        ) from error
    if not isinstance(decoded_document, _JSONModelProfileObject):
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: document must be an object"
        )

    profile_values: dict[str, str] = {}
    for profile_field, profile_value in decoded_document.pairs:
        if profile_field not in MODEL_PROFILE_FIELDS:
            raise LLMProxyModelProfileError(
                f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
                f"path={model_profile_path!r}: unsupported field={profile_field!r}"
            )
        if profile_field in profile_values:
            raise LLMProxyModelProfileError(
                f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
                f"path={model_profile_path!r}: duplicate field={profile_field!r}"
            )
        if not isinstance(profile_value, str):
            raise LLMProxyModelProfileError(
                f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
                f"path={model_profile_path!r}: field={profile_field!r} must be a string"
            )
        profile_values[profile_field] = profile_value
    try:
        provider = profile_values[PROVIDER_QUERY_KEY].strip()
        model = profile_values[MODEL_PROFILE_MODEL_KEY].strip()
    except KeyError as error:
        missing_field = str(error).strip("'")
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: missing {missing_field}"
        ) from error
    if not provider:
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: missing provider"
        )
    if not model:
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: missing model"
        )
    return _ModelProfile(provider=provider, model=model)


def _json_model_profile_object(profile_pairs: list[tuple[str, Any]]) -> _JSONModelProfileObject:
    """Preserve profile object field ordering and duplicate keys while decoding JSON."""

    return _JSONModelProfileObject(pairs=tuple(profile_pairs))


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

        return self._body_with_model(self.model.strip())

    def _body_with_model(self, model: str) -> dict[str, Any]:
        """Return the JSON body payload with one resolved model value."""

        payload: dict[str, Any] = {
            "messages": [message.body() for message in ordered_messages(self.messages)],
            "web_search": self.web_search,
        }
        if model:
            payload[MODEL_PROFILE_MODEL_KEY] = model
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
    """HTTP client for llm-proxy v2 JSON POST text requests."""

    config: ClientConfig
    opener: ResponseOpener | None = None

    def post_messages(self, request: ClientMessagesRequest) -> str:
        """Send a v2 messages-only JSON POST request and return the response text."""

        if self.config.model_profile_path.strip():
            if request.model.strip():
                raise LLMProxyModelProfileError(
                    f"llm_proxy_client_invalid_model_profile: request model conflicts with "
                    f"{MODEL_PROFILE_SUBJECT} path={self.config.model_profile_path.strip()!r}"
                )
            model_profile = self.config._current_model_profile()
            return self._post_json(
                request._body_with_model(model_profile.model),
                self.config._messages_post_url_for_provider(model_profile.provider),
            )
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
        failure_context = request_failure_context(request_payload, request_url, self.config.timeout_seconds)
        try:
            return opener(prepared_request, self.config.timeout_seconds)
        except urllib.error.HTTPError as error:
            body = error.read().decode("utf-8", errors="replace")
            raise LLMProxyHTTPError(error.code, body, str(error.reason), failure_context) from error
        except urllib.error.URLError as error:
            raise LLMProxyTransportError(
                f"llm_proxy_client_transport_failure: {failure_context} reason={error.reason}"
            ) from error
        except TimeoutError as error:
            raise LLMProxyTransportError(
                f"llm_proxy_client_transport_failure: {failure_context} reason={error}"
            ) from error
        except OSError as error:
            raise LLMProxyTransportError(
                f"llm_proxy_client_transport_failure: {failure_context} reason={error}"
            ) from error


def request_failure_context(request_payload: dict[str, Any], request_url: str, timeout_seconds: float) -> str:
    """Return non-secret request context for HTTP and transport failures."""

    parsed_url = urllib.parse.urlparse(request_url)
    query_values = urllib.parse.parse_qs(parsed_url.query)
    provider = first_query_value(query_values, PROVIDER_QUERY_KEY, "omitted")
    model_value = request_payload.get("model")
    model = model_value if isinstance(model_value, str) and model_value.strip() else "omitted"
    return f"provider={provider} model={model} timeout_seconds={timeout_seconds:g}"


def first_query_value(query_values: dict[str, list[str]], key: str, default: str) -> str:
    """Return the first non-empty query value for a key."""

    values = query_values.get(key, [])
    if not values:
        return default
    value = values[0].strip()
    if not value:
        return default
    return value


def default_response_opener(request: urllib.request.Request, timeout: float) -> str:
    """Execute a prepared urllib request and return decoded text."""

    with urllib.request.urlopen(request, timeout=timeout) as response:
        response_body = cast(bytes, response.read())
        return response_body.decode("utf-8")
