# nats-tui

A beautiful, keyboard-first terminal workspace for NATS JetStream.

See [FEATURES.md](FEATURES.md) for the live feature-parity roadmap and checklist.

The current vertical slice presents an interactive connection flow, connects to
NATS without requiring authentication, loads a real JetStream account snapshot,
lists streams and consumers, browses messages, watches live traffic, and explores
the configured subject hierarchy.
NUI is available locally
under .reference/nui for behavioral reference and is intentionally excluded
from this project's source and dependency graph.

## Run locally

Requirements:

- Go 1.26+
- Docker with Compose

    make dev-up
    make run

The startup flow is:

1. Product presentation.
2. Editable NATS server URL.
3. Visible connection/loading state.
4. Navigable workspace.

Saved connection profiles appear on the startup screen ordered by recent use;
direct URL entry without authentication remains available with `n`. Profiles
support multiple failover URLs; no authentication, token, credentials-file,
username/password, NKey, or user JWT authentication; and CA/client-certificate
TLS controls. Local state is written atomically with mode `0600`, seeds and
credentials are resolved only when needed, and list/export output never includes
secrets. Credentials embedded in server URLs are rejected.

    nats-tui profile save local-events --server-url nats://127.0.0.1:4222
    nats-tui profile save production \
      --server-url nats://a:4222,nats://b:4222 \
      --auth-user ops --auth-password env://NATS_PASSWORD
    nats-tui profile list
    nats-tui profile test production
    nats-tui --profile production stream list
    nats-tui profile export > profiles.sanitized.json
    nats-tui profile save jwt-prod \
      --server-url tls://nats.internal:4222 \
      --user-jwt ~/.config/nats/user.jwt --user-seed ~/.config/nats/user.seed \
      --tls-ca ~/.config/nats/ca.pem --tls-server-name nats.internal
    nats-tui profile import-nats

For reusable secrets, prefer resolver references such as
`--auth-token env://NATS_TOKEN` or `--auth-password env://NATS_PASSWORD` so the
value does not enter the profile file or sanitized output.

NATS CLI context imports read the official JSON documents from the user config
`nats/context` directory by default and preserve `file://`, `env://`, and embedded
base64 credential references. Existing nats-tui profiles are skipped unless
`--replace` is explicit. `--tls-insecure` is available for diagnostics and is
rendered with a warning marker in the startup selector; it should not be used in
production. External `op://` and `nsc://` resolvers are reported explicitly and
must first be exported to a file or environment reference.

Use arrow keys, h/l, tab, `1-9`, `0`, `A` (accounts), or `!` (alerts) to move between sections. Press c to return to
the connection screen, ? for help, and q to quit from the workspace.

In Streams, use j/k or the arrow keys to select a stream and Enter to load its
detail; press m to load its 50 most recent messages. Select a message and press
Enter to open the formatted
payload; scroll with j/k or the arrow keys and return with Escape.
Long message lists keep the selected row inside a real viewport. Page Up/Page
Down jump through the list, while g/G move to the first or last message. Wide
terminals show a synchronized list-and-preview split; Enter opens the payload
using the full content area.

Press / in the message monitor to search reactively across subjects, payloads,
headers, and sequences. Multiple terms are combined, Escape leaves the current
filter applied, and Ctrl+U clears it. Press w to start or stop Live Watch for the
selected stream and Space to pause or resume the UI feed. Live events are
coalesced into short batches and retained in a bounded 2,000-message buffer.
Press o before starting a watch to use a gap-detecting ordered JetStream
consumer instead of a Core NATS subscription; ordered mode includes the real
stream sequence and automatically recovers its ephemeral consumer on gaps.

