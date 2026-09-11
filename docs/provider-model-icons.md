# Provider and model icons

I253 implements the P013 logo plan in the management application and public catalog.
The source review date is September 7, 2026.

## Scope and placement

The provider catalog defines 14 API providers and 30 model families, including disabled candidates.
I237 separates API connection identities from model family identities.
The icons keep that separation.

| Surface | Placement | Icon size |
| --- | --- | --- |
| Management provider card | Before the API service title on both card faces | 20px |
| Management model family label | Before the family label | 16px |
| Public route explorer | Before the selected family and provider labels | 20px |
| Public model matrix | Before the exact model label and each provider label | 16px |

Native select options remain text labels. The selected-value summary carries the icon.
Charts, numeric usage values, capability symbols, and action controls retain their current presentation.

## API provider logos

The asset names below refer to the reviewed [Lobe Icons SVG directory][svg-directory].
They identify the installed artwork. The collection is not an official asset pack from each brand.

| Catalog provider | Current API service label | Logo | Local SVG |
| --- | --- | --- | --- |
| `openai` | OpenAI API | OpenAI Blossom | `openai.svg` |
| `deepseek` | DeepSeek API | DeepSeek whale | `deepseek-color.svg` |
| `dashscope` | DashScope API | Alibaba Cloud symbol | `alibabacloud-color.svg` |
| `moonshot` | Moonshot API | Moonshot symbol | `moonshot.svg` |
| `minimax` | MiniMax API | MiniMax symbol | `minimax-color.svg` |
| `siliconflow` | SiliconFlow API | SiliconFlow cloud symbol | `siliconcloud-color.svg` |
| `zai` | Z.AI API | Z.AI symbol | `zai.svg` |
| `gemini` | Gemini API | Gemini sparkle | `gemini-color.svg` |
| `anthropic` | Anthropic API | Anthropic symbol | `anthropic.svg` |
| `meta` | Meta API | Meta infinity symbol | `meta-color.svg` |
| `xai` | xAI API | xAI symbol | `xai.svg` |
| `baidu` | Qianfan API | Baidu Cloud symbol | `baiducloud-color.svg` |
| `vertex` | Google Cloud Vertex AI | Vertex AI product symbol | `vertexai-color.svg` |
| `dictator` | Dictator API | Explicit text-only presentation | No verified small logo |

Alibaba Cloud identifies the service behind the [DashScope connection][model-studio].
Baidu Cloud identifies the service organization behind the [Qianfan connection][baidu-cloud].
These are deliberate provider logo choices, not claims that each API has a separate logo.

## Model family logos

Each exact model uses its catalog family mapping.
Versions, reasoning levels, and capabilities do not receive separate brand artwork.

| Catalog families | Logo | Local SVG |
| --- | --- | --- |
| `gpt-6`, `gpt-4`, `gpt-5`, `gpt-transcribe` | OpenAI Blossom | `openai.svg` |
| `ernie-5` | Baidu paw | `baidu-color.svg` |
| `deepseek-v3`, `deepseek-v4`, `deepseek-r1` | DeepSeek whale | `deepseek-color.svg` |
| `qwen`, `qwen3-8` | Qwen symbol | `qwen-color.svg` |
| `kimi-k2`, `kimi-k3` | Kimi symbol | `kimi-color.svg` |
| `minimax-m2`, `minimax-m3` | MiniMax symbol | `minimax-color.svg` |
| `sensevoice` | Explicit text-only presentation | No verified small logo |
| `glm-5`, `glm-asr` | Z.AI symbol | `zai.svg` |
| `gemini` | Gemini sparkle | `gemini-color.svg` |
| `claude-fable`, `claude-sonnet`, `claude-opus`, `claude-haiku` | Claude starburst | `claude-color.svg` |
| `muse-spark`, `muse-voice` | Meta infinity symbol | `meta-color.svg` |
| `grok`, `grok-build`, `grok-code`, `grok-imagine` | Grok symbol | `grok.svg` |
| `xai-stt` | xAI symbol | `xai.svg` |
| `dictator-speech` | Explicit text-only presentation | No verified small logo |

The set contains 18 distinct SVG assets, one explicit text-only provider, and two explicit text-only families.
The Meta and Z.AI choices represent the model publishers.
They do not assert separate Muse or GLM product logos.

The DeepSeek model logo stays the same across DeepSeek, SiliconFlow, and Qianfan offerings.
Gemini models keep the Gemini logo through both Gemini API and Vertex AI.
The provider label identifies the selected connection in each case.

## Asset sources

The reviewed Lobe Icons revision is `a94750e3f5f8fc33757b839d85030e742284e43a`.
All 18 local files come from that revision.
The collection provides static SVG assets under its [MIT license][lobe-license].
The asset directory retains the license.

The shared manifest records the exact source URL and SHA-256 digest for each asset.
Review the source and brand guidance before an asset change.
An asset collection license is not a claim of brand endorsement.

