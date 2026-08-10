# NATS TUI Feature Roadmap

This document tracks the path toward a terminal-first operational control plane
for NATS and JetStream. The target is feature parity with the NATS management
capabilities of graphical control planes, without cloud provisioning, billing,
organization management, or proprietary hosted services.

## Status

- [x] Implemented and verified.
- [ ] Not implemented.
- **Partial** means the existing slice is useful but the section is not complete.

A feature is only checked when it is reachable from the TUI, handles empty and
error states, is responsive, and has automated coverage at the appropriate
application and adapter boundaries.

## Foundation and UX — Partial

- [x] Go CLI built with Cobra.
- [x] Bubble Tea presentation layer is decoupled from application ports and NATS adapters.
- [x] Full-screen responsive layout.
- [x] Startup presentation and visible connection loading state.
- [x] Keyboard-first navigation and help overlay.
- [x] Foreground-only color system compatible with terminals that mishandle ANSI backgrounds.
- [x] Periodic refresh with timeouts, retry state, and stale-result rejection.
- [x] Direct unauthenticated NATS URL connection.
- [x] Token and credentials-file CLI options.
- [ ] Contextual notifications/toasts with an inspectable error log.
- [ ] Configurable key bindings.
- [ ] Mouse support as an optional enhancement.
- [ ] Accessibility and reduced-color theme.
- [ ] Capability discovery based on server version, account permissions, and enabled APIs.
- [ ] Read-only safety mode.
- [ ] Destructive-operation audit log.

## Connection Profiles and Authentication — Partial

- [x] Connect using a single editable NATS URL.
- [x] Connect without authentication.
- [x] Token authentication.
- [x] Credentials-file authentication.
- [ ] Persistent named profiles.
- [ ] Startup profile selector and recent connections.
- [ ] Multiple server URLs with failover.
- [ ] Username/password authentication.
- [ ] NKey seed authentication.
- [ ] JWT authentication.
- [ ] TLS CA, client certificate, client key, server name, and verification controls.
- [ ] Connection test with actionable diagnostics.
- [ ] Secure handling and redaction of secrets.
- [ ] Import profiles from NATS CLI contexts.
- [ ] Export sanitized profile configuration.

## Dashboard and Real-Time Metrics — Partial

- [x] Server name, version, and cluster summary.
- [x] Account messages, storage, streams, and consumers.
- [x] Automatic and manual refresh.
- [x] Connected, refreshing, and stale states.
- [ ] Message and byte rates over time.
- [ ] Memory versus file storage breakdown.
- [ ] JetStream account limits and utilization percentages.
- [ ] Slow-consumer and connection health summary.
- [ ] Server CPU, memory, goroutines, uptime, and traffic.
- [ ] Historical sparklines with bounded local sampling.
- [ ] User-defined dashboard panels.

## Stream Management — Partial

- [x] List streams with subjects, messages, bytes, and consumers.
- [x] Responsive list, selection, and stream detail.
- [x] Create streams.
- [x] Edit streams while preserving unexposed advanced configuration.
- [x] Delete streams with explicit destructive confirmation.
- [x] Configure name, description, subjects, storage, retention, replicas, max messages, and max bytes.
- [x] Full stream configuration editor.
- [x] Max age, max message size, max messages per subject, duplicate window, discard policy, and compression.
- [x] Allow direct, mirror direct, rollup, deny delete, deny purge, and sealed state.
- [x] Placement tags and cluster placement.
- [x] Republish configuration.
- [x] Subject transforms.
- [x] Metadata editing.
- [x] Stream purge: all, sequence range, subject, and keep count.
- [x] Delete an individual stored message.
- [x] Full stream backup and restore with messages and durable consumer state.
- [x] Copy/export stream configuration.
- [x] Cluster and replica health with peer lag and current status.
- [x] Stream leader step-down.

## Mirrors and Sources

- [x] Create a stream mirror.
- [x] Configure mirror start sequence or start time.
- [x] Mirror subject transforms and external API/delivery prefixes.
- [x] Create and edit stream sources.
- [x] Multiple source streams.
- [x] Source filters and subject transforms.
- [x] Mirror/source lag and health.
- [x] Visualize stream dependency chains.

## Consumer Management and Lag — Partial

- [x] Account-wide consumer catalog.
- [x] Pending plus ack-pending lag calculation.
- [x] Active, idle, lagging, and paused status.
- [x] Responsive list, preview, and full detail.
- [x] Pause and resume.
- [x] Reset delivery to ack floor plus one.
- [x] Delete with confirmation.
- [ ] Create and fully edit consumers.
- [ ] Push, pull, durable, and ephemeral configuration.
- [ ] Deliver policy and start sequence/time.
- [ ] Ack policy, ack wait, max deliver, max ack pending, and backoff.
- [ ] Filter subject and multiple filter subjects.
- [ ] Replay policy, rate limit, sample frequency, headers only, and flow control.
- [ ] Ordered consumer support.
- [ ] Consumer metadata.
- [ ] Dead-letter/advisory visibility.
- [ ] Consumer throughput and lag history.
- [ ] Inspect redeliveries and delivery failures.

