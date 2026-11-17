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

After startup, open `http://localhost:8787/` for the admin console. Config and data use platform user directories by default; run `evergate -config <path>` for a portable config. On Windows the npm command detaches the GUI process and returns the terminal immediately; use the `evergate-windows-amd64-console.exe` Release asset for terminal diagnostics.

Documentation: https://github.com/guol9180/evergate

MIT © guol9180
