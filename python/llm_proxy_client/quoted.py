"""Bound hosted quote resources over the configured official client transport."""
from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Any

from .client import (
    Client, ClientMediaOperation, ClientMediaOperationInput, ClientMessagesRequest,
    ClientTextRequestResult, LLMProxyClientError, LLMProxyRequestPendingError,
    LLMProxyTransportError, ClientHTTPResponse, IDEMPOTENCY_KEY_PATTERN,
    FINANCIAL_DIGEST_PATTERN, FINANCIAL_INTEGER_PATTERN, FINANCIAL_REQUEST_ID_PATTERN,
    DIAGNOSTIC_PROVIDER_PATTERN, _decode_media_operation, _decode_text_request_pending,
)

FINANCIAL_QUOTES_PATH = "/model/v1/financial-quotes"
QUOTE_ID_PATTERN = re.compile(r"^quote-[a-f0-9]{32}$")
HEADER_QUOTE_ID = "x-llm-proxy-quote-id"
HEADER_PROCESSING_DIGEST = "x-llm-proxy-processing-digest"
HEADER_REQUEST_DIGEST = "x-llm-proxy-request-digest"
QUOTE_FIELDS = frozenset({"id", "revision", "state", "execution_kind", "processing_digest", "request_digest", "request_id", "reservation_id", "pricing_revision", "price_snapshot_digest", "currency", "maximum_cents", "expires_at", "created_at", "updated_at", "execution_id"})


def _invalid_input(message: str) -> LLMProxyClientError:
    return LLMProxyClientError(f"llm_proxy_client_invalid_request: {message}")


def _invalid_response(message: str) -> LLMProxyTransportError:
    return LLMProxyTransportError(f"llm_proxy_client_transport_failure: {message}")


def _encoded(value: Any) -> str:
    try:
        return json.dumps(value, ensure_ascii=False, allow_nan=False, sort_keys=True, separators=(",", ":"))
    except (TypeError, ValueError) as error:
        raise _invalid_input("invalid quoted JSON") from error


@dataclass(frozen=True)
class FinancialQuoteInput:
    """An immutable typed execution intent. Nested payloads are detached JSON."""

    processing_digest: str
    execution_kind: str
    provider: str
    _request_json: str = field(repr=False)

    def __post_init__(self) -> None:
        if not isinstance(self.processing_digest, str) or not FINANCIAL_DIGEST_PATTERN.fullmatch(self.processing_digest) or not isinstance(self.execution_kind,str) or self.execution_kind not in {"native_v2", "media_operation"} or not isinstance(self.provider, str) or not DIAGNOSTIC_PROVIDER_PATTERN.fullmatch(self.provider):
            raise _invalid_input("invalid quoted processing identity")
        try:
            request = json.loads(self._request_json)
        except (TypeError, json.JSONDecodeError) as error:
            raise _invalid_input("invalid quoted request") from error
        if not isinstance(request, dict) or not isinstance(request.get("model"), str) or not request["model"].strip():
            raise _invalid_input("quoted request requires an explicit model")
        if self.execution_kind == "native_v2":
            if not isinstance(request.get("messages"), list) or not request["messages"] or not isinstance(request.get("structured_output"), dict):
                raise _invalid_input("quoted native request requires messages and structured output")
        elif request.get("capability") not in {"image.generate", "image.edit"} or request.get("provider") != self.provider or not isinstance(request.get("input"), dict) or not isinstance(request.get("controls"), dict):
            raise _invalid_input("quoted media requires a matching image intent")
        object.__setattr__(self, "_request_json", _encoded(request))

    @classmethod
    def native(cls, processing_digest: str, provider: str, request: ClientMessagesRequest) -> FinancialQuoteInput:
        """Freeze an explicit structured request with the server work-budget policy."""
        if not isinstance(request, ClientMessagesRequest) or request.structured_output is None or request.request_timeout_seconds is not None:
            raise _invalid_input("quoted native requires structured output with server work budget")
        return cls(processing_digest, "native_v2", provider, _encoded(request.body()))

    @classmethod
    def media(cls, processing_digest: str, request: ClientMediaOperationInput) -> FinancialQuoteInput:
        """Freeze an image generation or image edit intent."""
        if not isinstance(request, ClientMediaOperationInput):
            raise _invalid_input("quoted media requires typed input")
        return cls(processing_digest, "media_operation", request.provider, _encoded(request.body()))

    def body(self) -> dict[str, Any]:
        """Return an independent complete request body."""
        return {"processing_digest": self.processing_digest, "intent": {"kind": self.execution_kind, "provider": self.provider, "request": json.loads(self._request_json)}}


def _timestamp(value: str) -> datetime:
    try:
        if not isinstance(value,str) or not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|\+00:00)",value):
            raise ValueError("RFC3339 UTC required")
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except (ValueError, TypeError) as error:
        raise _invalid_response("invalid quote timestamp") from error
    if parsed.utcoffset() != timedelta(0) or "T" not in value:
        raise _invalid_response("quote timestamp must be RFC3339 UTC")
    return parsed