The Consumers workspace provides an account-wide lag dashboard. It combines
undelivered and unacknowledged work into lag, shows active, idle, lagging, and
paused states, keeps selection inside a responsive viewport, and displays a
synchronized detail pane on wide terminals. Enter opens the compact full detail.
Press n to create a consumer and e to edit one. The centered responsive form
covers pull/push and durable/ephemeral modes, delivery and acknowledgement
policies, start sequence/time, filters, retry backoff, pull limits, flow control,
heartbeats, storage, and metadata. Each field includes contextual guidance and
updates preserve the complete server configuration.
Press p to pause or resume the selected consumer. Reset with x and delete with d;
both require an explicit y confirmation and can be cancelled with n or Escape.
Reset returns delivery to `ack_floor + 1`. Mutations have a five-second timeout,
ignore responses from old connections, and force an immediate refresh. The
detail pane keeps a bounded lag sparkline, calculates delivery throughput from
server observations, and surfaces recent max-delivery/termination advisories.

Consumer management is also scriptable. Omitted edit flags preserve their
existing values:

    nats-tui consumer list --output json
    nats-tui consumer create EVENTS worker \
      --filter 'events.us.*' --filter 'events.eu.*' \
      --ack-policy explicit --ack-wait 30s --max-deliver 5 \
      --backoff 1s,5s,30s --metadata team=payments
    nats-tui consumer create EVENTS push-worker \
      --deliver deliver.events --deliver-group workers \
      --flow-control --heartbeat 5s
    nats-tui consumer edit EVENTS worker --max-ack-pending 500
    nats-tui consumer delete EVENTS worker --yes

The Dashboard combines JetStream usage with cluster runtime telemetry. It shows
message and byte rates, bounded local sparklines, memory/file storage, account
limits and utilization, CPU/RAM/uptime/traffic, and connection health. Runtime
metrics use `$SYS` server stats when authorized and fall back to `--monitor-url`;
ordinary accounts degrade cleanly when neither source is available. Goroutines
are shown when the selected monitoring source publishes that optional field.

Custom panels are persistent per connection profile and appear below the
built-in dashboard. Available metrics and panel configuration are scriptable:

    nats-tui dashboard metrics
    nats-tui dashboard panel add traffic message-rate --title 'Message traffic'
    nats-tui dashboard panel add server-cpu cpu
    nats-tui dashboard panel
    nats-tui dashboard panel delete server-cpu --yes

Dashboard, stream, and consumer state refresh automatically every three seconds.
Press r to refresh immediately. Refresh calls have a three-second timeout and do
not overlap. Failures retain the last good snapshot, show `Stale · retrying`, and
retry after two seconds. Results from an old connection generation are ignored.

The Streams workspace supports full inspection and safe management. Enter opens
stream detail, m opens its message monitor, n creates a stream, e edits the
selected stream, and d opens destructive delete confirmation. Forms expose name,
description, subjects, storage, retention, limits, discard/compression, placement,
metadata, transforms, republish, mirrors, sources, and safety controls. The
centered form scrolls around the active field; navigate with Tab, change choices
with arrow keys, and save with Ctrl+S. Irreversible sealing requires a second
Ctrl+S confirmation. During updates, immutable name/storage fields are locked
and source details that were not changed are preserved.
Stream detail also renders cluster leadership, replica current/offline state,
peer lag, and a compact upstream dependency chain with mirror/source lag.

Press `S` to back up the selected file-backed stream into the standard NATS
`backup.json` + `stream.tar.s2` directory format. The centered form explains
consumer-state inclusion, checksum verification, and safe replacement. Press
`R` to restore a backup; restore remains available when the account has no
streams and requires an explicit confirmation. Both directions stream data
without loading the archive into memory, show bytes/chunks/speed, and can be
canceled with Escape. Local backups are built in a private staging directory
and become visible only after both files are synced successfully.

