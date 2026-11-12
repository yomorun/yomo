# Model Profiles

Model profiles adapt a `UnifiedRequest` for documented, model-specific request behavior before it is sent upstream.

The registry selects a profile by the exact `ModelBinding.public_model`. A profile may inspect `upstream`, `upstream_model`, and `dialect`. It returns a dialect request when it handles the binding, or `None` to use Yomo's default conversion.

## Guidelines

- Add a profile only for evidence-backed behavior specific to a model. Put API-, dialect-, or upstream-wide behavior in its corresponding layer; do not infer capabilities or add empty profiles.
- Keep one model's behavior in one model-named file. Register exact public model names; aliases may share an implementation when their behavior is identical.
- Models whose only behavior is effort normalization declare a shared `EffortProfile` constant instead of duplicating the conversion.
- Check the binding's upstream and dialect. Return `None` for bindings the profile does not handle.
- Prefer the default dialect converter and make the smallest necessary changes. Preserve unrelated fields, extensions, and caller intent; do not inject unspecified options without a documented reason.
- Report documented constraints clearly. Do not silently discard request semantics.
- Keep routing, authentication, transport, and response parsing outside model profiles.

## Reasoning effort policy

Profiles normalize `reasoning_effort` only for models with a documented supported
set. Levels are ordered
`none < minimal < low < medium < high < xhigh < max`, and normalization follows one
rule:

- A supported named level passes through unchanged.
- Any other named level is rewritten to the nearest supported level.
- Ties go to the higher level, so normalization never silently weakens reasoning
  quality; the cost of a tie is at most one level.
- Provider-specific strings and numeric values pass through unchanged for the
  upstream to validate.
- An omitted effort stays omitted; profiles never inject one.
- A model that does not accept `reasoning_effort` at all reports an error instead
  of being silently adjusted.

Derive supported sets from the model's documentation; do not infer a set from a
sibling model. Keep exceptions documented next to the profile and in this table.

| Public model | Supported levels | Notes |
| --- | --- | --- |
| `gpt-5.1-codex` | none, low, medium, high | |
| `gpt-5.1-codex-max` | none, low, medium, high, xhigh | |
| `gpt-5.2-codex`, `gpt-5.3-codex` | low, medium, high, xhigh | |
| `gpt-5.4`, `gpt-5.4-mini`, `gpt-5.4-nano`, `gpt-5.5` | none, low, medium, high, xhigh | |
| `gpt-5.6-sol`, `gpt-5.6-luna`, `gpt-5.6-terra`, `gpt-6-sol`, `gpt-6-luna`, `gpt-6.1-terra` | none, low, medium, high, xhigh, max | |
| `gpt-6-astra`, `gpt-6.1-sol` | low, medium, high, xhigh, max | |
| `glm-5.2` | none, high, max | |
| `glm-5.3`, `glm-5.3-flash` | low, high, max | Always-on thinking; `glm-5.3` removed `top_k` |
| `deepseek-v4-flash`, `deepseek-v4-pro`, `deepseek-v4-pro-0813` | none, low, high, max | |
| `deepseek-v4.1-flash` | low, high, max | `ultra` is an alias for `max` |
| `deepseek-v4-flash-vision-exp` | none, low, high, max | `none` is encoded as `thinking.type=disabled` |
| `kimi-k2.6` | — | Rejects `reasoning_effort`; use `thinking` |
| `kimi-k3`, `kimi-k2.8-preview`, `minimax-m3`, `minimax-m2.7` | not documented | Forwarded unchanged |

## Tests

Test exact-name registration, applicable bindings, fallback for other bindings, the model-specific transformation, and preservation of unrelated or omitted options.