| Source | Purpose |
| --- | --- |
| [Lobe Icons][svg-directory] | Pinned SVG source inventory |
| [OpenAI brand guidance][openai-brand] | OpenAI Blossom and presentation rules |
| [Anthropic official assets][anthropic-brand] | Anthropic and Claude artwork review |
| [Google Cloud icon library][google-icons] | Vertex AI product artwork review |
| [Alibaba Cloud Model Studio][model-studio] | DashScope service identity |
| [Moonshot developer platform][moonshot-platform] | Current Kimi developer identity |
| [SiliconFlow][siliconflow] | SiliconFlow service identity |
| [Baidu AI Cloud][baidu-cloud] | Qianfan service organization |
| [SenseVoice source repository][sensevoice] | Model ownership and logo investigation |

## Presentation rules

1. Keep the visible service, model, and family names beside their icons.
2. Preserve each logo's proportions and native brand colors.
3. Use the supplied light or dark variant when available.
4. Declare a neutral background for an asset that needs more contrast.
5. Give Qwen, Baidu, and DeepSeek artwork a light background for contrast.
6. Keep decorative icons out of the accessible name and keyboard order.
7. Reserve fixed dimensions before the image loads.
8. Keep logos independent from connection status, selection, and usage colors.
9. Keep native selectors and existing card controls unchanged.

## Implementation

`site/assets/llm-proxy/js/brandIconManifest.js` owns asset definitions and exact provider and family mappings.
An explicit `null` mapping selects text-only presentation when no reviewed small logo exists.
`site/assets/llm-proxy/img/brands/` contains the 18 SVG files and their license.
The shared stylesheet declares dimensions and background treatments.

Management uses the `brand-icon` custom element with provider and family IDs from the current payloads.
The public renderer writes image elements into the generated HTML.
Public catalog icons remain available without JavaScript.
Exact models use their catalog family relationship.

The manifest owns presentation only. `configs/providers.yml` continues to own providers, families, models, and offerings.
The existing payloads supply provider and family IDs. The icons need no new API fields or event contracts.
Runtime pages load local assets. They do not depend on a logo CDN or a new React package.
Missing metadata is a build error. It does not select a generic logo or an inferred publisher logo.

`make check-brand-icons` validates the complete private catalog, including disabled candidates.
It rejects missing or unknown mappings, invalid references, missing files, and changed asset digests.
The public renderer also validates local assets and rejects runtime identities without a mapping.
`make frontend-lint` and `make ci` include the catalog and asset check.

`make test-brand-icons` runs the browser and build regressions.
The tests cover model and provider identities, text-only families, local image loads, narrow layouts, and invalid build inputs.

## Acceptance criteria

- Verify the correct logo and visible name on both faces of each management provider card.
- Verify family icons in management and the public route explorer.
- Verify exact model and provider icons in the public matrix.
- Verify DeepSeek identity through SiliconFlow and Qianfan.
- Verify separate Gemini API and Vertex AI connection identities.
- Verify each declared text-only family without a broken image or empty icon space.
- Verify keyboard access, accessible names, card controls, and native selectors.
- Verify desktop and 390px layouts without overflow or new layout movement.
- Verify icon visibility against light and dark backgrounds.
- Verify zero external image requests and successful local asset loads.
- Verify that disabled catalog records remain absent from runtime discovery.
- Verify build failure for an unmapped catalog identity.

## Accepted identity decisions

SenseVoice uses text-only presentation.
The reviewed collection has no SenseVoice entry, and the source review did not establish a distinct small logo.
The original FunAudioLLM repository now redirects to QwenAudio.
That redirect alone does not justify a Qwen logo on the existing SenseVoice family.

The Moonshot logo follows the current `Moonshot API` catalog title.
Its developer URL now redirects to the Kimi platform.
A change to that service title requires a separate explicit catalog decision.

The icons retain native color and use 20px title dimensions.
Monochrome marks and dark Qwen, Baidu, and DeepSeek artwork use a neutral light background.
The white Kimi symbol uses a neutral dark background in every theme.
Browser checks set both theme and palette attributes and verify the actual page colors before icon contrast.

[svg-directory]: https://github.com/lobehub/lobe-icons/tree/a94750e3f5f8fc33757b839d85030e742284e43a/packages/static-svg/icons

[lobe-license]: https://github.com/lobehub/lobe-icons/blob/a94750e3f5f8fc33757b839d85030e742284e43a/LICENSE

[openai-brand]: https://openai.com/brand/

[anthropic-brand]: https://brandfolder.com/anthropic/

[google-icons]: https://cloud.google.com/icons

[model-studio]: https://www.alibabacloud.com/en/product/modelstudio

[moonshot-platform]: https://platform.moonshot.ai/

[siliconflow]: https://www.siliconflow.com/

[baidu-cloud]: https://intl.cloud.baidu.com/en

[sensevoice]: https://github.com/QwenAudio/SenseVoice