Press `P` on a stream to open guided purge. It supports the whole stream, a
subject filter, a sequence boundary, or keeping the newest N messages and always
requires a second confirmation. Advanced stream operations are equally
scriptable:

    nats-tui stream create ORDERS \
      --subject 'orders.>' --max-age 168h --max-messages-per-subject 10000 \
      --discard old --compression --allow-direct \
      --placement-tag ssd --metadata owner=platform

    nats-tui stream create ORDERS_EU \
      --mirror ORDERS --mirror-filter 'orders.eu.>' \
      --mirror-start-sequence 42 --mirror-direct

    nats-tui stream create ALL_ORDERS \
      --source-config '{"name":"ORDERS_EU","filter_subject":"orders.eu.>"}' \
      --source-config '{"name":"ORDERS_US","filter_subject":"orders.us.>"}'

    nats-tui stream edit ORDERS --max-age 336h --compression=false
    nats-tui stream purge ORDERS --subject 'orders.expired.>' --keep 10 --yes
    nats-tui stream delete-message ORDERS 421 --secure --yes
    nats-tui stream export ORDERS
    nats-tui stream backup ORDERS ./backups/ORDERS --consumers --check
    nats-tui stream restore ORDERS ./backups/ORDERS --yes

These backups are interoperable with the official `nats stream backup` and
`nats stream restore` commands. Progress is emitted on stderr, leaving stdout
safe for scripts. Existing backup directories are preserved unless `--force`
is passed; the target stream must not exist during restore.

The Subject Explorer merges configured patterns with live application subjects.
Its rolling ten-second projection shows message/byte rates, hierarchy heat,
owning streams, attached consumers, and visible managed-user publish/subscribe
rules. NATS control-plane and request-inbox traffic is excluded from the live
projection so operational noise does not bury application subjects. Reactive `/`
search covers subjects and owners. Press Enter or w for Live Watch, p for a
prefilled publish, or R for request/reply; configured wildcards are preserved for
watching and converted into a concrete-subject prompt for publishing.

The same static ownership view and contextual operations are scriptable:

    nats-tui subject list --output table
    nats-tui subject inspect 'events.payment.>'
    nats-tui subject publish events.payment.created '{"id":42}'
    nats-tui subject request rpc.lookup '{"id":42}' --timeout 2s

The Key-Value workspace lists real JetStream KV buckets with their value count,
size, storage medium, replicas, retained history, TTL, and compression state.
Selection remains stable while the workspace refreshes. Press n to create a
bucket interactively, or use the scriptable CLI:

    nats-tui kv create SESSIONS \
      --storage memory \
      --history 5 \
      --ttl 1h \
      --replicas 1

Create replicated read models with `--mirror PRIMARY`, or aggregate buckets
with repeatable `--source REGION_A --source REGION_B`. Mirror and sources are
mutually exclusive and are shown in the TUI bucket detail.

Press e on a selected bucket to edit its mutable settings. Name and storage are
kept immutable, and advanced settings not exposed by the form are preserved.
For automation, flags omitted from `kv edit` remain unchanged:

    nats-tui kv edit SESSIONS \
      --history 10 \
      --ttl 2h \
      --compression=false

Delete requires an impact confirmation with d then y in the TUI. Scripts must
opt in explicitly:

    nats-tui kv delete SESSIONS --yes

Opened values support `C` to copy, `W` to start/stop a live key watch, and `H`
for the revision timeline. From a retained PUT revision, `R` restores that value
as a new latest revision. Equivalent scriptable operations include:

    nats-tui kv watch SESSIONS '>'
    nats-tui kv restore SESSIONS session.42 --revision 7
    nats-tui kv export SESSIONS session.42 --output session.json