## Message Monitor and Live Watch — Partial

- [x] Load recent stored messages.
- [x] Responsive list and synchronized preview.
- [x] Scrollable full-message detail.
- [x] JSON formatting and syntax colors.
- [x] Subject, payload, header, and sequence search.
- [x] Core NATS Live Watch without creating a persistent consumer.
- [x] Pause/resume live rendering.
- [x] Bounded message buffer and batched UI updates.
- [x] Preserve stable selection while live messages arrive.
- [ ] Select history start time, sequence, and result limit.
- [ ] Filter by subject before loading history.
- [ ] JSONPath/JMESPath payload filtering.
- [ ] XML, text, hex, and binary inspection modes.
- [ ] Full header and JetStream metadata panel.
- [ ] Copy subject, payload, headers, or complete message.
- [ ] Save/export selected messages to disk.
- [ ] Publish or republish a message.
- [ ] Delete a stored message with confirmation.
- [ ] Dropped-message and subscription-pressure telemetry.
- [ ] Freeze a live snapshot for offline inspection.

## Subject Explorer — Partial

- [x] Build a navigable tree from configured stream subjects.
- [x] Preserve configured wildcard patterns.
- [x] Search by subject or owning stream.
- [x] Responsive list and detail pane.
- [x] Jump from any branch or pattern directly into Live Watch.
- [ ] Merge observed live subjects into the configured tree.
- [ ] Per-subject message and byte rates.
- [ ] Show streams and consumers attached to a subject.
- [ ] Show publish/subscribe permissions affecting a subject.
- [ ] Publish directly to the selected subject.
- [ ] Request/reply directly from the selected subject.
- [ ] Subject hierarchy traffic heat indicators.

## Key-Value Store Management

- [x] List KV buckets with storage, replicas, size, history, and TTL.
- [x] Create buckets from both the TUI and the non-interactive CLI.
- [x] Edit mutable bucket settings from both the TUI and non-interactive CLI while preserving advanced configuration.
- [x] Delete buckets safely from the TUI and CLI with explicit confirmation.
- [x] Create mirror and source KV buckets from the TUI and CLI, and surface topology in bucket detail.
- [x] Browse and search keys from the TUI, with scriptable CLI listing and filtering.
- [x] View value, operation, revision, timestamp, and delta.
- [x] JSON formatting for values.
- [x] Create and update values from the TUI and CLI, including file and stdin input.
- [x] Compare-and-set update by revision.
- [x] Delete and purge keys with impact confirmation and optional revision CAS.
- [x] View revision history as an interactive TUI timeline and scriptable CLI output.
- [x] Restore a deleted key from a retained PUT revision with optimistic concurrency.
- [x] Watch key changes live in the TUI and watch a key or whole bucket from the CLI.
- [x] Copy values from the TUI and export raw bytes safely from the CLI.

## Object Store Management

- [x] List object buckets with storage, replicas, size, TTL, compression, and sealed state in a responsive catalog/detail workspace.
- [x] Create, edit, and delete object buckets from guided TUI forms and scriptable CLI commands.
- [x] Mirror and source object buckets with subject transforms, complete catalog visibility, and read-only mirror UX.
- [x] Browse and reactively search objects by name, description, digest, metadata, or headers.
- [x] Inspect metadata, headers, size, chunks, modification time, and digest in a responsive, scrollable detail view.
- [x] Upload files from TUI and CLI with metadata, headers, progress, speed, and cancellation.
- [x] Download files from TUI and CLI with progress, cancellation, safe overwrite semantics, and SHA-256 digest verification.
- [x] Rename objects from TUI and CLI while preserving untouched metadata.
- [x] Update descriptions, metadata, and headers from TUI and CLI.
- [x] Delete objects with explicit confirmation in TUI and `--yes` in CLI.
- [x] Watch bucket changes live with incremental updates, pause/resume reconciliation, counters, and safe cancellation.
- [x] Create, search, inspect, and live-watch cross-bucket object links and bucket links from TUI and CLI.

## Publish and Request/Reply

- [x] Publish payloads to arbitrary concrete subjects from TUI and CLI.
- [x] Add and edit repeatable headers with validation.
- [x] Load payload from file through injected filesystem ports.
- [x] Pretty JSON editor with validation and syntax-colored reply inspection.
- [x] Request/reply with configurable timeout.
- [x] Display multiple replies with latency, headers, and payload detail.
- [x] Persistent request history and replay.
- [x] Save, load, run, replace, and delete persistent request templates.
- [x] Generate and copy a shell-safe CLI equivalent for a publish or request.

