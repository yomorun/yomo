# Model Profiles

Model profiles adapt a `UnifiedRequest` for documented, model-specific request behavior before it is sent upstream.

The registry selects a profile by the exact `ModelBinding.public_model`. A profile may inspect `upstream`, `upstream_model`, and `dialect`. It returns a dialect request when it handles the binding, or `None` to use Yomo's default conversion.

## Guidelines

- Add a profile only for evidence-backed behavior specific to a model. Put API-, dialect-, or upstream-wide behavior in its corresponding layer; do not infer capabilities or add empty profiles.
- Keep one model's behavior in one model-named file. Register exact public model names; aliases may share an implementation when their behavior is identical.
- Check the binding's upstream and dialect. Return `None` for bindings the profile does not handle.
- Prefer the default dialect converter and make the smallest necessary changes. Preserve unrelated fields, extensions, and caller intent; do not inject unspecified options without a documented reason.
- Report documented constraints clearly. Do not silently discard request semantics.
- Keep routing, authentication, transport, and response parsing outside model profiles.

## Tests

Test exact-name registration, applicable bindings, fallback for other bindings, the model-specific transformation, and preservation of unrelated or omitted options.