The Object Stores workspace loads real bucket status and presents storage,
replicas, total size, capacity, TTL, compression, metadata, and sealed/writable
state. From the bucket catalog, press `n` to create, `e` to edit mutable
settings, or `d` to review the impact and confirm permanent deletion. The
centered form explains each setting and keeps bucket name and storage immutable
during edits. Creation also supports a single mirror or multiple source buckets.
Mirrors are clearly marked read-only: browsing, searching, watching, inspecting,
and downloading remain available while mutations direct the operator back to
the source bucket. Press Enter to browse a bucket. `/` searches reactively across object names, descriptions,
digests, metadata, and headers; Enter opens a responsive, scrollable detail with
size, chunks, modification time, digest, metadata, and headers. Press `u` to
upload a local file with optional object name, description, metadata, and
headers. Press `D` on an object to download it. Both operations show progress,
average speed, elapsed time, and support cancellation with Escape. Downloads
are written to a private temporary file, SHA-256 verified, and only then made
visible at the destination; overwrite must be enabled explicitly.
Press `e` to open the centered metadata editor; it supports rename,
description, metadata, and headers without silently dropping untouched values.
Press `d` for a destructive-impact summary and explicit confirmation.
Press `W` to start or stop a live bucket watch. Uploads, metadata changes,
renames, and deletes update the catalog incrementally. `Space` pauses visual
updates while continuing to drain the subscription; resuming performs one
catalog reconciliation so no event is lost from the final view. The `LIVE` or
`PAUSED` badge includes the number of received mutations.
Press `L` to create a link inside the current bucket. Provide a target bucket
and optionally a target object; leaving the object empty creates a bucket link.
Links are marked with `↗`, are searchable by target, and expose their type and
destination in the detail panel.

The Topology workspace discovers every server visible to the connected NATS
system account. It shows identity, version, uptime, endpoints, tags, client
load, per-server JetStream memory/storage, routes, gateways, leaf nodes, and
the JetStream meta leader and peers. Use arrows to navigate, Enter for server
detail, and `g` to switch between the table and terminal graph. Link health is
derived from connection state and pending bytes.

Topology discovery prefers the NATS `$SYS` request API. For private servers
without a system account, configure `--monitor-url http://host:8222` (or
`NATS_TUI_MONITOR_URL`) to use the read-only HTTP monitoring endpoints as a
local-server fallback. Reload and step-down still require the corresponding
NATS permissions. The workspace reports missing capabilities explicitly. `R`
reloads the selected server and `S` steps down the JetStream meta leader; both
require an interactive confirmation. The same guarded operations are available
from the CLI:

    nats-tui --monitor-url http://127.0.0.1:8422 topology
    nats-tui topology --json
    nats-tui topology reload SERVER_ID --yes
    nats-tui topology step-down --yes

The Connections workspace is refreshed while visible and keeps a bounded
in-memory timeline of connection and subscription counts. `/` filters across
client name, user, IP, language, version, and server; `o` cycles sort fields,
`G` cycles grouping, and `f` isolates slow consumers. A client is flagged slow
when NATS reports stalls or at least 1 MiB of pending output. Enter opens traffic,
RTT, pending bytes, account, and subscription detail. `[` and `]` select a
subscription and `s` jumps to it in Subject Explorer. `x` closes the selected
connection only after confirmation and only when `$SYS` permissions allow it.

