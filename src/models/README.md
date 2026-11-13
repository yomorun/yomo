# Model Profiles

Model profiles adapt a `UnifiedRequest` for documented, model-specific request
behavior before it is sent upstream. The registry selects a profile by the exact
`ModelBinding.public_model`; a profile may inspect `upstream`, `upstream_model`,
and `dialect`, and returns `None` or a no-op when it does not handle the binding.

## Where model-specific policy lives

- **Per-model policy is in the model file.** Every `src/models/<model>.rs` starts
  with a `//!` header that records its official source, binding scope, supported
  reasoning controls, sampling rules, and any model-specific validation.
- **This document defines the general policy and the process.** It intentionally
  does not enumerate each model's supported values.
- **The registry is `src/models/mod.rs`.** Register an exact public model name
  there; aliases are separate registrations, not wildcard matches.

## Selection and scope

- Register profiles by exact public model name. A profile must not claim a
  binding it does not handle: return `None` for conversion hooks and do nothing
  in adaptation hooks.
- Check `binding.upstream` and `binding.dialect` before applying rules.
- A profile may share an implementation with an alias only when both names have
  identical behavior; otherwise use one model-named file per model.
- Keep routing, authentication, transport, HTTP diagnostics, and response
  parsing outside model profiles.

## Reasoning effort policy

Levels are ordered `none < minimal < low < medium < high < xhigh < max`.
Normalization follows one rule:

- A supported named level passes through unchanged.
- Any other named level is rewritten to the nearest supported level.
- Ties go to the higher level, so normalization never silently weakens reasoning
  quality; the cost of a tie is at most one level.
- Provider-specific strings and numeric values pass through unchanged for the
  upstream to validate, unless the target dialect cannot represent them; in that
  case report an error instead of guessing.
- An omitted effort stays omitted; profiles never inject one.
- A model that does not accept `reasoning_effort` at all reports an error
  instead of being silently adjusted.

Every rewrite is logged at `debug` with the model and the requested/forwarded
values so cost or quality anomalies can be traced without changing the response.

## Thinking, sampling, and validation policy

- Normalize `thinking.type` only when the target control has a documented
  equivalent. For example, a mode that is unsupported but has a documented
  replacement may be converted; a mode with no equivalent must report an error.
- Do not silently drop request semantics. When the target dialect or model
  cannot represent a control, prefer a clear `InvalidRequest` or
  `UnsupportedCapability` error over guessing.
- Remove a sampling parameter only when the model documentation says it is
  ignored, deprecated, or no longer supported. Log the removal at `debug`.
- When thinking is active and a model requires a fixed sampling value, set that
  value explicitly rather than forwarding an incompatible client value.
- Validate model-specific conversation constraints (for example, a rejected
  prefill or an invalid tool choice) before encoding the upstream request.
- Derive supported sets from the model's documentation; never infer a set from a
  sibling model.
- Prefer the default dialect converter and make the smallest necessary change.
  Preserve unrelated fields, extensions, and caller intent.

## Adding a model profile

1. **Collect evidence.** Find the official model documentation, including
   supported thinking modes, effort levels, sampling rules, tool-choice limits,
   and conversation constraints. Record the URL in the model file header.
2. **Create one model-named file.** Use `src/models/<model>.rs`; do not add a
   family-wide file unless a documented shared behavior genuinely applies.
3. **Write the `//!` policy header** using the template below. This header is
   the normative per-model policy; do not duplicate it in this README.
4. **Implement the smallest hook.** Use `to_openai_chat_request`,
   `to_openai_responses_request`, `to_bedrock_converse_request`,
   `adapt_bedrock_request`, or `adapt_gemini_generate_content_request` as
   appropriate. Models whose only behavior is effort normalization should use
   the shared `EffortProfile` constant instead of duplicating conversion code.
5. **Scope the binding.** Check `public_model`, `upstream`, and `dialect`; return
   `None`/no-op for foreign bindings so the default converter still works.
6. **Register the exact public name** in `src/models/mod.rs`.
7. **Add tests** covering exact-name registration, binding scope, the
   model-specific transformation, fallback for other bindings, and preservation
   of omitted or unrelated options. Do not use mocked providers.
8. **Run `cargo fmt`, `cargo check --all-targets`, and `cargo test --lib`.**

## Profile skeleton

```rust
//! <Model> request behavior.
//!
//! Source: <https://official.example/model>
//!
//! Binding: <upstream> / <dialect>, exact public model `<model>`.
//! Reasoning effort: <supported levels or "not documented">; <normalization>.
//! Thinking: <supported modes and constraints>.
//! Sampling: <supported/removed/forced values>.
//! Other: <tool choice, prefill, output limits, media limits>.

use crate::models::{ModelProfile, ModelProfileError};
use crate::provider::UnifiedRequest;
use crate::serve_config::ModelBinding;

use super::profile_support::/* shared helpers */;

/// <Model> profile for its exact public model name.
pub struct ModelProfileImpl;

impl ModelProfile for ModelProfileImpl {
    fn adapt_bedrock_request(
        &self,
        request: &mut UnifiedRequest,
        binding: &ModelBinding,
    ) -> Result<(), ModelProfileError> {
        // Check binding first; return Ok(()) for a foreign binding.
        todo!()
    }
}
```

## Tests

Test exact-name registration, applicable bindings, fallback for other bindings,
the model-specific transformation, and preservation of unrelated or omitted
options. Keep transformation tests table-driven when they cover a matrix of
levels or modes.

## Anti-patterns

- Do not add an empty profile for a model with no documented special behavior.
- Do not build a model capability table in this README or infer support from a
  model family.
- Do not handle routing, authentication, transport, headers, or response
  parsing in a profile.
- Do not silently discard a client control that the target cannot represent.
- Do not rely on a sibling model's rules when the model documentation differs.

## Definition of done

A profile change is complete when:

- the model file has a source-backed `//!` policy header;
- the exact public name is registered in `src/models/mod.rs`;
- the profile no-ops for foreign bindings;
- unit tests cover registration, binding scope, transformation, fallback, and
  omitted controls;
- `cargo fmt`, `cargo check --all-targets`, and `cargo test --lib` pass.