@dataclass(frozen=True)
class FinancialQuote:
    """Retained reservation evidence, independent of execution payload lifetime."""

    id: str
    revision: int
    state: str
    execution_kind: str
    processing_digest: str
    request_digest: str
    request_id: str
    reservation_id: str
    pricing_revision: str
    price_snapshot_digest: str
    currency: str
    maximum_cents: str
    expires_at: str
    created_at: str
    updated_at: str
    execution_id: str
    _bound_input: FinancialQuoteInput | None = field(default=None, repr=False, compare=False)

    def __post_init__(self) -> None:
        strings = (self.id, self.state, self.execution_kind, self.processing_digest, self.request_digest, self.request_id, self.reservation_id, self.pricing_revision, self.price_snapshot_digest, self.currency, self.maximum_cents, self.expires_at, self.created_at, self.updated_at, self.execution_id)
        if any(not isinstance(value, str) for value in strings) or type(self.revision) is not int or not 0 < self.revision <= 2**64 - 1:
            raise _invalid_response("invalid quote field types")
        if not QUOTE_ID_PATTERN.fullmatch(self.id) or self.state not in {"reserved", "consumed", "expired", "released"} or self.execution_kind not in {"native_v2", "media_operation"} or not FINANCIAL_REQUEST_ID_PATTERN.fullmatch(self.request_id) or self.reservation_id != self.request_id or not self.pricing_revision.strip():
            raise _invalid_response("invalid quote identity or state")
        if any(not FINANCIAL_DIGEST_PATTERN.fullmatch(value) for value in (self.processing_digest, self.request_digest, self.price_snapshot_digest)) or self.currency != "USD" or not FINANCIAL_INTEGER_PATTERN.fullmatch(self.maximum_cents) or len(self.maximum_cents)>19 or int(self.maximum_cents) > 2**63 - 1:
            raise _invalid_response("invalid quote digests or bound")
        created, updated, expires = _timestamp(self.created_at), _timestamp(self.updated_at), _timestamp(self.expires_at)
        if expires <= created or updated < created or self.state == "expired" and updated < expires:
            raise _invalid_response("inconsistent quote timestamps")
        if (self.state == "consumed" and not IDEMPOTENCY_KEY_PATTERN.fullmatch(self.execution_id)) or (self.state != "consumed" and self.execution_id):
            raise _invalid_response("inconsistent quote execution identity")


@dataclass(frozen=True)
class QuotedExecution:
    """A result with verified quote, processing and opaque request identities."""

    quote_id: str
    processing_digest: str
    request_digest: str
    native: ClientTextRequestResult | None = None
    media: ClientMediaOperation | None = None

    def __post_init__(self) -> None:
        if not QUOTE_ID_PATTERN.fullmatch(self.quote_id) or not FINANCIAL_DIGEST_PATTERN.fullmatch(self.processing_digest) or not FINANCIAL_DIGEST_PATTERN.fullmatch(self.request_digest) or (self.native is None) == (self.media is None):
            raise _invalid_response("invalid quoted execution")

    def pending_error(self) -> LLMProxyRequestPendingError | None:
        """Return the existing native pending error without changing execution."""
        if self.native is not None and self.native.state != "succeeded":
            return LLMProxyRequestPendingError(self.native)
        return None


def _json_object(response: ClientHTTPResponse) -> dict[str, Any]:
    if len(response.body.encode("utf-8")) > 8 * 1024 * 1024:
        raise _invalid_response("quoted response exceeds limit")
    try:
        value = json.loads(response.body)
    except (TypeError, json.JSONDecodeError) as error:
        raise _invalid_response("invalid quoted response JSON") from error
    if not isinstance(value, dict):
        raise _invalid_response("quoted resource response must be an object")
    return value