The connection read model is also available to scripts:

    nats-tui connections
    nats-tui connections --filter instructionrelay --sort traffic
    nats-tui connections --group server --slow
    nats-tui connections --json
    nats-tui connections close SERVER_ID CID --yes

    # Operator/system-account inspection. Also available in workspace "A".
    nats-tui accounts
    nats-tui accounts --json

    # Explicit secure authority workflows. The output directory must be private
    # and existing files are never overwritten.
    nats-tui accounts create ORDERS \
      --operator-seed ./operator.seed --output-dir ./private \
      --max-connections 100 --max-storage 10737418240

    nats-tui accounts user create worker \
      --account AACCOUNTPUBLICKEY --account-seed ./private/ORDERS.account.seed \
      --output-dir ./private --pub-allow 'orders.*' --sub-allow 'events.>'

    # Update/delete preserve advanced claims and archive removed local files.
    nats-tui accounts update AACCOUNTPUBLICKEY ORDERS-PROD \
      --operator-seed ./operator.seed --output-dir ./private \
      --max-connections 200 --max-streams 20
    nats-tui accounts delete AACCOUNTPUBLICKEY \
      --operator-seed ./operator.seed --output-dir ./private --yes

    # User edits retain the existing user NKey. Revoke updates the account JWT;
    # delete also moves the credentials into private/.trash.
    nats-tui accounts user update UUSERPUBLICKEY worker-v2 \
      --account AACCOUNTPUBLICKEY --output-dir ./private \
      --pub-allow 'orders.*' --sub-allow 'events.>'
    nats-tui accounts user revoke AACCOUNTPUBLICKEY UUSERPUBLICKEY \
      --operator-seed ./operator.seed --output-dir ./private --yes
    nats-tui accounts user delete AACCOUNTPUBLICKEY UUSERPUBLICKEY \
      --operator-seed ./operator.seed --output-dir ./private --yes

    # Regular account signing key.
    nats-tui accounts signing-key add AACCOUNTPUBLICKEY \
      --operator-seed ./operator.seed --output-dir ./private

    # Scoped signing key with an enforced user permission template.
    nats-tui accounts signing-key add AACCOUNTPUBLICKEY \
      --operator-seed ./operator.seed --output-dir ./private \
      --scoped --role orders-reader --sub-allow 'orders.>'

Configure `authority-dir` (and optionally `authority-operator-seed`) to merge
locally managed accounts/users into both the CLI and workspace `A`. Press `u`
inside that workspace to switch between accounts and managed users. NATS does
not expose a global user directory: only JWTs owned by this local authority can
be enumerated safely.

The Alerts workspace (`!`) is a persistent, profile-scoped incident inbox. It
subscribes to JetStream advisories, API audit events, consumer ACK metrics, and
local server/connection health evaluations. Use `/` to search, `v` to cycle
severity, Space to pause the visual feed, Enter for the full JSON payload, `a`
or `A` to acknowledge one/all alerts, `s` to scan health now, and `g` to jump to
the affected stream, consumer, server, or connection. Stable IDs deduplicate
replayed advisories, while malformed events never terminate the watch.

The same inbox and rules are automation-friendly:

    nats-tui alerts list --unacked --severity critical
    nats-tui alerts list --search ORDERS --output json
    nats-tui alerts watch --output ndjson
    nats-tui alerts ack ALERT_ID
    nats-tui alerts ack --all
    nats-tui alerts scan

    nats-tui alerts rules
    nats-tui alerts rules add max-delivery \
      --match '$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>' \
      --severity critical --cooldown 5m
    nats-tui alerts rules add high-memory \
      --metric server-memory --threshold 1073741824 \
      --severity warning --cooldown 10m

Live alerts can optionally be POSTed as JSON or passed to a local executable as
JSON on stdin. Executables receive literal argument vectors and never run
through a shell:

    nats-tui alerts watch --webhook https://ops.example/hooks/nats
    nats-tui alerts watch --notify-program ./notify \
      --notify-arg=--channel --notify-arg=nats-ops

Press `:` from the workspace to open the global command bar (`Ctrl+K` also
works, including inside the publish/request editor). Search is fuzzy across
commands and live streams/consumers. Results are grouped into Navigate,
Inspect, Create, Operate, and Custom actions. Mutating commands open a preview;
destructive actions cannot bypass it. Press `C` in preview to copy the exact
equivalent `nats` argv rendered for a shell, and `Ctrl+R` to open persistent
profile-scoped history. Replaying history only reopens the command or its
parameter form—it never repeats a mutation automatically.

Custom commands use declared parameters and typed internal actions. Templates
are expanded as data and never evaluated by a shell:

    nats-tui commands add watch-payments \
      --title 'Watch payment events' \
      --action navigate.messages \
      --parameter subject --required subject \
      --argument 'subject={{subject}}' \
      --equivalent-part nats --equivalent-part sub \
      --equivalent-part '{{subject}}'

    nats-tui commands preview watch-payments \
      --value 'subject=events.payment.>'
    nats-tui commands history --json
    nats-tui commands delete watch-payments --yes

