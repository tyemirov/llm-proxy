# Baidu Qianfan

## Connection and routes

F032 adds the canonical `baidu` provider with the display label `Baidu Qianfan`.
The provider uses the international Qianfan API.
Each request uses `POST https://api.baiduqianfan.ai/v1/chat/completions` with bearer authentication.
The connection has one `api_key` credential and one `base_url` setting.
The catalog supplies `https://api.baiduqianfan.ai/v1` as the URL default.
The optional Qianfan `appid` header is absent.

| Exact model | Maximum output tokens | Baidu text default |
| --- | ---: | --- |
| `ernie-5.0` | 65,536 | Yes |
| `deepseek-v4-pro` | 131,072 | No |
| `deepseek-v4-flash` | 131,072 | No |
| `deepseek-v3.2` | 32,768 | No |

The two DeepSeek V4 offerings share exact model records with the direct DeepSeek provider.
The provider selector determines the connection and offering limits.
These Baidu offerings accept text and return text through synchronous requests.
They expose no image, audio, reasoning, tool, search, structured-output, or streaming controls.
The `created` timestamp records catalog registration on September 5, 2026, rather than a provider release date.

Sources: [Quick Start](https://intl.cloud.baidu.com/en/doc/qianfan/s/qm8qxemze-intl-en),
[text API reference](https://intl.cloud.baidu.com/en/doc/qianfan/s/3m7of64lb-intl-en), and
[model list](https://intl.cloud.baidu.com/en/doc/qianfan/s/7m95lyy43-intl-en).
The source review date is September 5, 2026.

## Response policy

The catalog declares `protocol_parameters.response_policy: qianfan` on the shared Chat Completions adapter.
The adapter accepts `finish_reason=stop` as completion.
It sends `length` responses to the common missing-suffix coordinator.
Other finish reasons fail the request.

The adapter checks every returned choice before it exposes text.
An absent `flag` is valid.
A present flag must be the JSON integer `0` or `1`.
Blocked flags, unknown values, null values, and malformed flags fail the request.
A failure cannot expose partial text from the response.
The adapter keeps valid reported token usage when response policy rejects the content.
Managed usage records these tokens on the failed request.
Managed key verification uses the same response parser and policy.

## Managed connection

1. Open the Baidu provider card in the management application.
2. Select the exact Qianfan text model.
3. Keep the default Qianfan API URL or enter the intended Qianfan endpoint.
4. Paste the API key into the credential field.
5. Wait for verification to finish and the field to display a masked value.
6. In Settings, select Baidu and the intended text model as the tenant default.

Verification uses the selected model and a 16-token output limit.
Successful verification saves the encrypted key, provider settings, and eligible tenant default in one transaction.
A rejected response preserves the previous connection and defaults.
Profiles and logs contain no raw key or Qianfan response body.

The runtime uses tenant credentials from the managed store.
A request for Baidu without a saved tenant key returns `409 provider_not_configured` before provider dispatch.
The service rejects provider-key blocks in `config.yml`.

## Prices

The catalog records international pay-as-you-go prices in USD per million tokens.
The price review date is September 5, 2026.

| Exact model | Input | Output | Cache read |
| --- | ---: | ---: | ---: |
| `ernie-5.0` | 1.40 | 5.60 | Not recorded |
| `deepseek-v4-pro` | 1.69 | 3.38 | 0.14 |
| `deepseek-v4-flash` | 0.14 | 0.28 | 0.028 |
| `deepseek-v3.2` | 0.28 | 0.42 | 0.028 |

Source: [Qianfan model prices](https://intl.cloud.baidu.com/en/doc/qianfan/s/Jm8r1826a-intl-en).
The catalog does not infer numeric context limits from abbreviated model-list values.

## Qualification

Run the local HTTP and managed-key checks:

```shell
make test-baidu
make test-live-provider-defaults
```

Run the harness preflight without an external provider call:

```shell
make test-live-provider-harness
```

Put `BAIDU_API_KEY` in an ignored private environment file.
Run one paid key verification and one canonical text request:

```shell
LLM_PROXY_LIVE_PROVIDERS=baidu \
  make test-live-providers LIVE_ENV_FILE=/absolute/path/to/private.env
```

The harness imports provider values only from the selected file.
It uses the catalog URL default, so the API key is sufficient.
It reports HTTP status and route identity without the key or response body.

On September 5, 2026, key verification and text generation passed for the default `ernie-5.0` route.
Both operations returned HTTP 200 through the disposable local API.
The other three models have local protocol coverage but have not completed live qualification in this implementation run.
Production deployment and acceptance remain operator-owned.
