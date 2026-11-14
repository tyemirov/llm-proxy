# Qwen 3.8 Flash Production Runbook

## Selected route

The user selected Alibaba Cloud on November 13, 2026.
The exact proxy route is `dashscope/qwen3.8-flash` through synchronous Responses.
Use the approved Singapore workspace connection:

| Input | Selected value |
| --- | --- |
| Alibaba account | `5123889231922603` |
| Workspace | `ws-s8jvkkuq2w12az3h` |
| Access region | `ap-southeast-1` |
| Provider URL | `https://ws-s8jvkkuq2w12az3h.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1` |
| Model | `qwen3.8-flash` |
| Required model grants | `qwen-plus` for canonical connection verification and `qwen3.8-flash` for generation |
| Production website | `https://llm-proxy.mprlab.com/` |
| Production API | `https://llm-proxy-api.mprlab.com` |

The approved key is in private `configs/.env` for local qualification.
Production stores the key in an account-owned encrypted provider connection, assigned explicitly to the selected tenant.
The deployment manifest supplies the application runtime and retained database; it does not supply upstream provider keys.
The exact workspace origin requires its committed admission allocation: four active and 24 queued requests.

## Qualification and source activation

F050's Flash scope passed live qualification through the real CLI, saved managed connection, and public HTTP route.
The five text cases covered omitted effort, `none`, `low`, `medium`, and `xhigh`.
An ordered red PNG and blue JPEG returned `RED BLUE`.
All six generation requests returned HTTP 200 and consistent, positive measured usage.
The [execution record](../.mprlab/evidence/F050-flash-20261113.json) retains request IDs, durations, and token counts.

The source catalog enables Flash only. The other four Qwen 3.8 candidates remain disabled.
Local public and tenant discovery include Flash.
Local tests verify explicit model dispatch, retained provider defaults, image admission boundaries, and browser selection after reload.
Final `make ci` passed all 15 gates with 100.0% Go statement coverage in 839 seconds.
All 177 frontend browser tests and 13 authentication browser tests passed.
Independent final review found no remaining actionable code or security findings.
The browser reload test uses a controlled management API; deployed tenant acceptance remains a separate check.

## Request and cost contract

The selected route supports text and ordered image input.
Strict JSON Schema generation is available for text-only requests through the native structured request lifecycle.
Image inputs combined with a schema are rejected before provider work.
Caller tools, video, PDF input, and server-side conversation storage are outside this proxy offering.
Responses requests use `store: false`.

Structured responses require completed provider status and valid JSON that conforms to the supplied supported schema.
Output-limit responses fail without continuation, partial JSON repair, or another provider.
See the [DashScope schema contract](dashscope-responses.md#structured-generation).

Omitted reasoning effort uses Alibaba's `xhigh` default.
Use explicit `none` for requests that should avoid reasoning work, subject to application quality requirements.
The proxy exposes only the distinct `none`, `low`, `medium`, and `xhigh` controls.
The `max_tokens` range is 16 through 131,072. It includes reasoning and visible output for each provider response.
Continuation can make additional provider calls; the per-response limit is not a total cost cap.

Singapore list prices are $0.15 input and $0.47 output per million tokens, verified November 13, 2026.
Reasoning contributes to billed output. Promotions, free quota, and cache discounts are separate from list prices.

The nominal model context is 1,000,000 tokens.
The Responses API documents an effective input window of approximately 80% and automatically truncates excess input.
It documents no truncation-disable control or exact safe threshold.
Maximum-context input preservation is outside the qualified scope.
Use representative application requests within their own accepted prompt budget before selecting this route for long-context work.
Source: [Responses limitations and controls](https://www.alibabacloud.com/help/en/model-studio/qwen-api-via-openai-responses).

## Production activation

F095 tracks the remaining production operations. F050 records repository implementation.
The user selected the existing TellTale tenant under the signed-in Vadym Tyemirov account.
The admin view directly verified the owner email, recorded here as `tem****@gmail.com`.
The November 13 Chrome inspection confirmed tenant `managed-13f5fc612c3fac74f219d0bd483286be`, with two existing connections and an existing client key.
The approved operation subsequently saved `Alibaba Singapore` connection
`connection-9cfa9a89de34a14a61c425d2f10d4871` and assigned it to TellTale.
The existing client key remains assigned to TellTale; the deployed Story Service key resolves to that tenant.
The temporary connection tab was closed after persistence verification.

1. Complete repository validation and have the selected changes committed through the normal execution chain.
2. Prepare and publish the immutable release through the repository lifecycle. Use `make release` and `make publish` only for their separately selected phases.
3. The operator runs canonical `make deploy` for application activation. Verify the deployed API, website release marker, and exact release identity.
4. Confirm production discovery includes `dashscope/qwen3.8-flash` and the deployed capacity configuration includes the selected workspace origin.
5. In the verified owner's account, create or reuse the Alibaba connection with the selected URL and private key. Verify the connection and attach it to TellTale.
6. Select Flash explicitly for acceptance. Change the tenant's text default only as part of the selected routing decision.
7. Run authenticated production text and image acceptance for this exact tenant and route, retaining safe request IDs and usage.
8. Record the release identity, connection and tenant identifiers, selected default, and production acceptance results here or in a linked operational record.

The approved `make deploy` operation activated LLM Proxy `v1.12.13` from
`0d4853647a2953949610943466050aa71a37af37`.
Six authenticated production Flash checks passed through TellTale's existing key:
omitted effort, `none`, `low`, `medium`, `xhigh`, and ordered PNG/JPEG input.
They consumed 657 input and 172 output tokens, with positive consistent usage and unique request identifiers.
This evidence covers direct text and image requests; it does not qualify structured Story generation.
The private operation record is `.git/llm-proxy-operations/F095.json`.
Native lifecycle receipts retain the immutable release, publication, and deployment identities.

F096 adds the strict text-only schema contract needed by TellTale.
Deploy and qualify that dependency before activating the consumer's default route.
TellTale's single automatic route controls new World, Plot, and Story operations.
Use `dashscope`, `qwen3.8-flash`, and explicit `none` after application qualification.
Existing operations retain their captured provider, model, reasoning effort, and work budget.
Acceptance must complete a fresh synthetic World-to-Story flow, select an opening explicitly,
and verify the completed Story and Reader with linked invocation and usage evidence.
Preserve paused user work and inspect active paid work immediately before consumer activation.

Read-only lifecycle preparation confirmed Gateway `v9.2.2`, lifecycle contract `4`, and GitHub actor `tyemirov` with administrator access to `tyemirov/llm-proxy`.
Local and remote `master` both identified `cd32123b6c7508ce7dcde164e9a081679a27f0f5` before Git finalization.
The selected MPR lifecycle uses committed default-branch source and requires it to match the remote default branch.
Unrelated pending work remains outside the selected release unless its source is committed.