Supported custom actions deliberately form a small allowlist:
`navigate.workspace`, `navigate.stream`, `navigate.consumer`,
`navigate.messages`, `form.stream.create`, `form.kv.create`,
`form.object.create`, `operate.alerts.scan`, `operate.core.publish`, and
`operate.core.request`. Unknown actions remain stored
and previewable but fail closed when invoked.

Bookmarks are global within the active connection profile. Select a stream,
consumer, subject, KV bucket/key, or object bucket/object and press `B` to add
or remove it. Press `b` from any workspace to open the responsive bookmarks
list; use Enter to jump and `d` to remove an entry. Bookmarks whose resources
were renamed or deleted remain visible as stale instead of disappearing. Child
resources are verified when their bucket is opened.

The same profile-scoped collection is scriptable, including explicit retargeting
after a resource rename and access to recent reactive searches:

    nats-tui bookmarks
    nats-tui bookmarks add consumer ORDERS PROCESSOR --label order-worker
    nats-tui bookmarks retarget BOOKMARK_ID ORDERS_V2 PROCESSOR
    nats-tui bookmarks delete BOOKMARK_ID --yes
    nats-tui bookmarks searches --scope messages

    nats-tui --authority-dir ./private \
      --authority-operator-seed ./operator.seed

With both options configured and system-account permissions available, press
`n` in the Accounts pane to open the centered account form. Secrets never enter
the Bubble Tea model; the form sends only a validated account definition.
In `Managed Users`, `n` opens the user form. It only offers accounts whose seed
is held by the configured authority and supports comma-separated pub/sub allow
and deny subjects, response max/TTL, and per-user subscription limits.
Press `e` to edit the selected account or user. Account edits preserve claim
sections and limits not exposed by the form; user edits retain the existing
NKey. In Accounts, `s` opens the regular/scoped signing-key form and its private
seed is never rendered. Press `d` to delete an account or user, or `x` to revoke
a user; every destructive action opens a centered confirmation. Removed local
artifacts are moved to the authority's mode-0700 `.trash` directory so an
operator can recover them after an accidental action.

The Publish workspace is a Core NATS message lab. Switch between publish and
request mode with Ctrl+R, edit repeatable headers, use a multiline payload, and
press Ctrl+F to validate and format JSON. Ctrl+L loads the configured payload
file without coupling Bubble Tea to the filesystem. Requests accept a timeout
and reply limit; replies appear in a responsive list/detail view with latency,
headers, syntax-colored JSON, and raw-text fallback.
The active field always shows a visible cursor and accepts digits normally,
including inside JSON payloads. Press Tab to move between fields and Escape to
return directly to the Dashboard; workspace number shortcuts resume outside
the composer.

Ctrl+H opens persistent request history and `R` replays the selected exchange.
Ctrl+T saves the current request as a named template; Ctrl+O lists templates,
where Enter loads one into the composer, `R` runs it, and `d` deletes it.
Generated shell-safe CLI equivalents are shown for publish and request flows
and can be copied with Ctrl+Y or `C` from result/history views.

The same workflows are scriptable:

    nats-tui publish events.payment.created '{"payment_id":"p-42"}' \
      -H Content-Type=application/json \
      -H X-Trace=demo

    nats-tui request rpc.payment.lookup '{"payment_id":"p-42"}' \
      --timeout 3s \
      --max-replies 5

    nats-tui request rpc.payment.lookup --file request.json
    nats-tui request history
    nats-tui request replay REQUEST_ID
    nats-tui request template save payment-lookup rpc.payment.lookup --file request.json
    nats-tui request template list
    nats-tui request template run payment-lookup
    nats-tui request template delete payment-lookup --yes

History, templates, bookmarks, and recent searches are written atomically with
private permissions beneath `$XDG_CONFIG_HOME/nats-tui` (or the platform config
directory equivalent). Files are separated by the sanitized profile name, so
local state from one NATS environment never appears in another. Request history
is bounded to the newest 100 exchanges; recent searches are deduplicated and
bounded per workspace.

