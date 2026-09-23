"""Transport-only client for llm-proxy v2 JSON POST text requests."""

from __future__ import annotations

import base64
import json
import re
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timezone
from types import MappingProxyType
from typing import Any, Mapping, Protocol, Sequence, cast

ACCEPT_HEADER = "Accept"
CONTENT_TYPE_HEADER = "Content-Type"
FORMAT_QUERY_KEY = "format"
FORMAT_QUERY_VALUE_TEXT_PLAIN = "text/plain"
JSON_CONTENT_TYPE = "application/json; charset=utf-8"
KEY_QUERY_KEY = "key"
REQUEST_TIMEOUT_HEADER = "X-LLM-Proxy-Request-Timeout-Seconds"
IDEMPOTENCY_KEY_HEADER = "Idempotency-Key"
TEXT_REQUEST_STATE_HEADER = "X-LLM-Proxy-Structured-Request-State"
ASSET_ENDPOINT_PATH = "/model/v1/assets"
MEDIA_CAPABILITIES_ENDPOINT_PATH = "/model/v1/capabilities"
MEDIA_OPERATIONS_ENDPOINT_PATH = "/model/v1/operations"
MEDIA_VOICES_ENDPOINT_PATH = "/model/v1/voices"
PROVIDER_DIAGNOSTICS_ENDPOINT_PATH = "/model/v1/provider-diagnostics"
PROVIDER_RESOURCES_ENDPOINT_PATH = "/model/v1/provider-resources"
DIAGNOSTIC_PROVIDER_PATTERN = re.compile(r"^[a-z][a-z0-9_-]*$")
DIAGNOSTIC_COUNTER_FIELDS = ("operations_total", "queued", "running", "succeeded", "failed", "cancelled", "uncertain")
PROVIDER_QUERY_KEY = "provider"
MODEL_PROFILE_MODEL_KEY = "model"
MODEL_PROFILE_SUBJECT = "model_profile"
MODEL_PROFILE_FIELDS = frozenset({PROVIDER_QUERY_KEY, MODEL_PROFILE_MODEL_KEY})
RETIRED_MODEL_PROFILE_PROVIDERS = frozenset({"qwencloud"})
POST_BODY_QUERY_KEYS = frozenset(
    {
        "messages",
        "tools",
        "tool_choice",
        "parallel_tool_calls",
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
MESSAGE_ROLES = frozenset({"system", "user", "assistant", "tool"})
IMAGE_MIME_TYPES = frozenset({"image/jpeg", "image/png", "image/webp"})
AUDIO_MIME_TYPES = frozenset({"audio/m4a", "audio/mpeg", "audio/wav"})
ASSET_MIME_TYPES = IMAGE_MIME_TYPES | AUDIO_MIME_TYPES | frozenset({
    "audio/flac", "audio/ogg", "video/mp4", "video/webm", "application/json", "application/x-subrip", "application/octet-stream",
})
ASSET_ID_PATTERN = re.compile(r"^ast_[0-9a-f]{32}$")
MEDIA_OPERATION_ID_PATTERN = re.compile(r"^mop_[0-9a-f]{32}$")
MEDIA_VOICE_ID_PATTERN = re.compile(r"^voi_[0-9a-f]{32}$")
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
        self.proxy_error_code = _proxy_error_code(body)


class LLMProxyTransportError(RuntimeError):
    """Raised when the HTTP transport cannot complete the request."""


@dataclass(frozen=True)
class ClientHTTPResponse:
    """One HTTP response with its status, decoded body, and immutable headers."""

    status_code: int
    body: str
    headers: Mapping[str, str] = field(default_factory=dict)

    def __post_init__(self) -> None:
        if type(self.status_code) is not int or not 100 <= self.status_code <= 599 or not isinstance(self.body, str):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid HTTP response")
        if not isinstance(self.headers, Mapping) or any(not isinstance(key, str) or not isinstance(value, str) for key, value in self.headers.items()):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid HTTP response headers")
        object.__setattr__(self, "headers", MappingProxyType({key.lower(): value for key, value in self.headers.items()}))


@dataclass(frozen=True)
class ClientTextRequestResult:
    """One saved result or pending execution receipt from the text request resource."""

    state: str
    proxy_request_id: str = ""
    started_at: str = ""
    updated_at: str = ""
    elapsed_seconds: int = 0
    output: str = ""

    def __post_init__(self) -> None:
        if not isinstance(self.state, str) or self.state not in {"not_dispatched", "dispatched", "succeeded"}:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid text request state")
        if self.state == "succeeded":
            try:
                json.loads(self.output)
            except (json.JSONDecodeError, TypeError) as error:
                raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid text request result") from error
        elif any(not isinstance(value, str) or not value.strip() for value in (self.proxy_request_id, self.started_at, self.updated_at)) or type(self.elapsed_seconds) is not int or self.elapsed_seconds < 0:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid text request receipt")


class LLMProxyRequestPendingError(RuntimeError):
    """Raised when a text request is accepted and has no terminal result yet."""

    def __init__(self, snapshot: ClientTextRequestResult) -> None:
        self.snapshot = snapshot
        super().__init__(f"llm_proxy_client_request_pending: state={snapshot.state} request_id={snapshot.proxy_request_id}")


class ResponseOpener(Protocol):
    """Callable that executes a prepared urllib request."""

    def __call__(self, request: urllib.request.Request, *, timeout: float | None = None) -> ClientHTTPResponse:
        """Return status, decoded body, and headers for the prepared request."""


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

    def media_resource_url(self, resource_path: str, query: dict[str, str] | None = None) -> str:
        """Return one authenticated media-resource URL without legacy query credentials."""

        parsed_url = urllib.parse.urlparse(self.base_url.strip())
        base_path = parsed_url.path.strip().rstrip("/")
        if base_path.endswith("/v2"):
            base_path = base_path[: -len("/v2")]
        query_values = urllib.parse.urlencode(query or {})
        return urllib.parse.urlunparse(
            (parsed_url.scheme, parsed_url.netloc, f"{base_path}{resource_path}", "", query_values, "")
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
    """One exact inline or tenant media attachment."""

    attachment_type: str
    mime_type: str
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
        if self.data is not None:
            if not isinstance(self.data, bytes) or not self.data:
                raise LLMProxyClientError("llm_proxy_client_invalid_request: attachment data is empty")
        if self.asset_id is not None and not ASSET_ID_PATTERN.fullmatch(self.asset_id):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid attachment asset_id")

    def body(self) -> dict[str, str]:
        """Return this attachment as one JSON-ready union variant."""

        payload = {"type": self.attachment_type, "mime_type": self.mime_type}
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


def image_asset_attachment(asset_id: str, mime_type: str) -> ClientAttachment:
    """Construct one tenant image asset attachment."""

    return _asset_attachment("image", asset_id, mime_type, IMAGE_MIME_TYPES)


def audio_asset_attachment(asset_id: str, mime_type: str) -> ClientAttachment:
    """Construct one tenant audio asset attachment."""

    return _asset_attachment("audio", asset_id, mime_type, AUDIO_MIME_TYPES)


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
        data=data,
    )


def _asset_attachment(
    attachment_type: str,
    asset_id: str,
    mime_type: str,
    supported_mime_types: frozenset[str],
) -> ClientAttachment:
    normalized_mime_type = mime_type.strip().lower()
    if normalized_mime_type not in supported_mime_types:
        raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported attachment MIME type")
    return ClientAttachment(
        attachment_type=attachment_type,
        mime_type=normalized_mime_type,
        asset_id=asset_id,
    )


FUNCTION_NAME_PATTERN = re.compile(r"^[A-Za-z0-9_-]{1,64}$")


@dataclass(frozen=True)
class ClientFunction:
    """A caller function declaration."""

    name: str
    parameters: dict[str, Any]
    description: str = ""
    strict: bool | None = None

    def __post_init__(self) -> None:
        if not FUNCTION_NAME_PATTERN.fullmatch(self.name) or self.parameters.get("type") != "object":
            raise LLMProxyClientError("invalid function declaration")
        json.dumps(self.parameters, allow_nan=False)

    def body(self) -> dict[str, Any]:
        """Return the native function declaration."""
        value: dict[str, Any] = {"name": self.name, "parameters": self.parameters, "description": self.description}
        if self.strict is not None:
            value["strict"] = self.strict
        return value


@dataclass(frozen=True)
class ClientFunctionCall:
    """An exact function call from an assistant turn."""

    id: str
    name: str
    arguments: str

    def __post_init__(self) -> None:
        if not self.id or not FUNCTION_NAME_PATTERN.fullmatch(self.name):
            raise LLMProxyClientError("invalid function call")
        try:
            arguments = json.loads(self.arguments)
        except ValueError as error:
            raise LLMProxyClientError("invalid function arguments") from error
        if not isinstance(arguments, dict):
            raise LLMProxyClientError("function arguments must be an object")

    def body(self) -> dict[str, str]:
        """Return the exact native call fields."""
        return {"id": self.id, "name": self.name, "arguments": self.arguments}


@dataclass(frozen=True)
class ClientToolChoice:
    """A selection mode and an optional function name."""

    mode: str
    name: str = ""

    def __post_init__(self) -> None:
        if self.mode not in {"auto", "none", "required", "function"}:
            raise LLMProxyClientError("invalid tool choice")
        if (self.mode == "function") != bool(self.name):
            raise LLMProxyClientError("invalid selected function name")

    def body(self) -> dict[str, str]:
        """Return the native selection fields."""
        return {"mode": self.mode, "name": self.name} if self.name else {"mode": self.mode}


@dataclass(frozen=True)
class ClientMessage:
    """Validated chat message; order is optional but all-or-none within one request."""

    role: str
    content: str
    order: int | None = None
    attachments: Sequence[ClientAttachment] = ()
    tool_calls: Sequence[ClientFunctionCall] = ()
    tool_call_id: str = ""

    def __post_init__(self) -> None:
        role = self.role.strip().lower()
        if role not in MESSAGE_ROLES:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported message role")
        if self.content == "" and not self.tool_calls and role != "tool":
            raise LLMProxyClientError("llm_proxy_client_invalid_request: empty message content")
        if self.order is not None and self.order < 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: message order must be non-negative")
        if self.attachments and role != "user":
            raise LLMProxyClientError("llm_proxy_client_invalid_request: attachments require user role")
        if any(not isinstance(attachment, ClientAttachment) for attachment in self.attachments):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid attachment")

    def body(self) -> dict[str, Any]:
        """Return this message as a JSON-ready body item."""

        payload: dict[str, Any] = {"role": self.role.strip().lower(), "content": self.content}
        if self.tool_calls:
            payload["tool_calls"] = [call.body() for call in self.tool_calls]
        if self.tool_call_id:
            payload["tool_call_id"] = self.tool_call_id
        if self.order is not None:
            payload["order"] = self.order
        if self.attachments:
            payload["attachments"] = [attachment.body() for attachment in self.attachments]
        return payload


@dataclass(frozen=True)
class ClientStructuredOutput:
    """One caller JSON Schema for provider-enforced output."""

    schema: dict[str, Any]

    def __post_init__(self) -> None:
        if not isinstance(self.schema, dict):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: structured output schema must be an object")
        try:
            json.dumps(self.schema, ensure_ascii=False, allow_nan=False)
        except (TypeError, ValueError) as error:
            raise LLMProxyClientError(
                "llm_proxy_client_invalid_request: structured output schema must be valid JSON"
            ) from error

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
    idempotency_key: str = ""
    request_timeout_seconds: int | None = None
    tools: Sequence[ClientFunction] = ()
    tool_choice: ClientToolChoice | None = None
    parallel_tool_calls: bool | None = None

    def __post_init__(self) -> None:
        if len(self.messages) == 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: missing messages")
        validate_messages(self.messages)
        names = {tool.name for tool in self.tools}
        if len(names) != len(self.tools):
            raise LLMProxyClientError("duplicate function declaration")
        if not self.tools and (self.tool_choice is not None or self.parallel_tool_calls is not None):
            raise LLMProxyClientError("tools required")
        if self.tool_choice is not None and self.tool_choice.mode == "function" and self.tool_choice.name not in names:
            raise LLMProxyClientError("unknown selected function")
        if self.parallel_tool_calls is not None and not isinstance(self.parallel_tool_calls, bool):
            raise LLMProxyClientError("parallel_tool_calls must be a boolean")
        if not isinstance(self.web_search, bool):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: web_search must be a boolean")
        if self.max_tokens is not None and self.max_tokens <= 0:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: max_tokens must be positive")
        if self.reasoning_effort is not None and not self.reasoning_effort.strip():
            raise LLMProxyClientError("llm_proxy_client_invalid_request: reasoning_effort must be nonblank")
        if self.structured_output is not None and not isinstance(self.structured_output, ClientStructuredOutput):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid structured output")
        if not isinstance(self.idempotency_key, str) or (
            self.idempotency_key and not IDEMPOTENCY_KEY_PATTERN.fullmatch(self.idempotency_key)
        ) or (self.structured_output is not None and not self.idempotency_key):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid or missing idempotency key")
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
        if self.tools:
            payload["tools"] = [tool.body() for tool in self.tools]
        if self.tool_choice is not None:
            payload["tool_choice"] = self.tool_choice.body()
        if self.parallel_tool_calls is not None:
            payload["parallel_tool_calls"] = self.parallel_tool_calls
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

    pending: set[str] = set()
    seen: set[str] = set()
    for message in ordered_messages(messages):
        role = message.role.strip().lower()
        if role == "tool":
            if message.tool_call_id not in pending or message.tool_calls:
                raise LLMProxyClientError("unmatched tool result")
            pending.remove(message.tool_call_id)
            continue
        if pending or message.tool_call_id:
            raise LLMProxyClientError("missing tool result")
        if message.tool_calls and role != "assistant":
            raise LLMProxyClientError("function calls require assistant role")
        for call in message.tool_calls:
            if call.id in seen:
                raise LLMProxyClientError("duplicate function call identifier")
            seen.add(call.id)
            pending.add(call.id)
    if pending:
        raise LLMProxyClientError("missing tool result")


def ordered_messages(messages: Sequence[ClientMessage]) -> Sequence[ClientMessage]:
    """Return messages sorted by explicit order when provided."""

    if any(message.order is not None for message in messages):
        return tuple(sorted(messages, key=lambda message: cast(int, message.order)))
    return messages


@dataclass(frozen=True)
class ClientAsset:
    """One tenant asset returned by llm-proxy."""

    asset_id: str
    mime_type: str
    size_bytes: int
    state: str
    created_at: str
    expires_at: str


@dataclass(frozen=True)
class ClientMediaOperationInput:
    """One complete durable media operation intent."""

    capability: str
    provider: str
    model: str | None
    input: dict[str, Any]
    controls: dict[str, Any]

    def __post_init__(self) -> None:
        if not self.capability.strip() or not self.provider.strip() or (self.model is not None and (not isinstance(self.model, str) or not self.model.strip())):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: incomplete media operation")
        if self.provider != self.provider.strip().lower():
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation provider")
        if not isinstance(self.input, dict) or not isinstance(self.controls, dict):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: media input and controls must be objects")
        try:
            json.dumps(self.body(), ensure_ascii=False, allow_nan=False)
        except (TypeError, ValueError) as error:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation JSON") from error

    def body(self) -> dict[str, Any]:
        """Return the canonical media operation request object."""

        return {
            "capability": self.capability.strip(),
            "provider": self.provider,
            **({"model": self.model.strip()} if self.model is not None else {}),
            "input": self.input,
            "controls": self.controls,
        }


@dataclass(frozen=True)
class ClientAspectRatioImageInput:
    """Image generation controls defined by the selected catalog route."""

    provider: str
    model: str
    prompt: str
    aspect_ratio: str
    output_format: str
    output_count: int

    def operation(self) -> ClientMediaOperationInput:
        """Build the canonical media request for image generation."""

        return ClientMediaOperationInput(
            capability="image.generate",
            provider=self.provider,
            model=self.model,
            input={"prompt": self.prompt},
            controls={
                "aspect_ratio": self.aspect_ratio,
                "output_format": self.output_format,
                "output_count": self.output_count,
            },
        )


@dataclass(frozen=True)
class ClientMediaOperationOutput:
    """One ordered result asset."""

    asset_id: str
    mime_type: str
    size_bytes: int
    ordinal: int


@dataclass(frozen=True)
class ClientMediaOperationPartialOutput:
    """One verified preview available before operation completion."""

    asset_id: str
    mime_type: str
    size_bytes: int
    output_ordinal: int
    partial_ordinal: int


@dataclass(frozen=True)
class ClientMediaOperation:
    """One durable tenant media operation."""

    operation_id: str
    capability: str
    provider: str
    model: str | None
    catalog_revision: str
    state: str
    cancellation_state: str
    outputs: tuple[ClientMediaOperationOutput, ...]
    partial_outputs: tuple[ClientMediaOperationPartialOutput, ...]
    previous_operation_id: str | None
    error_code: str | None
    cost_available: bool
    cost_reason: str | None
    accepted_at: str
    updated_at: str
    deadline_at: str


@dataclass(frozen=True)
class ClientMediaCapabilityRoute:
    """One tenant-available durable media route."""

    capability: str
    provider: str
    model: str
    controls: tuple[dict[str, Any], ...]
    limits: tuple[dict[str, Any], ...]


@dataclass(frozen=True)
class ClientMediaCapabilityService:
    """One tenant-available provider service without a model."""

    capability: str
    provider: str
    controls: tuple[dict[str, Any], ...]
    limits: tuple[dict[str, Any], ...]


PROVIDER_RESOURCE_KINDS = frozenset({"voices", "voice_library", "history", "pronunciation_dictionaries", "metadata", "quotas", "elements"})


@dataclass(frozen=True)
class ClientProviderResource:
    """One account resource available through a provider connection."""

    provider: str
    kind: str


@dataclass(frozen=True)
class ClientMediaCapabilities:
    """Tenant-available durable media routes and their catalog revision."""

    catalog_revision: str
    routes: tuple[ClientMediaCapabilityRoute, ...]
    resources: tuple[ClientProviderResource, ...]
    services: tuple[ClientMediaCapabilityService, ...]


@dataclass(frozen=True)
class ClientMediaVoiceQuery:
    """Select one provider voice page or continue an opaque cursor."""

    provider: str
    cursor: str = ""
    search: str = ""
    page_size: int | None = None
    sort: str = ""
    sort_direction: str = ""
    voice_type: str = ""
    category: str = ""
    include_total_count: bool | None = None

    def __post_init__(self) -> None:
        strings = (self.provider, self.cursor, self.search, self.sort, self.sort_direction, self.voice_type, self.category)
        if (
            any(not isinstance(value, str) for value in strings)
            or not self.provider or self.provider != self.provider.strip().lower()
            or any(value and not value.strip() for value in strings)
            or len(self.cursor) > 16384 or len(self.search) > 1000
            or any(0xD800 <= ord(character) <= 0xDFFF for character in self.search)
            or self.page_size is not None and (type(self.page_size) is not int or not 1 <= self.page_size <= 100)
            or self.include_total_count is not None and type(self.include_total_count) is not bool
            or self.sort not in {"", "name", "created_at_unix"}
            or self.sort_direction not in {"", "asc", "desc"}
            or self.voice_type not in {"", "personal", "community", "default", "account", "non-default", "non-community", "saved"}
            or self.category not in {"", "premade", "cloned", "generated", "professional"}
            or self.cursor and (any(strings[2:]) or self.page_size is not None or self.include_total_count is not None)
        ):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media voice query")

    def _query(self) -> dict[str, str]:
        values = {"provider": self.provider}
        for name in ("cursor", "search", "sort", "sort_direction", "voice_type", "category"):
            value = getattr(self, name)
            if value:
                values[name] = value
        if self.page_size is not None:
            values["page_size"] = str(self.page_size)
        if self.include_total_count is not None:
            values["include_total_count"] = str(self.include_total_count).lower()
        return values


@dataclass(frozen=True)
class ClientMediaVoiceLanguage:
    """One verified language observation and local preview link."""

    language: str
    model_id: str
    accent: str | None
    locale: str | None
    preview: str | None


@dataclass(frozen=True)
class ClientMediaVoice:
    """One tenant-owned public synthesis voice."""

    voice_id: str
    provider: str
    mode: str
    language: str | None
    display_name: str
    default: bool
    sample_rates: tuple[int, ...]
    default_sample_rate: int | None


    description: str | None
    category: str | None
    labels: dict[str, str]
    high_quality_base_model_ids: tuple[str, ...]
    verified_languages: tuple[ClientMediaVoiceLanguage, ...]
    preview: str | None


@dataclass(frozen=True)
class ClientMediaVoicePage:
    """One voice page with opaque continuation and optional total count."""

    voices: tuple[ClientMediaVoice, ...]
    has_more: bool
    total_count: int | None
    next_cursor: str | None


@dataclass(frozen=True)
class ClientProviderDiagnostics:
    """Retained operation counts for the authenticated tenant and provider."""

    provider: str
    scope: str
    operations_total: int
    queued: int
    running: int
    succeeded: int
    failed: int
    cancelled: int
    uncertain: int


@dataclass(frozen=True)
class ClientProviderModelMetadata:
    """An upstream observation that does not create a catalog offering."""

    model_id: str
    name: str
    can_do_text_to_speech: bool | None
    can_do_voice_conversion: bool | None
    maximum_text_length_per_request: int | None
    max_characters_request_free_user: int | None
    max_characters_request_subscribed_user: int | None


@dataclass(frozen=True)
class ClientProviderMetadata:
    """Account-visible model metadata."""

    provider: str
    models: tuple[ClientProviderModelMetadata, ...]


@dataclass(frozen=True)
class ClientProviderCreditExtension:
    """Unlimited usage or a bounded credit count."""

    unlimited: bool
    value: int | None


@dataclass(frozen=True)
class ClientProviderOverage:
    """The provider's decimal monetary observation."""

    amount: str
    currency: str


@dataclass(frozen=True)
class ClientProviderSubscription:
    """Current account quota, independent of catalog prices."""

    tier: str
    status: str
    character_count: int
    character_limit: int
    credit_extension: ClientProviderCreditExtension
    can_extend_credit_limit: bool
    current_overage: ClientProviderOverage | None
    has_open_invoices: bool
    currency: str | None
    next_character_count_reset_unix: int | None


@dataclass(frozen=True)
class ClientProviderQuotas:
    """One provider's current subscription observation."""

    provider: str
    subscription: ClientProviderSubscription


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
                request.idempotency_key,
            )
        return self._post_json(
            request.body(),
            self.config.messages_post_url(),
            request.request_timeout_seconds,
            request.idempotency_key,
        )

    def get_text_request(self, idempotency_key: str) -> ClientTextRequestResult:
        """Read a hosted or structured text request without provider dispatch."""

        if not isinstance(idempotency_key, str) or not IDEMPOTENCY_KEY_PATTERN.fullmatch(idempotency_key):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid idempotency key")
        source = urllib.parse.urlsplit(self.config.messages_post_url())
        query = urllib.parse.parse_qs(source.query, keep_blank_values=True)
        query.pop(PROVIDER_QUERY_KEY, None)
        query.pop(FORMAT_QUERY_KEY, None)
        address = urllib.parse.urlunsplit((source.scheme, source.netloc, source.path.rstrip("/") + "/requests", urllib.parse.urlencode(query, doseq=True), ""))
        request = urllib.request.Request(address, headers={ACCEPT_HEADER: "application/json", IDEMPOTENCY_KEY_HEADER: idempotency_key}, method="GET")
        response = _open_response(self.opener or default_response_opener, request, "operation=text_request")
        if response.status_code == 202:
            return _decode_text_request_pending(response.body)
        if response.status_code != 200 or response.headers.get(TEXT_REQUEST_STATE_HEADER.lower()) != "succeeded":
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid text request result")
        return ClientTextRequestResult(state="succeeded", output=response.body)

    def upload_asset(self, data: bytes, mime_type: str) -> ClientAsset:
        """Upload exact tenant media bytes and return their asset record."""

        normalized_mime_type = mime_type.strip().lower()
        if normalized_mime_type not in ASSET_MIME_TYPES:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: unsupported asset MIME type")
        if not isinstance(data, bytes) or not data:
            raise LLMProxyClientError("llm_proxy_client_invalid_request: asset data is empty")
        prepared_request = urllib.request.Request(
            self.config.asset_upload_url(),
            data=data,
            headers={CONTENT_TYPE_HEADER: normalized_mime_type, "Authorization": f"Bearer {self.config.secret.strip()}"},
            method="POST",
        )
        response_text = _open_response(self.opener or default_response_opener, prepared_request, "operation=asset_upload").body
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
            state=response["state"],
            created_at=response["created_at"],
            expires_at=response["expires_at"],
        )

    def create_media_operation(
        self, idempotency_key: str, operation: ClientMediaOperationInput
    ) -> ClientMediaOperation:
        """Accept one durable media operation."""

        if not IDEMPOTENCY_KEY_PATTERN.fullmatch(idempotency_key):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid idempotency key")
        if not isinstance(operation, ClientMediaOperationInput):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation")
        response = self._media_json_request(
            "POST",
            MEDIA_OPERATIONS_ENDPOINT_PATH,
            operation.body(),
            {IDEMPOTENCY_KEY_HEADER: idempotency_key},
        )
        return _decode_media_operation(response)

    def get_media_operation(self, operation_id: str) -> ClientMediaOperation:
        """Read one tenant-owned durable media operation."""

        if not MEDIA_OPERATION_ID_PATTERN.fullmatch(operation_id):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation identifier")
        response = self._media_json_request("GET", f"{MEDIA_OPERATIONS_ENDPOINT_PATH}/{operation_id}")
        return _decode_media_operation(response)

    def cancel_media_operation(self, operation_id: str) -> ClientMediaOperation:
        """Request cancellation and return the observed operation state."""

        if not MEDIA_OPERATION_ID_PATTERN.fullmatch(operation_id):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation identifier")
        response = self._media_json_request(
            "PUT", f"{MEDIA_OPERATIONS_ENDPOINT_PATH}/{operation_id}/cancellation"
        )
        return _decode_media_operation(response)

    def wait_media_operation(
        self, operation_id: str, poll_interval_seconds: float, timeout_seconds: float
    ) -> ClientMediaOperation:
        """Poll one durable media operation until it reaches a terminal state."""

        if not MEDIA_OPERATION_ID_PATTERN.fullmatch(operation_id):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation identifier")
        if (
            isinstance(poll_interval_seconds, bool)
            or not isinstance(poll_interval_seconds, (int, float))
            or poll_interval_seconds <= 0
            or isinstance(timeout_seconds, bool)
            or not isinstance(timeout_seconds, (int, float))
            or timeout_seconds <= 0
        ):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media operation wait budget")
        deadline = time.monotonic() + timeout_seconds
        while True:
            remaining_seconds = deadline - time.monotonic()
            if remaining_seconds <= 0:
                raise LLMProxyTransportError(
                    "llm_proxy_client_transport_failure: media operation wait timed out"
                )
            operation = _decode_media_operation(self._media_json_request(
                "GET", f"{MEDIA_OPERATIONS_ENDPOINT_PATH}/{operation_id}", timeout_seconds=remaining_seconds
            ))
            if operation.state in {"succeeded", "failed", "cancelled", "uncertain"}:
                return operation
            remaining_seconds = deadline - time.monotonic()
            if remaining_seconds <= 0:
                raise LLMProxyTransportError(
                    "llm_proxy_client_transport_failure: media operation wait timed out"
                )
            time.sleep(min(poll_interval_seconds, remaining_seconds))

    def get_media_capabilities(self) -> ClientMediaCapabilities:
        """Read durable media routes available to the authenticated tenant."""

        response = self._media_json_request("GET", MEDIA_CAPABILITIES_ENDPOINT_PATH)
        return _decode_media_capabilities(response)

    def get_media_voices(self, query: ClientMediaVoiceQuery) -> ClientMediaVoicePage:
        """Read one tenant-owned voice page with its continuation cursor."""

        response = self._media_json_request("GET", MEDIA_VOICES_ENDPOINT_PATH, query=query._query())
        if (
            set(response) != {"voices", "has_more", "total_count", "next_cursor"}
            or not isinstance(response["voices"], list)
            or type(response["has_more"]) is not bool
            or response["total_count"] is not None and (type(response["total_count"]) is not int or response["total_count"] < 0)
            or response["has_more"] != (response["next_cursor"] is not None)
            or response["next_cursor"] is not None and (not isinstance(response["next_cursor"], str) or not response["next_cursor"].strip())
        ):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media voice collection")
        voices = tuple(_decode_media_voice(voice) for voice in response["voices"])
        if len({voice.voice_id for voice in voices}) != len(voices) or any(voice.provider != query.provider for voice in voices):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media voice collection")
        return ClientMediaVoicePage(voices, response["has_more"], response["total_count"], response["next_cursor"])

    def get_provider_metadata(self, provider: str) -> ClientProviderMetadata:
        """Read model observations without adding executable catalog routes."""

        if not DIAGNOSTIC_PROVIDER_PATTERN.fullmatch(provider):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid resource provider")
        response = self._media_json_request("GET", f"{PROVIDER_RESOURCES_ENDPOINT_PATH}/{provider}/metadata")
        if set(response) != {"provider", "models"} or response["provider"] != provider or not isinstance(response["models"], list):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider metadata")
        models = tuple(_decode_provider_model(model) for model in response["models"])
        if len({model.model_id for model in models}) != len(models):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: duplicate provider model")
        return ClientProviderMetadata(provider, models)

    def get_provider_quotas(self, provider: str) -> ClientProviderQuotas:
        """Read account quota separately from published catalog prices."""

        if not DIAGNOSTIC_PROVIDER_PATTERN.fullmatch(provider):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid resource provider")
        response = self._media_json_request("GET", f"{PROVIDER_RESOURCES_ENDPOINT_PATH}/{provider}/quotas")
        if set(response) != {"provider", "subscription"} or response["provider"] != provider:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider quotas")
        return ClientProviderQuotas(provider, _decode_provider_subscription(response["subscription"]))

    def get_media_voice(self, voice_id: str) -> ClientMediaVoice:
        """Read one tenant-owned media voice."""

        if not MEDIA_VOICE_ID_PATTERN.fullmatch(voice_id):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid media voice identifier")
        response = self._media_json_request("GET", f"{MEDIA_VOICES_ENDPOINT_PATH}/{voice_id}")
        return _decode_media_voice(response)

    def get_provider_diagnostics(self, provider: str) -> ClientProviderDiagnostics:
        """Read retained tenant operation counts without contacting the provider."""

        if not DIAGNOSTIC_PROVIDER_PATTERN.fullmatch(provider):
            raise LLMProxyClientError("llm_proxy_client_invalid_request: invalid diagnostic provider")
        response = self._media_json_request("GET", f"{PROVIDER_DIAGNOSTICS_ENDPOINT_PATH}/{provider}")
        required = {"provider", "scope", *DIAGNOSTIC_COUNTER_FIELDS}
        if (
            set(response) != required
            or response["provider"] != provider
            or response["scope"] != "tenant"
            or any(type(response[field]) is not int or response[field] < 0 or response[field] > 2**63 - 1 for field in DIAGNOSTIC_COUNTER_FIELDS)
        ):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider diagnostics response")
        if response["operations_total"] != sum(response[field] for field in DIAGNOSTIC_COUNTER_FIELDS if field != "operations_total"):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: inconsistent provider operation counts")
        return ClientProviderDiagnostics(**response)

    def _media_json_request(
        self,
        method: str,
        resource_path: str,
        body: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
        query: dict[str, str] | None = None,
        timeout_seconds: float | None = None,
    ) -> dict[str, Any]:
        """Execute one strict tenant media-resource request."""

        request_headers = {ACCEPT_HEADER: "application/json", "Authorization": f"Bearer {self.config.secret.strip()}"}
        request_headers.update(headers or {})
        request_data = None
        if body is not None:
            request_data = json.dumps(body, ensure_ascii=False, allow_nan=False).encode("utf-8")
            request_headers[CONTENT_TYPE_HEADER] = "application/json"
        prepared_request = urllib.request.Request(
            self.config.media_resource_url(resource_path, query),
            data=request_data,
            headers=request_headers,
            method=method,
        )
        response_text = _open_response(self.opener or default_response_opener, prepared_request, f"operation=media_resource method={method}", timeout_seconds).body
        if not isinstance(response_text, str) or len(response_text.encode("utf-8")) > 8 * 1024 * 1024:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media response")
        try:
            response = json.loads(response_text)
        except (json.JSONDecodeError, TypeError) as error:
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media response") from error
        if not isinstance(response, dict):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media response")
        return response

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
        failure_context = request_failure_context(request_payload, request_url, request_timeout_seconds)
        response = _open_response(self.opener or default_response_opener, prepared_request, failure_context)
        if response.status_code == 202:
            raise LLMProxyRequestPendingError(_decode_text_request_pending(response.body))
        return response.body


