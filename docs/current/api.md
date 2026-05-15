# API, CLI, and MCP

## HTTPS API

The supervisor serves HTTPS and requires bearer tokens issued with
`otis admin token issue`.

Implemented endpoints:

- `GET /api/v1/projects`
- `GET /api/v1/projects/{project}/passes`
- `GET /api/v1/projects/{project}/findings`
- `GET /api/v1/projects/{project}/findings/{pass}/{seq}`
- `POST /api/v1/projects/{project}/findings/{pass}/{seq}/disposition`
- `GET /api/v1/projects/{project}/runs/{pass}/{date}/{time_seq}/report`
- `POST /api/v1/projects/{project}/passes/{pass}/run`

The findings list endpoint accepts `pass`, `disposition`, and `open=true` query
parameters.

## CLI

The same `otis` binary runs supervisor and workstation commands.

Supervisor-side commands:

- `otis serve`
- `otis serve --once`
- `otis config check <path>`
- `otis admin token issue --label <label>`
- `otis bok list --bok-path <path>`
- `otis bok resolve --bok-path <path> --include <csv> --project <name>`

Workstation commands use `--client-config` and call the HTTPS API:

- `otis projects list`
- `otis passes list --project <project>`
- `otis pass run <project>/<pass>`
- `otis findings list --project <project> [--open]`
- `otis findings show <project>/<pass>/<NNNN>`
- `otis report show <project>/<pass>/<YYYY-MM-DD>/<HHMMSSZ-NNN>`
- `otis accept|defer|reject <finding-id> --note <note>`

Client config is YAML:

```yaml
url: https://127.0.0.1:8443
token: your-issued-token
tls:
  ca_cert: ./tls/cert.pem
```

## MCP

`otis mcp` runs a stdio MCP bridge that forwards to the supervisor through the
same workstation client config.

Implemented tools:

- `otis_list_findings`
- `otis_get_finding`
- `otis_get_report`
- `otis_disposition_finding`

See `docs/example/mcp.json` for a client configuration example.
