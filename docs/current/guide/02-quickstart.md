# 2. Quickstart

This chapter takes the demo from the README and explains what each step is
doing. By the end you will have run the full loop — supervisor, pass,
finding, triage, prior-context replay — without needing a real reviewer
CLI. Every command works as written; the dummy reviewer is deterministic.

## Install Otis

```bash
cd /path/to/otis
go install ./...
```

Make sure your Go install directory (usually `~/go/bin`) is on `PATH`.
Confirm:

```bash
otis version
```

## Stage a Scratch Demo Directory

We copy the runnable example out of the repo so the demo can write state
without touching `docs/example/`. Pick any directory you like; the guide
uses `/tmp/otis-demo`.

```bash
DEMO=/tmp/otis-demo
rm -rf "$DEMO"
mkdir -p "$DEMO"
cp -R docs/example/. "$DEMO"/
```

The example carries a small Go project (`testproj/`), a BoK with three
category subtrees, a shared `standard.yaml` profile, and the per-project
`projects/testproj/otis.yaml`. See [`docs/example/`](../../example/) if you
want to look at the source.

Otis needs the supervised project to be a real git repo (it snapshots
`HEAD` into a worktree), so initialize one inside the demo:

```bash
git -C "$DEMO/testproj" init
git -C "$DEMO/testproj" add .
git -C "$DEMO/testproj" \
  -c user.name="Otis Demo" \
  -c user.email="otis@example.com" \
  -c commit.gpgsign=false \
  commit -m "seed demo project"
```

## Start the Supervisor

In one terminal:

```bash
otis --config "$DEMO/global.yaml" serve
```

This loads `global.yaml`, opens the state store at `$DEMO/.state`, prunes
any stale Otis worktrees, starts the HTTP API on `127.0.0.1:8443`, and
begins ticking the scheduler. Leave it running.

The default demo config uses plain HTTP because there is no TLS pair
configured. See [06-deployment.md](06-deployment.md) for HTTPS and
reverse-proxy options.

## Issue a Workstation Token

The supervisor authenticates every API request with a bearer token. In a
second terminal, issue one and write a workstation client config:

```bash
TOKEN=$(otis --config "$DEMO/global.yaml" admin token issue --label demo)
sed "s/replace-with-admin-token/$TOKEN/" "$DEMO/client.yaml.example" \
  > "$DEMO/client.yaml"
```

The resulting `client.yaml` looks like:

```yaml
url: http://127.0.0.1:8443
token: <the token you just issued>
```

Workstation commands take `--client-config` and call the API; they never
open the state directory directly. That separation is what lets the same
binary act as supervisor and workstation.

## Force a Pass and Inspect the Finding

The demo's `vocabulary-sweep` pass has a 24-hour cadence and the `manual`
window, so it will not fire on its own. Force it:

```bash
otis --client-config "$DEMO/client.yaml" pass run testproj/vocabulary-sweep
sleep 2
otis --client-config "$DEMO/client.yaml" findings list --project testproj --open
cat "$DEMO/.state/projects/testproj/backlog.md"
```

The dummy reviewer reads `$DEMO/dummy-output.json` and emits exactly one
finding. The CLI should print:

```text
testproj/vocabulary-sweep/0001 high open view vocabulary drifts from lens convention
```

`backlog.md` is regenerated from that finding. Open the run directory if
you want to see the audit trail:

```bash
ls "$DEMO/.state/projects/testproj/runs"
```

You will find a `<date>/vocabulary-sweep/<time-seq>/` directory holding
`prompt.md` (the full reviewer prompt), `output.json` (the reviewer's raw
response), `findings.json` (the normalized findings for this run),
`report.md`, and `git-head.txt` (the SHA the worktree was pinned to).

## Triage the Finding

Accept it through the CLI:

```bash
otis --client-config "$DEMO/client.yaml" \
  accept testproj/vocabulary-sweep/0001 \
  --note "accepted demo finding"

otis --client-config "$DEMO/client.yaml" findings list --project testproj --open
```

The open list should print `no findings`. The finding has not been
deleted — it is still on disk in
`projects/testproj/findings/testproj--vocabulary-sweep--0001.json` and a
`finding_accepted` event has been appended to
`projects/testproj/dispositions.jsonl`. The backlog renders only the open
ones, so it is empty now.

## Watch Prior Context Flow Into the Next Pass

Make the dummy reviewer quiet on the next run, force the pass again, and
look at the prompt Otis built:

```bash
printf '{"findings":[]}\n' > "$DEMO/dummy-output.json"
otis --client-config "$DEMO/client.yaml" pass run testproj/vocabulary-sweep
sleep 2
otis --client-config "$DEMO/client.yaml" findings list --project testproj --open

PROMPT=$(find "$DEMO/.state/projects/testproj/runs" -name prompt.md | sort | tail -1)
grep -n "accepted demo finding" "$PROMPT"
```

The grep should hit. That proves the accepted finding and your triage note
were included in the next pass's prior-context section, even though they
no longer appear on the backlog. This is the mechanism that lets Otis stop
re-raising things you have already decided.

## Stop the Supervisor

Back in the first terminal, `Ctrl+C` ends `otis serve`. State is durable
on disk; restarting picks up where you left off.

## What Just Happened

You ran a supervisor, scheduled and force-ran a pass against a snapshotted
git worktree, invoked a (deterministic) reviewer, validated structured
findings, wrote immutable run artifacts and a live backlog, triaged a
finding through the API, and watched prior context flow into the next
pass. Every step generalizes to real projects and real reviewers; the
remaining chapters show you how to wire those up.

Next: [03-body-of-knowledge.md](03-body-of-knowledge.md) — write the BoK
that anchors what Otis is actually reviewing for.
