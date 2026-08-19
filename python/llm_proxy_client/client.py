"""Transport-only client for llm-proxy v2 JSON POST text requests."""

from __future__ import annotations

import base64
import hashlib
import json
import re
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Protocol, Sequence, cast

ACCEPT_HEADER = "Accept"
CONTENT_TYPE_HEADER = "Content-Type"
FORMAT_QUERY_KEY = "format"
FORMAT_QUERY_VALUE_TEXT_PLAIN = "text/plain"
JSON_CONTENT_TYPE = "application/json; charset=utf-8"
KEY_QUERY_KEY = "key"
REQUEST_TIMEOUT_HEADER = "X-LLM-Proxy-Request-Timeout-Seconds"
IDEMPOTENCY_KEY_HEADER = "Idempotency-Key"
ASSET_SHA256_HEADER = "X-LLM-Proxy-Asset-SHA256"
ASSET_ENDPOINT_PATH = "/model/v1/assets"
PROVIDER_QUERY_KEY = "provider"
MODEL_PROFILE_MODEL_KEY = "model"
MODEL_PROFILE_SUBJECT = "model_profile"
MODEL_PROFILE_FIELDS = frozenset({PROVIDER_QUERY_KEY, MODEL_PROFILE_MODEL_KEY})
RETIRED_MODEL_PROFILE_PROVIDERS = frozenset({"qwencloud"})
POST_BODY_QUERY_KEYS = frozenset(
    {
        "messages",
        MODEL_PROFILE_MODEL_KEY,
        "max_output_tokens",
        "max_tokens",
        "reasoning_effort",
        "structured_output",
        "prompt",
        "system_prompt",
        "web_search",
    }
)
MESSAGE_ROLES = frozenset({"system", "user", "assistant"})
IMAGE_MIME_TYPES = frozenset({"image/jpeg", "image/png", "image/webp"})
AUDIO_MIME_TYPES = frozenset({"audio/m4a", "audio/mpeg", "audio/wav"})
MEDIA_MIME_TYPES = IMAGE_MIME_TYPES | AUDIO_MIME_TYPES
ASSET_ID_PATTERN = re.compile(r"^ast_[0-9a-f]{32}$")
SHA256_PATTERN = re.compile(r"^[0-9a-f]{64}$")
IDEMPOTENCY_KEY_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")


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

    def __call__(self, request: urllib.request.Request) -> str:
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

    def asset_upload_url(self) -> str:
        """Return the authenticated tenant asset upload URL for this config."""

        parsed_url = urllib.parse.urlparse(self.base_url.strip())
        query_items = urllib.parse.parse_qsl(parsed_url.query, keep_blank_values=True)
        stripped_query_keys = set(POST_BODY_QUERY_KEYS)
        stripped_query_keys.update({KEY_QUERY_KEY, FORMAT_QUERY_KEY, PROVIDER_QUERY_KEY})
        preserved_items = [
            (query_key, query_value) for query_key, query_value in query_items if query_key not in stripped_query_keys
        ]
        preserved_items.append((KEY_QUERY_KEY, self.secret.strip()))
        return urllib.parse.urlunparse(
            (
                parsed_url.scheme,
                parsed_url.netloc,
                asset_endpoint_path(parsed_url.path or "/"),
                parsed_url.params,
                urllib.parse.urlencode(preserved_items),
                "",
            )
        )

    def _current_model_profile(self) -> _ModelProfile:
        """Read and validate the current model-profile document."""

        model_profile_path = self.model_profile_path.strip()
        model_profile_reader = cast(ModelProfileReader, self.model_profile_reader)
        try:
            model_profile_document = model_profile_reader(model_profile_path)
        except Exception as error:
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
    if provider in RETIRED_MODEL_PROFILE_PROVIDERS:
        raise LLMProxyModelProfileError(
            f"llm_proxy_client_invalid_model_profile: validate {MODEL_PROFILE_SUBJECT} "
            f"path={model_profile_path!r}: provider is retired"
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


def asset_endpoint_path(base_path: str) -> str:
    """Return the tenant asset endpoint for an optional base path prefix."""

    trimmed_path = base_path.strip().rstrip("/")
    if trimmed_path.endswith("/v2"):
        trimmed_path = trimmed_path[: -len("/v2")]
    if not trimmed_path:
        return ASSET_ENDPOINT_PATH
    return f"{trimmed_path}{ASSET_ENDPOINT_PATH}"


@dataclass(frozen=True)
class ClientAttachment:
    """One exact inline or hash-bound tenant media attachment."""

    attachment_type: str
    mime_type: str
    sha256: str
    data: bytes | None = None
    asset_id: str | None = None

    def __post_init__(self) -> None:
        if self.attachment_type not in {"image", "audio"}:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported attachment type")
        supported_mime_types = IMAGE_MIME_TYPES if self.attachment_type == "image" else AUDIO_MIME_TYPES
        if self.mime_type not in supported_mime_types:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported attachment MIME type")
        if (self.data is None) == (self.asset_id is None):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: attachment requires data or asset_id")
        if not SHA256_PATTERN.fullmatch(self.sha256):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid attachment sha256")
        if self.data is not None:
            if not isinstance(self.data, bytes) or not self.data:
                raise LLMProxyClientError("llm_proxy_client_invalid_request: attachment data is empty")
            if hashlib.sha256(self.data).hexdigest() != self.sha256:
                raise LLMProxyClientError("llm_proxy_client_invalid_request: attachment sha256 mismatch")
        if self.asset_id is not None and not ASSET_ID_PATTERN.fullmatch(self.asset_id):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid attachment asset_id")

    def body(self) -> dict[str, str]:
        """Return this attachment as one JSON-ready union variant."""

        payload = {"type": self.attachment_type, "mime_type": self.mime_type, "sha256": self.sha256}
        if self.data is not None:
            payload["data"] = base64.b64encode(self.data).decode("ascii")
        else:
            payload["asset_id"] = cast(str, self.asset_id)
        return payload


def image_attachment(data: bytes, mime_type: str) -> ClientAttachment:
    """Construct one exact inline image attachment."""

    return _inline_attachment("image", data, mime_type, IMAGE_MIME_TYPES)


def audio_attachment(data: bytes, mime_type: str) -> ClientAttachment:
    """Construct one exact inline audio attachment."""

    return _inline_attachment("audio", data, mime_type, AUDIO_MIME_TYPES)


def image_asset_attachment(asset_id: str, mime_type: str, sha256: str) -> ClientAttachment:
    """Construct one hash-bound tenant image asset attachment."""

    return _asset_attachment("image", asset_id, mime_type, sha256, IMAGE_MIME_TYPES)


def audio_asset_attachment(asset_id: str, mime_type: str, sha256: str) -> ClientAttachment:
    """Construct one hash-bound tenant audio asset attachment."""

    return _asset_attachment("audio", asset_id, mime_type, sha256, AUDIO_MIME_TYPES)


def _inline_attachment(
    attachment_type: str, data: bytes, mime_type: str, supported_mime_types: frozenset[str]
) -> ClientAttachment:
    normalized_mime_type = mime_type.strip().lower()
    if normalized_mime_type not in supported_mime_types:
        raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported attachment MIME type")
    if not isinstance(data, bytes) or not data:
        raise LLMProxyClientError("llm_proxy_client_invalid_request: attachment data is empty")
    return ClientAttachment(
        attachment_type=attachment_type,
        mime_type=normalized_mime_type,
        sha256=hashlib.sha256(data).hexdigest(),
        data=data,
    )


def _asset_attachment(
    attachment_type: str,
    asset_id: str,
    mime_type: str,
    sha256: str,
    supported_mime_types: frozenset[str],
) -> ClientAttachment:
    normalized_mime_type = mime_type.strip().lower()
    if normalized_mime_type not in supported_mime_types:
        raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported attachment MIME type")
    return ClientAttachment(
        attachment_type=attachment_type,
        mime_type=normalized_mime_type,
        sha256=sha256,
        asset_id=asset_id,
    )


@dataclass(frozen=True)
class ClientMessage:
    """Validated chat message; order is optional but all-or-none within one request."""

    role: str
    content: str
    order: int | None = None
    attachments: Sequence[ClientAttachment] = ()

    def __post_init__(self) -> None:
        if self.role.strip().lower() not in MESSAGE_ROLES:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported message role")
        if self.content == "":
            raise LLMProxyClientError("llm_proxy_client_invalid_request: empty message content")
        if self.order is not None and self.order < 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: message order must be non-negative")
        if self.attachments and self.role.strip().lower() != "user":
            raise LLMProxyClientError("llm_proxy_client_invalid_request: attachments require user role")
        if any(not isinstance(attachment, ClientAttachment) for attachment in self.attachments):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid attachment")

    def body(self) -> dict[str, Any]:
        """Return this message as a JSON-ready body item."""

        payload: dict[str, Any] = {"role": self.role.strip().lower(), "content": self.content}
        if self.order is not None:
            payload["order"] = self.order
        if self.attachments:
            payload["attachments"] = [attachment.body() for attachment in self.attachments]
        return payload


@dataclass(frozen=True)
class ClientStructuredOutput:
    """One caller JSON Schema and its durable request identity."""

    schema: dict[str, Any]
    idempotency_key: str

    def __post_init__(self) -> None:
        if not isinstance(self.schema, dict):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: structured output schema must be an object")
        try:
            json.dumps(self.schema, ensure_ascii=False, allow_nan=False)
        except (TypeError, ValueError) as error:
            raise LLMProxyClientError(
                "llm_proxy_client_invalid_request: structured output schema must be valid JSON"
            ) from error
        if not IDEMPOTENCY_KEY_PATTERN.fullmatch(self.idempotency_key):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid idempotency key")

    def body(self) -> dict[str, Any]:
        """Return the canonical structured-output body object."""

        return {"schema": self.schema}


@dataclass(frozen=True)
class ClientMessagesRequest:
    """Validated v2 messages-only JSON POST request."""

    messages: Sequence[ClientMessage]
    model: str = ""
    web_search: bool = False
    max_tokens: int | None = None
    reasoning_effort: str | None = None
    structured_output: ClientStructuredOutput | None = None
    request_timeout_seconds: int | None = None

    def __post_init__(self) -> None:
        if len(self.messages) == 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: missing messages")
        validate_messages(self.messages)
        if not isinstance(self.web_search, bool):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: web_search must be a boolean")
        if self.max_tokens is not None and self.max_tokens <= 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: max_tokens must be positive")
        if self.reasoning_effort is not None and not self.reasoning_effort.strip():
            raise LLMProxyClientError("llm_proxy_client_invalid_request: reasoning_effort must be nonblank")
        if self.structured_output is not None and not isinstance(self.structured_output, ClientStructuredOutput):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid structured output")
        if self.structured_output is not None and self.web_search:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: structured output conflicts with web_search")
        if self.request_timeout_seconds is not None and (
            isinstance(self.request_timeout_seconds, bool)
            or not isinstance(self.request_timeout_seconds, int)
            or self.request_timeout_seconds <= 0
        ):
            raise LLMProxyClientError(
                "llm_proxy_client_invalid_request: request_timeout_seconds must be a positive whole number"
            )

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
        if self.reasoning_effort is not None:
            payload["reasoning_effort"] = self.reasoning_effort
        if self.structured_output is not None:
            payload["structured_output"] = self.structured_output.body()
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
class ClientAsset:
    """One hash-bound tenant asset returned by llm-proxy."""

    asset_id: str
    mime_type: str
    size_bytes: int
    sha256: str
    state: str
    created_at: str
    expires_at: str


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
                request.request_timeout_seconds,
                request.structured_output.idempotency_key if request.structured_output is not None else "",
            )
        return self._post_json(
            request.body(),
            self.config.messages_post_url(),
            request.request_timeout_seconds,
            request.structured_output.idempotency_key if request.structured_output is not None else "",
        )

    def upload_asset(self, data: bytes, mime_type: str) -> ClientAsset:
        """Upload exact tenant media bytes and return their hash-bound asset record."""

        normalized_mime_type = mime_type.strip().lower()
        if normalized_mime_type not in MEDIA_MIME_TYPES:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported asset MIME type")
        if not isinstance(data, bytes) or not data:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: asset data is empty")
        digest = hashlib.sha256(data).hexdigest()
        prepared_request = urllib.request.Request(
            self.config.asset_upload_url(),
            data=data,
            headers={CONTENT_TYPE_HEADER: normalized_mime_type, ASSET_SHA256_HEADER: digest},
            method="POST",
        )
        opener = self.opener or default_response_opener
        try:
            response_text = opener(prepared_request)
        except urllib.error.HTTPError as error:
            body = error.read().decode("utf-8", errors="replace")
            raise LLMProxyHTTPError(error.code, body, str(error.reason), "operation=asset_upload") from error
        except (urllib.error.URLError, TimeoutError, OSError) as error:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: operation=asset_upload") from error
        if not isinstance(response_text, str) or len(response_text.encode("utf-8")) > 64 * 1024:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response")
        try:
            response = json.loads(response_text)
        except (json.JSONDecodeError, TypeError) as error:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response") from error
        required_fields = {
            "asset_id",
            "mime_type",
            "size_bytes",
            "sha256",
            "state",
            "created_at",
            "expires_at",
        }
        if not isinstance(response, dict) or set(response) != required_fields:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response")
        if (
            not isinstance(response["asset_id"], str)
            or not ASSET_ID_PATTERN.fullmatch(response["asset_id"])
            or response["mime_type"] != normalized_mime_type
            or isinstance(response["size_bytes"], bool)
            or response["size_bytes"] != len(data)
            or response["sha256"] != digest
            or response["state"] != "available"
        ):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response")
        created_at = _asset_timestamp(response["created_at"])
        expires_at = _asset_timestamp(response["expires_at"])
        if expires_at <= created_at:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response")
        return ClientAsset(
            asset_id=response["asset_id"],
            mime_type=response["mime_type"],
            size_bytes=response["size_bytes"],
            sha256=response["sha256"],
            state=response["state"],
            created_at=response["created_at"],
            expires_at=response["expires_at"],
        )

    def _post_json(
        self,
        request_payload: dict[str, Any],
        request_url: str,
        request_timeout_seconds: int | None,
        idempotency_key: str,
    ) -> str:
        """Send a JSON POST request payload and return the response text."""

        request_body = json.dumps(request_payload, ensure_ascii=False).encode("utf-8")
        request_headers = {
            ACCEPT_HEADER: FORMAT_QUERY_VALUE_TEXT_PLAIN,
            CONTENT_TYPE_HEADER: JSON_CONTENT_TYPE,
        }
        if request_timeout_seconds is not None:
            request_headers[REQUEST_TIMEOUT_HEADER] = str(request_timeout_seconds)
        if idempotency_key:
            request_headers[IDEMPOTENCY_KEY_HEADER] = idempotency_key
        prepared_request = urllib.request.Request(
            request_url,
            data=request_body,
            headers=request_headers,
            method="POST",
        )
        opener = self.opener or default_response_opener
        failure_context = request_failure_context(request_payload, request_url, request_timeout_seconds)
        try:
            return opener(prepared_request)
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