def _decode_media_operation(response: dict[str, Any]) -> ClientMediaOperation:
    """Decode one exact durable media operation response."""

    required_fields = {
        "operation_id",
        "capability",
        "provider",
        "catalog_revision",
        "state",
        "cancellation_state",
        "outputs",
        "cost",
        "accepted_at",
        "updated_at",
        "deadline_at",
    }
    if not required_fields <= response.keys() or response.keys() - required_fields - {"error", "partial_outputs", "previous_operation_id", "model"}:
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    string_fields = ("capability", "provider", "catalog_revision")
    if (
        not isinstance(response["operation_id"], str)
        or not MEDIA_OPERATION_ID_PATTERN.fullmatch(response["operation_id"])
        or any(not isinstance(response[field], str) or not response[field] for field in string_fields)
        or ("model" in response and (not isinstance(response["model"], str) or not response["model"].strip()))
        or not isinstance(response["state"], str)
        or response["state"] not in {"queued", "running", "succeeded", "failed", "cancelled", "uncertain"}
        or not isinstance(response["cancellation_state"], str)
        or response["cancellation_state"] not in {"not_requested", "requested", "confirmed", "unsupported"}
        or not isinstance(response["outputs"], list)
        or not isinstance(response["cost"], dict)
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    outputs = tuple(_decode_media_operation_output(output) for output in response["outputs"])
    previous_operation_id = response.get("previous_operation_id")
    if "previous_operation_id" in response and (
        not isinstance(previous_operation_id, str) or not MEDIA_OPERATION_ID_PATTERN.fullmatch(previous_operation_id)
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation parent")
    partial_values = response.get("partial_outputs", [])
    if not isinstance(partial_values, list):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation previews")
    partial_outputs = tuple(_decode_media_operation_partial(output) for output in partial_values)
    positions = [(output.output_ordinal, output.partial_ordinal) for output in partial_outputs]
    if any(current <= previous for previous, current in zip(positions, positions[1:])):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation preview order")
    cost = response["cost"]
    if set(cost) not in ({"available"}, {"available", "reason"}) or not isinstance(cost["available"], bool):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    cost_reason = cost.get("reason")
    if cost_reason is not None and (not isinstance(cost_reason, str) or not cost_reason):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    error_code = None
    if "error" in response:
        error_value = response["error"]
        if not isinstance(error_value, dict) or set(error_value) != {"code"} or not isinstance(error_value["code"], str):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
        error_code = error_value["code"]
    accepted_at = _media_timestamp(response["accepted_at"])
    updated_at = _media_timestamp(response["updated_at"])
    deadline_at = _media_timestamp(response["deadline_at"])
    if updated_at < accepted_at or deadline_at <= accepted_at:
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    return ClientMediaOperation(
        operation_id=response["operation_id"],
        capability=response["capability"],
        provider=response["provider"],
        model=response.get("model"),
        catalog_revision=response["catalog_revision"],
        state=response["state"],
        cancellation_state=response["cancellation_state"],
        outputs=outputs,
        partial_outputs=partial_outputs,
        previous_operation_id=previous_operation_id,
        error_code=error_code,
        cost_available=cost["available"],
        cost_reason=cost_reason,
        accepted_at=response["accepted_at"],
        updated_at=response["updated_at"],
        deadline_at=response["deadline_at"],
    )


def _decode_media_operation_partial(value: Any) -> ClientMediaOperationPartialOutput:
    """Decode one exact progressive asset reference."""

    if (
        not isinstance(value, dict)
        or set(value) != {"asset_id", "mime_type", "size_bytes", "output_ordinal", "partial_ordinal"}
        or not isinstance(value["asset_id"], str)
        or not ASSET_ID_PATTERN.fullmatch(value["asset_id"])
        or not isinstance(value["mime_type"], str)
        or not value["mime_type"]
        or any(isinstance(value[field], bool) or not isinstance(value[field], int) for field in ("size_bytes", "output_ordinal", "partial_ordinal"))
        or value["size_bytes"] <= 0
        or value["output_ordinal"] < 0
        or value["partial_ordinal"] < 0
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation preview")
    return ClientMediaOperationPartialOutput(**value)


def _decode_media_operation_output(value: Any) -> ClientMediaOperationOutput:
    """Decode one exact operation output asset."""

    if (
        not isinstance(value, dict)
        or set(value) != {"asset_id", "mime_type", "size_bytes", "ordinal"}
        or not isinstance(value["asset_id"], str)
        or not ASSET_ID_PATTERN.fullmatch(value["asset_id"])
        or not isinstance(value["mime_type"], str)
        or not value["mime_type"]
        or isinstance(value["size_bytes"], bool)
        or not isinstance(value["size_bytes"], int)
        or value["size_bytes"] <= 0
        or isinstance(value["ordinal"], bool)
        or not isinstance(value["ordinal"], int)
        or value["ordinal"] < 0
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    return ClientMediaOperationOutput(
        asset_id=value["asset_id"],
        mime_type=value["mime_type"],
        size_bytes=value["size_bytes"],
        ordinal=value["ordinal"],
    )


def _decode_media_capabilities(value: Any) -> ClientMediaCapabilities:
    """Decode the exact public media-capability collection."""

    if (
        not isinstance(value, dict)
        or set(value) != {"catalog_revision", "routes", "resources", "services"}
        or not isinstance(value["catalog_revision"], str)
        or not value["catalog_revision"]
        or not isinstance(value["routes"], list)
        or not isinstance(value["resources"], list)
        or not isinstance(value["services"], list)
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media capabilities response")
    resources = tuple(_decode_provider_resource(resource) for resource in value["resources"])
    if len(set(resources)) != len(resources):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: duplicate provider resource")
    services = tuple(_decode_media_capability_service(service) for service in value["services"])
    if len({(service.provider, service.capability) for service in services}) != len(services):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: duplicate provider service")
    return ClientMediaCapabilities(
        catalog_revision=value["catalog_revision"],
        routes=tuple(_decode_media_capability_route(route) for route in value["routes"]),
        resources=resources,
        services=services,
    )


def _decode_media_capability_service(value: Any) -> ClientMediaCapabilityService:
    """Decode one explicit model-free service."""

    if (
        not isinstance(value, dict)
        or set(value) != {"capability", "provider", "controls", "limits"}
        or value["capability"] not in ("audio.align", "audio.dictionary.create")
        or not isinstance(value["provider"], str)
        or not value["provider"]
        or any(not isinstance(value[field], list) or not all(isinstance(item, dict) for item in value[field]) for field in ("controls", "limits"))
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider service")
    return ClientMediaCapabilityService(capability=value["capability"], provider=value["provider"], controls=tuple(value["controls"]), limits=tuple(value["limits"]))


def _decode_provider_resource(value: Any) -> ClientProviderResource:
    """Decode the exact provider-resource discovery contract."""

    if (
        not isinstance(value, dict)
        or set(value) != {"provider", "kind"}
        or not isinstance(value["provider"], str)
        or not value["provider"]
        or not isinstance(value["kind"], str)
        or value["kind"] not in PROVIDER_RESOURCE_KINDS
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider resource")
    return ClientProviderResource(provider=value["provider"], kind=value["kind"])


def _decode_media_capability_route(value: Any) -> ClientMediaCapabilityRoute:
    """Decode one exact public media-capability route."""

    if (
        not isinstance(value, dict)
        or set(value) != {"capability", "provider", "model", "controls", "limits"}
        or any(
            not isinstance(value[field], str) or not value[field]
            for field in ("capability", "provider", "model")
        )
        or not isinstance(value["controls"], list)
        or not all(isinstance(control, dict) for control in value["controls"])
        or not isinstance(value["limits"], list)
        or not all(isinstance(limit, dict) for limit in value["limits"])
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media capabilities response")
    return ClientMediaCapabilityRoute(
        capability=value["capability"],
        provider=value["provider"],
        model=value["model"],
        controls=tuple(value["controls"]),
        limits=tuple(value["limits"]),
    )


def _decode_media_voice(value: Any) -> ClientMediaVoice:
    """Decode one exact public voice without accepting private provider fields."""

    required_fields = {
        "voice_id",
        "provider",
        "mode",
        "language",
        "display_name",
        "default",
        "sample_rates",
        "default_sample_rate",
        "description", "category", "labels", "high_quality_base_model_ids", "verified_languages", "preview",
    }
    if (
        not isinstance(value, dict)
        or set(value) != required_fields
        or not isinstance(value["voice_id"], str)
        or not MEDIA_VOICE_ID_PATTERN.fullmatch(value["voice_id"])
        or not isinstance(value["provider"], str)
        or not value["provider"]
        or value["mode"] not in {"preset", "extracted"}
        or value["language"] is not None and (not isinstance(value["language"], str) or not value["language"].strip())
        or not isinstance(value["display_name"], str)
        or not value["display_name"]
        or not isinstance(value["default"], bool)
        or not isinstance(value["sample_rates"], list)
        or any(isinstance(rate, bool) or not isinstance(rate, int) or rate <= 0 for rate in value["sample_rates"])
        or len(set(value["sample_rates"])) != len(value["sample_rates"])
        or (value["default_sample_rate"] is None) != (len(value["sample_rates"]) == 0)
        or value["default_sample_rate"] is not None and (type(value["default_sample_rate"]) is not int or value["default_sample_rate"] not in value["sample_rates"])
        or any(value[key] is not None and not isinstance(value[key], str) for key in ("description", "category"))
        or not isinstance(value["labels"], dict) or any(not isinstance(key, str) or not isinstance(label, str) for key, label in value["labels"].items())
        or not isinstance(value["high_quality_base_model_ids"], list) or any(not isinstance(model, str) or not model.strip() for model in value["high_quality_base_model_ids"])
        or not isinstance(value["verified_languages"], list)
        or not _valid_voice_preview(value["preview"], value["voice_id"], 0)
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media voice response")
    languages = []
    for index, language in enumerate(value["verified_languages"]):
        if (
            not isinstance(language, dict) or set(language) != {"language", "model_id", "accent", "locale", "preview"}
            or any(not isinstance(language[key], str) or not language[key].strip() for key in ("language", "model_id"))
            or any(language[key] is not None and not isinstance(language[key], str) for key in ("accent", "locale"))
            or not _valid_voice_preview(language["preview"], value["voice_id"], index + 1)
        ):
            raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media voice language")
        languages.append(ClientMediaVoiceLanguage(**language))
    return ClientMediaVoice(
        voice_id=value["voice_id"],
        provider=value["provider"],
        mode=value["mode"],
        language=value["language"],
        display_name=value["display_name"],
        default=value["default"],
        sample_rates=tuple(value["sample_rates"]),
        default_sample_rate=value["default_sample_rate"],
        description=value["description"], category=value["category"], labels=dict(value["labels"]),
        high_quality_base_model_ids=tuple(value["high_quality_base_model_ids"]), verified_languages=tuple(languages), preview=value["preview"],
    )


def _valid_voice_preview(value: Any, voice_id: str, index: int) -> bool:
    return value is None or value == f"{MEDIA_VOICES_ENDPOINT_PATH}/{voice_id}/previews/{index}"


def _media_timestamp(value: Any) -> datetime:
    """Parse one exact UTC media-resource timestamp."""

    if not isinstance(value, str) or not value.endswith("Z"):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    try:
        timestamp = datetime.fromisoformat(value.removesuffix("Z") + "+00:00")
    except ValueError as error:
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response") from error
    if timestamp.tzinfo is None or timestamp.utcoffset() != timezone.utc.utcoffset(timestamp):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid media operation response")
    return timestamp


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


def default_response_opener(request: urllib.request.Request, *, timeout: float | None = None) -> ClientHTTPResponse:
    """Execute a prepared request and retain its HTTP status and headers."""

    with urllib.request.urlopen(request, timeout=timeout) as response:
        response_body = cast(bytes, response.read())
        return ClientHTTPResponse(status_code=response.status, body=response_body.decode("utf-8"), headers=dict(response.headers.items()))


def _open_response(opener: ResponseOpener, request: urllib.request.Request, failure_context: str, timeout: float | None = None) -> ClientHTTPResponse:
    try:
        response = opener(request, timeout=timeout)
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="replace")
        raise LLMProxyHTTPError(error.code, body, str(error.reason), failure_context) from error
    except (urllib.error.URLError, TimeoutError, OSError) as error:
        reason = error.reason if isinstance(error, urllib.error.URLError) else str(error)
        raise LLMProxyTransportError(f"llm_proxy_client_transport_failure: {failure_context} reason={reason}") from error
    if not isinstance(response, ClientHTTPResponse):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid HTTP transport response")
    if response.status_code < 200 or response.status_code >= 300:
        raise LLMProxyHTTPError(response.status_code, response.body, "", failure_context)
    return response


def _decode_text_request_pending(body: str) -> ClientTextRequestResult:
    try:
        value = json.loads(body)
    except json.JSONDecodeError as error:
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid text request receipt") from error
    fields = {"state", "proxy_request_id", "started_at", "updated_at", "elapsed_seconds"}
    if not isinstance(value, dict) or set(value) != fields or value["state"] not in ("not_dispatched", "dispatched"):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid text request receipt")
    return ClientTextRequestResult(**value)


def _proxy_error_code(body: str) -> str:
    try:
        envelope = json.loads(body)
    except json.JSONDecodeError:
        return ""
    if not isinstance(envelope, dict) or not isinstance(envelope.get("error"), dict):
        return ""
    code = envelope["error"].get("code")
    recognized = {
        "insufficient_funds", "financial_admission_unavailable",
        "hosted_authority_denied", "hosted_result_expired", "usage_journal_conflict", "usage_journal_claim_lost",
        "invalid_idempotency_key", "structured_request_failed", "structured_request_intent_conflict",
        "structured_request_invalid", "structured_request_not_found", "structured_request_outcome_unknown",
        "structured_request_store_error", "provider_error", "provider_rate_limited", "request_timeout",
    }
    return code if isinstance(code, str) and code in recognized else ""


def _provider_optional_integer(value: Any) -> bool:
    return value is None or (type(value) is int and value >= 0)


def _decode_provider_model(value: Any) -> ClientProviderModelMetadata:
    fields = {"model_id", "name", "can_do_text_to_speech", "can_do_voice_conversion", "maximum_text_length_per_request", "max_characters_request_free_user", "max_characters_request_subscribed_user"}
    if (
        not isinstance(value, dict) or set(value) != fields
        or any(not isinstance(value[key], str) or not value[key].strip() for key in ("model_id", "name"))
        or any(value[key] is not None and type(value[key]) is not bool for key in ("can_do_text_to_speech", "can_do_voice_conversion"))
        or any(not _provider_optional_integer(value[key]) for key in ("maximum_text_length_per_request", "max_characters_request_free_user", "max_characters_request_subscribed_user"))
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider model")
    return ClientProviderModelMetadata(**value)


def _decode_provider_subscription(value: Any) -> ClientProviderSubscription:
    fields = {"tier", "status", "character_count", "character_limit", "credit_extension", "can_extend_credit_limit", "current_overage", "has_open_invoices", "currency", "next_character_count_reset_unix"}
    if (
        not isinstance(value, dict) or set(value) != fields
        or any(not isinstance(value[key], str) or not value[key].strip() for key in ("tier", "status"))
        or any(type(value[key]) is not int or value[key] < 0 for key in ("character_count", "character_limit"))
        or any(type(value[key]) is not bool for key in ("can_extend_credit_limit", "has_open_invoices"))
        or not _provider_optional_integer(value["next_character_count_reset_unix"])
        or (value["currency"] is not None and (not isinstance(value["currency"], str) or not value["currency"]))
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider subscription")
    extension = value["credit_extension"]
    if (
        not isinstance(extension, dict) or set(extension) != {"unlimited", "value"}
        or type(extension["unlimited"]) is not bool
        or (extension["unlimited"] and extension["value"] is not None)
        or (not extension["unlimited"] and (type(extension["value"]) is not int or extension["value"] < 0))
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider credit extension")
    overage = value["current_overage"]
    if overage is not None and (
        not isinstance(overage, dict) or set(overage) != {"amount", "currency"}
        or not isinstance(overage["amount"], str) or re.fullmatch(r"[0-9]+(?:\.[0-9]+)?", overage["amount"]) is None
        or not isinstance(overage["currency"], str) or not overage["currency"]
    ):
        raise LLMProxyTransportError("llm_proxy_client_transport_failure: invalid provider overage")
    return ClientProviderSubscription(
        tier=value["tier"], status=value["status"], character_count=value["character_count"], character_limit=value["character_limit"],
        credit_extension=ClientProviderCreditExtension(**extension), can_extend_credit_limit=value["can_extend_credit_limit"],
        current_overage=ClientProviderOverage(**overage) if overage is not None else None,
        has_open_invoices=value["has_open_invoices"], currency=value["currency"], next_character_count_reset_unix=value["next_character_count_reset_unix"],
    )
