# EverGate

Self-hosted OpenAI-compatible LLM gateway: one local endpoint for multiple providers, with key-pool failover and zero-buffer SSE streaming.

```bash
npm i -g @heguoliang/evergate
evergate
```

The platform binary is installed automatically through optional dependencies:

- `@heguoliang/evergate-win32-x64`
- `@heguoliang/evergate-linux-amd64`
- `@heguoliang/evergate-darwin-arm64`

After startup, open `http://localhost:8787/` for the admin console. Config and data use platform user directories by default; run `evergate -config <path>` for a portable config.

npm is an independent installation channel. Update it with `npm i -g @heguoliang/evergate@latest`; the standard Windows/macOS installer updater does not replace npm-installed files.

Documentation: https://github.com/guol9180/evergate

MIT © guol9180