def _asset_timestamp(value: Any) -> datetime:
    """Parse one timezone-aware RFC 3339 asset timestamp."""

    if not isinstance(value, str) or not value.endswith("Z"):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response")
    try:
        timestamp = datetime.fromisoformat(value.removesuffix("Z") + "+00:00")
    except ValueError as error:
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response") from error
    if timestamp.tzinfo is None or timestamp.utcoffset() != timezone.utc.utcoffset(timestamp):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid asset response")
    return timestamp


def request_failure_context(
    request_payload: dict[str, Any], request_url: str, request_timeout_seconds: int | None
) -> str:
    """Return non-secret request context for HTTP and transport failures."""

    parsed_url = urllib.parse.urlparse(request_url)
    query_values = urllib.parse.parse_qs(parsed_url.query)
    provider = first_query_value(query_values, PROVIDER_QUERY_KEY, "omitted")
    model_value = request_payload.get("model")
    model = model_value if isinstance(model_value, str) and model_value.strip() else "omitted"
    timeout_value = request_timeout_seconds if request_timeout_seconds is not None else "omitted"
    return f"provider={provider} model={model} request_timeout_seconds={timeout_value}"


def first_query_value(query_values: dict[str, list[str]], key: str, default: str) -> str:
    """Return the first non-empty query value for a key."""

    values = query_values.get(key, [])
    if not values:
        return default
    value = values[0].strip()
    if not value:
        return default
    return value


def default_response_opener(request: urllib.request.Request) -> str:
    """Execute a prepared urllib request and return decoded text."""

    with urllib.request.urlopen(request) as response:
        response_body = cast(bytes, response.read())
        return response_body.decode("utf-8")
