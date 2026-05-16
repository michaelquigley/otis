# Otis

Otis is a continuous code-quality agent. A supervisor runs review passes
against configured repositories, reads a body of knowledge, stores structured findings, and exposes them through HTTP(S), CLI commands, and an MCP bridge.

**Start here:** the [user guide](docs/current/guide/) walks you through the mental model, the demo, and a real configuration and deployment.

- [User guide](docs/current/guide/) — friendly, multi-page walkthrough.
- [Reference](docs/current/) — operating contracts and invariants.
- [Runnable example](docs/example/) — the BoK, project, and config used
  in the quickstart.

## Build

```bash
make build
make test
```

`make build` runs `go install $(TARGETS)`. `make test` runs `go vet
./...` and `go test ./... -count=1`. To install the CLI on your PATH:

```bash
go install ./...
```

## Quickstart

The checked-in demo uses the deterministic dummy reviewer, so no LLM
credentials are required. See [docs/current/guide/02-quickstart.md](docs/current/guide/02-quickstart.md) for the explained walkthrough; the short version is:

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

In another terminal, issue a workstation token, force a pass, and inspect
the result:

```bash
TOKEN=$(otis --config "$DEMO/global.yaml" admin token issue --label demo)
sed "s/replace-with-admin-token/$TOKEN/" "$DEMO/client.yaml.example" > "$DEMO/client.yaml"

otis --client-config "$DEMO/client.yaml" pass run testproj/vocabulary-sweep
sleep 2
otis --client-config "$DEMO/client.yaml" findings list --project testproj --open
```

You should see one finding from the dummy reviewer:

```text
testproj/vocabulary-sweep/0001 high open view vocabulary drifts from lens convention
```

The full demo (triage, prior-context replay, MCP bridge) is in
[docs/current/guide/02-quickstart.md](docs/current/guide/02-quickstart.md).