## Server Topology and Runtime Observability

- [x] List servers in a cluster or supercluster.
- [x] Server detail: identity, version, uptime, endpoints, and tags.
- [x] Cluster routes and gateways.
- [x] Leaf node topology.
- [x] Render topology as a terminal graph and as a navigable table.
- [x] JetStream meta group leader and peers.
- [x] JetStream storage and resource usage per server.
- [x] Route, gateway, and leaf health.
- [x] Server reload where authorized.
- [x] Leader step-down operations with confirmation.

## Connections and Subscriptions

- [x] Account-wide live connection list.
- [x] Filter and sort by name, user, IP, language, version, RTT, traffic, and pending bytes.
- [x] Connection detail and subscription list.
- [x] Group connections by client name, user, server, or language.
- [x] Slow-consumer detection.
- [x] Connection and subscription counts over time.
- [x] Close a connection where authorized, with confirmation.
- [x] Jump from a subscription to Subject Explorer.

## Accounts, Users, and Operator Mode

This section is available only when the connection and credentials expose the
required system-account and decentralized-authentication APIs.

- [x] Detect operator mode and system-account capabilities.
- [x] List accounts and account limits.
- [x] Account usage, connections, subscriptions, and JetStream assets.
- [x] Inspect account JWT claims.
- [x] Create accounts from CLI and capability-gated TUI form.
- [x] Update and delete accounts.
- [x] List locally managed users and inspect user JWT claims.
- [x] Create users from the CLI with granular permissions.
- [x] Create users from the TUI with a locally managed account selector.
- [x] Update, revoke, and delete users.
- [x] Inspect publish and subscribe permissions for managed users.
- [x] Inspect response permissions and account/subscription limits.
- [x] Signing keys and scoped signing keys.
- [x] Credential generation and secure local save (mode `0600`, no overwrite).
- [x] Imports and exports for streams and services.
- [x] Activation tokens.
- [x] Cross-account subject sharing visibility.
- [x] Never expose seeds unless an explicit secure workflow requires them.

## Alerts and Advisories

- [x] Consume JetStream advisories and API audit events.
- [x] Consumer delivery, ack, and max-delivery advisories.
- [x] Stream and consumer action advisories.
- [x] Server and connection health thresholds.
- [x] Local alert rules with severity and cooldown.
- [x] In-app alert inbox and acknowledgement.
- [x] Jump from an alert to the affected resource.
- [x] Optional webhook or command notification integration.

## Command Bar and Automation

- [x] Global `:` command bar.
- [x] Fuzzy command and resource completion.
- [x] Command history.
- [x] Built-in navigation and operation commands.
- [x] User-defined commands with safe argument interpolation.
- [x] Dry-run preview for mutating commands.
- [x] Generate equivalent `nats` CLI commands.
- [x] Non-interactive commands for scripting selected operations.

## Bookmarks and Local Persistence

- [x] Bookmark streams, consumers, subjects, KV buckets, keys, object buckets, and objects with `B`.
- [x] Global bookmarks list and quick jump.
- [x] Persist bookmarks across sessions.
- [x] Persist recent searches and request templates.
- [x] Profile-scoped local state.
- [x] Handle renamed or deleted bookmarked resources gracefully.

## Quality, Distribution, and Operations — Partial

- [x] Unit tests for application behavior.
- [x] NATS adapter integration tests.
- [x] Race-detector verification.
- [x] Architecture dependency test.
- [x] ANSI background and reverse-video regression test.
- [ ] End-to-end terminal interaction tests.
- [ ] Compatibility test matrix across supported NATS versions.
- [ ] Permission-restricted account test fixtures.
- [ ] Large-account performance benchmarks.
- [ ] CI with test, race, vet, lint, and vulnerability scanning.
- [ ] Cross-platform release binaries.
- [ ] GoReleaser configuration and checksums.
- [ ] Homebrew formula.
- [ ] Container image for remote/admin environments.
- [ ] Version command and embedded build metadata.
- [ ] Upgrade documentation and changelog.

## Explicitly Out of Scope

- Synadia billing, plans, and invoices.
- Organization membership and SaaS tenant administration.
- Cloud region provisioning and managed infrastructure lifecycle.
- Synadia Private Link management.
- Hosted PostgreSQL, Prometheus, KMS, or OIDC control-plane services.
- Kubernetes workload deployment and connector marketplace management.

## Recommended Delivery Order

1. Key-Value Store management.
2. Object Store management.
3. Advanced stream configuration, purge, mirrors, and sources.
4. Complete consumer creation and configuration.
5. Publish and request/reply workflows.
6. Server topology, connections, and runtime metrics.
7. Persistent profiles, TLS, and complete authentication.
8. Command bar and bookmarks.
9. Operator-mode accounts, users, permissions, imports, and exports.
10. Alerts, backups, release engineering, and final parity polish.
