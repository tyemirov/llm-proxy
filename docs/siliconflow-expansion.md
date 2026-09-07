# SiliconFlow Expansion Assessment

P010 records the next SiliconFlow expansion from the September 5, 2026 provider audit.
This assessment does not register new routes.
The user approved the full expansion on September 6, 2026.
F059 owns all six offerings, provider-specific controls, Kimi image input, and independent qualification.

## Current contract

The existing `siliconflow` provider uses `https://api.siliconflow.com/v1` and a tenant API key.
Its text transport uses synchronous Chat Completions.
Its two offerings are `deepseek-reasoner` and `sensevoice-small`.
Their upstream IDs are `deepseek-ai/DeepSeek-R1` and `FunAudioLLM/SenseVoiceSmall`.
Preserve both existing defaults and tenant selections during expansion.

## Candidate mappings

The following exact upstream strings appear in SiliconFlow's current provider documentation.
New public identities below are proposed values.
Existing identities remain shared across provider offerings.

| Public model | SiliconFlow upstream model | Catalog work |
| --- | --- | --- |
| `glm-5.3` | `zai-org/GLM-5.3` | Add a provider offering for the existing disabled model |
| `deepseek-v4-pro-20260813` | `deepseek-ai/DeepSeek-V4-Pro-0813` | Add the exact dated model and offering |
| `deepseek-v4-flash-20260731` | `deepseek-ai/DeepSeek-V4-Flash-0731` | Add the exact dated model and offering |
| `kimi-k3` | `moonshotai/Kimi-K3` | Add a provider offering for the existing model |
| `hy3` | `tencent/Hy3` | Add the Tencent publisher, model family, model, and offering |
| `longcat-2.0` | `meituan-longcat/LongCat-2.0` | Add the Meituan publisher, model family, model, and offering |

The DeepSeek announcements name separate dated API selectors.
They do not establish that the undated selectors resolve to the same versions.
Use the announced dated selectors and verify their authenticated availability before registration.

## Provider-specific controls

GLM-5.3 documents `reasoning_effort` values `low`, `high`, and `max`, with `max` as the provider default.
Its published input, output, and cache-read rates are USD 1.40, 4.40, and 0.26 per million tokens.
The Hy3 example uses `enable_thinking` and `thinking_budget`.
The Kimi K3 announcement describes default maximum thinking, with other levels planned at publication.
These provider contracts require independent verification.
The direct Z.AI, DeepSeek, and Moonshot adapters do not establish SiliconFlow's accepted controls.

Start with the current text contract for each exact route.
Declare an effort only when its SiliconFlow wire mapping and exact values are verified.
Leave an omitted effort absent from the provider request.
Assess Kimi image input separately against the provider's exact MIME types and attachment limits.
Keep output-token limits, media limits, and prices on provider offerings.

## Conflicting source values

The Kimi K3 launch article lists a maximum output of 1049K tokens.
The current homepage model list shows 262K.
The LongCat 2.0 launch article lists 262K output tokens, while the homepage model list shows 131K.
These values are unresolved provider evidence.
Obtain exact integer limits from the authenticated model library or a current API contract before declaring a bound.
Do not convert an ambiguous marketing abbreviation into a precise token limit.

## Qualification sequence

1. Verify an authorized `SILICONFLOW_API_KEY` against the international Models API.
2. Record exact model IDs, controls, limits, price conditions, and verification dates.
3. Build a disposable catalog with the selected complete provider offering.
4. Do a test of HTTP routing, reasoning, usage, continuation, invalid inputs, and management defaults with a local provider boundary.
5. Verify the exact route through the disposable proxy with the authorized provider key.
6. Run a small request for omitted effort and every declared explicit effort.
7. Add qualified offerings to the canonical catalog with unchanged existing defaults.
8. Update public discovery, management selectors, official client fixtures, prices, and current documentation.
9. Run final CI after the last application change.

Qualify one provider offering at a time.
A local protocol test proves the request mapping.
Paid acceptance proves provider access and behavior.
Keep production deployment and acceptance separate from repository completion.

## Activation boundary

F044 attaches `enabled` to the shared exact model.
The runtime keeps all offerings for an enabled model.
Kimi K3 already has an enabled offering through Moonshot.
Adding an unqualified SiliconFlow offering for that model would expose that offering immediately.
Keep proposed SiliconFlow offerings outside the canonical catalog until their independent acceptance passes.
Use a disposable complete catalog for those probes.
An extension to the harness must validate that catalog and preserve the primary checkout.
Changing another provider's model state cannot serve as a SiliconFlow qualification gate.

## Current acceptance inputs

On September 5, 2026, `SILICONFLOW_API_KEY` was absent from the process and all six repository private environment files.
The checked files were the deployment environment and the main, local, API, frontend, and TAuth configuration environments.
Authenticated model discovery and paid acceptance remain pending.
The public provider announcements establish candidate availability, not account access.

## Sources

- [Chat Completions](https://docs.siliconflow.com/en/api-reference/chat-completions/chat-completions)
- [Models API](https://docs.siliconflow.com/en/api-reference/models/get-model-list)
- [Homepage model list](https://www.siliconflow.com/)
- [GLM-5.3 release](https://www.siliconflow.com/blog/glm-5.3-now-live-on-siliconflow)
- [DeepSeek V4 Pro update](https://www.siliconflow.com/blog/deepseek-v4-pro-0813-now-live-on-siliconflow-enhanced-agent-capabilities-for-production-workflows)
- [DeepSeek V4 Flash update](https://www.siliconflow.com/blog/deepseek-v4-flash-0731-now-live-on-siliconflow)
- [Kimi K3 release](https://www.siliconflow.com/blog/kimi-k3-now-live-on-siliconflow-the-first-open-3t-class-model-at-frontier-level-performance)
- [Hy3 release](https://www.siliconflow.com/blog/hy3-now-live-on-siliconflow-advancing-agent-capabilities-ready-for-real-world-workflows)
- [LongCat 2.0 release](https://www.siliconflow.com/blog/longcat-2.0-now-on-siliconflow-1.6t-moe-native-1m-context-built-for-agentic-coding)