@dataclass(frozen=True)
class QuotedClient:
    """Quote and bound execution resources on one explicitly configured Client."""

    client: Client

    def __post_init__(self) -> None:
        if not isinstance(self.client, Client):
            raise _invalid_input("quoted client requires a configured Client")

    def _quote(self, method: str, path: str, body: dict[str, Any] | None = None, key: str = "", bound: FinancialQuoteInput | None = None) -> FinancialQuote:
        headers = {"Idempotency-Key": key} if key else None
        response = self.client._media_resource_response(method, path, body=body, headers=headers)
        expected = {200, 201} if method == "POST" else {200}
        if response.status_code not in expected:
            raise _invalid_response("unexpected quote response status")
        value = _json_object(response)
        if set(value) != QUOTE_FIELDS:
            raise _invalid_response("invalid quote fields")
        quote = FinancialQuote(**value, _bound_input=bound)
        if bound is not None and (quote.processing_digest != bound.processing_digest or quote.execution_kind != bound.execution_kind):
            raise _invalid_response("quote belongs to another processing intent")
        return quote

    def create_financial_quote(self, idempotency_key: str, intent: FinancialQuoteInput) -> FinancialQuote:
        """Reserve a maximum without provider dispatch or hidden retry."""
        if not isinstance(idempotency_key, str) or not IDEMPOTENCY_KEY_PATTERN.fullmatch(idempotency_key) or not isinstance(intent, FinancialQuoteInput):
            raise _invalid_input("invalid quote creation")
        return self._quote("POST", FINANCIAL_QUOTES_PATH, intent.body(), idempotency_key, intent)

    def get_financial_quote(self, idempotency_key: str) -> FinancialQuote:
        """Recover creation by key without the original request payload."""
        if not isinstance(idempotency_key, str) or not IDEMPOTENCY_KEY_PATTERN.fullmatch(idempotency_key):
            raise _invalid_input("invalid quote key")
        return self._quote("GET", FINANCIAL_QUOTES_PATH, key=idempotency_key)

    def get_financial_quote_by_id(self, quote_id: str) -> FinancialQuote:
        """Read retained quote evidence after execution payload deletion."""
        if not isinstance(quote_id, str) or not QUOTE_ID_PATTERN.fullmatch(quote_id):
            raise _invalid_input("invalid quote ID")
        quote = self._quote("GET", f"{FINANCIAL_QUOTES_PATH}/{quote_id}")
        if quote.id != quote_id:
            raise _invalid_response("quote belongs to another resource")
        return quote

    def release_financial_quote(self, quote: FinancialQuote) -> FinancialQuote:
        """Release by revision. An error does not confirm release."""
        if not isinstance(quote, FinancialQuote):
            raise _invalid_input("invalid quote handle")
        released = self._quote("PATCH", f"{FINANCIAL_QUOTES_PATH}/{quote.id}", {"revision": quote.revision, "state": "released"})
        stable = QUOTE_FIELDS - {"revision", "state", "updated_at", "execution_id"}
        if quote.state not in {"reserved","released"} or quote.state=="reserved" and released.revision<=quote.revision or _timestamp(released.updated_at)<_timestamp(quote.updated_at) or released.state != "released" or released.revision < quote.revision or any(getattr(released, name) != getattr(quote, name) for name in stable):
            raise _invalid_response("release differs from quoted reservation")
        return released

    def create_quoted_execution(self, quote: FinancialQuote, intent: FinancialQuoteInput) -> QuotedExecution:
        """Consume the existing hold with the quote ID as execution key."""
        if not isinstance(quote, FinancialQuote) or not isinstance(intent, FinancialQuoteInput) or quote.state not in {"reserved", "consumed"} or quote.processing_digest != intent.processing_digest or quote.execution_kind != intent.execution_kind or quote._bound_input is not None and quote._bound_input != intent:
            raise _invalid_input("execution differs from quoted intent")
        return self._execution("POST", quote, intent.body())

    def get_quoted_execution(self, quote: FinancialQuote) -> QuotedExecution:
        """Read once without a provider call or execution retry."""
        if not isinstance(quote, FinancialQuote):
            raise _invalid_input("invalid quote handle")
        return self._execution("GET", quote)

    def _execution(self, method: str, quote: FinancialQuote, body: dict[str, Any] | None = None) -> QuotedExecution:
        response = self.client._media_resource_response(method, f"{FINANCIAL_QUOTES_PATH}/{quote.id}/executions", body=body)
        if response.status_code not in {200, 202}:
            raise _invalid_response("unexpected quoted execution status")
        expected = {HEADER_QUOTE_ID: quote.id, HEADER_PROCESSING_DIGEST: quote.processing_digest, HEADER_REQUEST_DIGEST: quote.request_digest}
        if any(response.headers.get(name) != value for name, value in expected.items()):
            raise _invalid_response("execution response belongs to another quoted intent")
        if len(response.body.encode("utf-8")) > 8 * 1024 * 1024:
            raise _invalid_response("quoted execution response exceeds limit")
        if quote.execution_kind == "native_v2":
            if response.status_code == 202:
                native = _decode_text_request_pending(response.body)
                if quote.execution_id and native.proxy_request_id != quote.execution_id:
                    raise _invalid_response("native execution differs from quote")
            else:
                native = ClientTextRequestResult(state="succeeded", output=response.body)
            return QuotedExecution(quote.id, quote.processing_digest, quote.request_digest, native=native)
        media = _decode_media_operation(_json_object(response))
        if media.capability not in {"image.generate","image.edit"} or not DIAGNOSTIC_PROVIDER_PATTERN.fullmatch(media.provider) or not media.model or quote.execution_id and media.operation_id != quote.execution_id:
            raise _invalid_response("media execution differs from quote")
        if quote._bound_input is not None:
            request = quote._bound_input.body()["intent"]["request"]
            if (media.provider, media.model, media.capability) != (request["provider"], request["model"], request["capability"]):
                raise _invalid_response("media route differs from frozen intent")
        return QuotedExecution(quote.id, quote.processing_digest, quote.request_digest, media=media)
