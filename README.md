# Otis

Otis is a continuous code-quality agent. A supervisor runs review passes against
configured repositories, reads a filesystem body of knowledge, stores structured
findings, and exposes them through HTTP(S), CLI commands, and an MCP bridge.

Implemented behavior and operating contracts are summarized in [docs/current/](docs/current/). The runnable example lives in [docs/example/](docs/example/).

## Build

```bash
make build
make test
```

`make build` runs `go install $(TARGETS)`. `make test` runs `go vet ./...` and
`go test ./... -count=1`.

For the walkthrough below, install the CLI once and make sure your Go install
directory is on `PATH`:

```bash
go install ./...
```

## Demo

The checked-in demo uses the deterministic dummy reviewer so it does not require Codex, Claude Code, Pi, or Mattermost credentials.

Prepare a scratch demo directory:

```bash
DEMO=/tmp/otis-demo
rm -rf "$DEMO"
mkdir -p "$DEMO"
cp -R docs/example/. "$DEMO"/
git -C "$DEMO/testproj" init
git -C "$DEMO/testproj" add .
git -C "$DEMO/testproj" -c user.name="Otis Demo" -c user.email="otis@example.com" -c commit.gpgsign=false commit -m "seed demo project"
```

Start the supervisor in one terminal:

```bash
otis --config "$DEMO/global.yaml" serve
```

In another terminal, issue a workstation token and write a client config:

```bash
TOKEN=$(otis --config "$DEMO/global.yaml" admin token issue --label demo)
sed "s/replace-with-admin-token/$TOKEN/" "$DEMO/client.yaml.example" > "$DEMO/client.yaml"
```

Force one pass and inspect the resulting finding:

```bash
otis --client-config "$DEMO/client.yaml" pass run testproj/vocabulary-sweep
sleep 2
otis --client-config "$DEMO/client.yaml" findings list --project testproj --open
cat "$DEMO/.state/projects/testproj/backlog.md"
```

The dummy reviewer emits one finding:

```text
testproj/vocabulary-sweep/0001 high open view vocabulary drifts from lens convention
```

Accept it through the CLI:

```bash
otis --client-config "$DEMO/client.yaml" accept testproj/vocabulary-sweep/0001 --note "accepted demo finding"
otis --client-config "$DEMO/client.yaml" findings list --project testproj --open
```

The open list should now print `no findings`. To simulate the reviewer honoring
that accepted prior context on the next run, make the dummy reviewer quiet and
force the pass again:

```bash
printf '{"findings":[]}\n' > "$DEMO/dummy-output.json"
otis --client-config "$DEMO/client.yaml" pass run testproj/vocabulary-sweep
sleep 2
otis --client-config "$DEMO/client.yaml" findings list --project testproj --open
PROMPT=$(find "$DEMO/.state/projects/testproj/runs" -name prompt.md | sort | tail -1)
grep -n "accepted demo finding" "$PROMPT"
```

That last grep proves the next pass included the accepted finding and human note
in prior context, while the open findings list stayed empty.

## Useful Commands

```bash
otis version
otis config check docs/example/global.yaml
otis bok list --bok-path docs/example/bok
otis bok resolve --bok-path docs/example/bok --include vocabulary/,layering/,cognitive-load/ --project testproj
otis --client-config "$DEMO/client.yaml" projects list
otis --client-config "$DEMO/client.yaml" passes list --project testproj
```

For MCP clients, use [docs/example/mcp.json](docs/example/mcp.json) and point the
bridge at a client config like the demo `client.yaml`.
