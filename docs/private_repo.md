

## How to go get private repo

### dependency
`yomo-framework` is a private repo
- `yomo` dependent `yomo-framework`
- `yomo-plugin-echo` dependent `yomo-framework`

### steps
- Generate GITHUB_TOKEN here https://github.com/settings/tokens
- export GITHUB_TOKEN=xxx
- git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/10cella".insteadOf "https://github.com/10cella"
- go env -w GOPRIVATE=github.com/yomorun/yomo


