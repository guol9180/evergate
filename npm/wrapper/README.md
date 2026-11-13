# EverGate

Self-hosted OpenAI-compatible LLM gateway: one local endpoint for multiple providers, with key-pool failover and zero-buffer SSE streaming.

```bash
npm i -g evergate
evergate
```

The platform binary is installed automatically through optional dependencies:

- `evergate-win32-x64`
- `evergate-linux-amd64`
- `evergate-darwin-arm64`

After startup, open `http://localhost:8787/` for the admin console. Copy `evergate.yml.example` from the repository, or let the first run generate a config. Data lives in SQLite (`./data/evergate.db` by default).

Documentation: https://github.com/guol9180/evergate

MIT © guol9180