Object Store operations are also scriptable. Progress is written to stderr so stdout
remains safe for pipelines:

    nats-tui object bucket create ARTIFACTS \
      --description "release files" \
      --storage file \
      --replicas 3 \
      --metadata owner=platform

    nats-tui object bucket create ARTIFACTS_MIRROR \
      --mirror ARTIFACTS

    nats-tui object bucket create ALL_REGIONS \
      --source REGION_A \
      --source REGION_B

    nats-tui object bucket edit ARTIFACTS \
      --ttl 720h \
      --compression \
      --max-bytes 10737418240

    nats-tui object bucket delete ARTIFACTS --yes

    nats-tui object put ARTIFACTS ./release.tar \
      --name releases/v1.tar \
      --metadata environment=production \
      --header Content-Type=application/x-tar

    nats-tui object get ARTIFACTS releases/v1.tar \
      --output ./release.tar

    nats-tui object edit ARTIFACTS releases/v1.tar \
      --name releases/stable.tar \
      --description "stable release" \
      --metadata environment=production

    nats-tui object delete ARTIFACTS releases/stable.tar --yes

    nats-tui object link ARTIFACTS latest RELEASES v1.tar
    nats-tui object link ARTIFACTS releases RELEASES

The application core does not read clocks, files, terminals, or NATS directly.
Clock/scheduling, filesystem access, clipboard behavior, and the NATS session
connector are injected at the composition root, so tests can run without sleeps,
local files, clipboard access, or a live server.

Press Enter on a bucket to browse its keys. Search reactively with `/`, clear
with Ctrl+U, press `n` to create an entry, and edit an opened entry with `e`.
TUI edits use compare-and-set automatically. For shell pipelines:

    nats-tui kv keys SESSIONS --filter session
    nats-tui kv get SESSIONS session.42
    nats-tui kv put SESSIONS session.42 '{"active":true}'
    nats-tui kv put SESSIONS session.42 --file payload.json
    generate-payload | nats-tui kv put SESSIONS session.42 --file -
    nats-tui kv put SESSIONS session.42 '{"active":false}' --revision 7

Open a key and press `d` for a recoverable delete marker or `P` for a
destructive purge. Both TUI actions use the displayed revision as CAS. Scripts
must confirm removal explicitly:

    nats-tui kv delete-key SESSIONS session.42 --revision 7 --yes
    nats-tui kv purge-key SESSIONS session.42 --yes

Press `H` from an open key to browse its retained revision timeline and inspect
historical payloads. The same metadata is available as stable TSV:

    nats-tui kv history SESSIONS session.42

Configuration precedence is flags, NATS_TUI_* environment variables,
configuration file, then defaults:

    go run ./cmd/nats-tui \
      --profile local \
      --server nats://127.0.0.1:4222

The default config path is $HOME/.config/nats-tui/config.yaml.

Authentication currently supports --credentials and --token. Prefer
credentials files or environment variables over storing tokens in YAML.

## Architecture

    Cobra ─┐
           ├─> application ports <─ NATS adapter
    TUI ───┘

Dependencies point inward:

- internal/app owns use cases and the interfaces they consume.
- internal/domain owns validated domain concepts and invariants.
- internal/adapters translates NATS SDK types into application read models.
- internal/ports/tui translates Bubble Tea messages into application calls.
- internal/bootstrap is the only composition root.

An architecture test rejects Bubble Tea, Cobra, Viper, nats.go, adapters,
and ports from the application layer.

## Development

    make test
    make test-race
    make build
    make dev-down

## Next vertical slices

1. Account-wide connections and subscriptions.
2. Accounts, users, and operator mode.
3. Persistent connection profiles and authentication UX.
4. Persistent bookmarks and command bar.
5. Dropped-message telemetry and release automation.
