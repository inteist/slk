# Display starred people alongside starred channels

Treat Slack channel and IM stars as conversation membership; ignore
message/file/comment stars. Use one real Slack Starred section, channels first
(public/private), then people/app/group conversation rows, stably preserving
order within each block. Custom sections named Starred remain unchanged.

Starred membership, ordering and recovery require Slack-native sections
(`wctx.SectionStore`). They do not run with only config-defined sections or the
no-provider fallback. Slack-native sections are enabled by default; global or
per-workspace `use_slack_sections=false` disables this recovery. Deferred profile
repair also applies to ordinary DM rows independently of starred membership.

At startup, recovery checks starred IDs missing after `users.conversations`
(or the `client.userBoot` fallback) and the closed-group-DM filter. Only missing
IDs need recovery: boot IM metadata with a peer ID can restore a closed starred
one-to-one DM without a network request; otherwise use read-only conversation
info. The successful `users.conversations` path does not locally filter closed
one-to-one IMs, and recovery does not assume Slack always includes or omits them.
The boot fallback does exclude closed one-to-one IMs. Never call
`conversations.open` or invent user rows.

Reconnect repeats reconciliation only after a successful actual section
rebootstrap; a debounce skip does not trigger reconciliation. The section
refresh and reconciliation share a 30-second HTTP deadline and a successful
bootstrap debounces the next refresh for 30 seconds. Presence/DND bootstrap
starts after this block so recovered human DM peers are included; on a real
refresh it may wait for the shared HTTP budget, but rapid reconnects skip that
work. Unknown peers use a context-bounded edge batch and per-user fallback. Cache-only repairs still run after cancellation. Any peer
still unresolved gets a non-blocking deferred retry outside that shared budget,
so quiet peers do not depend on a message or membership lookup to resolve.
Unnamed cache placeholders remain eligible for a first successful lookup.

Peer resolution and row hydration are independent. Existing DMUserID values
retain retry targets. `repairDMPeer` imports cached bot classification and uses
`refreshDMPeerFromCache` to repair sidebar/finder snapshots
independently, preserves section/order/visits/DND, mirrors notifier maps and
publishes only changed active-workspace rows. Inactive snapshots remain correct
for subsequent workspace switches. Finder upserts retain the greater of the
incoming and UI-owned visit timestamps, so a stale workspace snapshot cannot
undo a visit recorded in the UI this session. Only the serialized event owner mutates
conversation slices; the background sweep updates synchronized profile state.

Successful edge and per-user resolver lookups also coalesce peer IDs in a
synchronized pending set and signal the event owner without blocking. The
WebSocket reader feeds a separate serialized dispatcher, which accepts this
local wake-up even when Slack sends no new frames. The dispatcher drains the
pending IDs, filters out users without DM rows before reading SQLite and calls
`repairDMPeer`. The automatically scheduled retry repairs both rows when it
succeeds, without another reconnect or a startup UnresolvedDMs entry.
No new Slack event or UI I/O is introduced.

The DM sweep's per-user fallback also queues a repair, including for inactive
workspaces with no UI sender.

Startup hands the UI cloned channel/finder slices before starting the event
owner. This fixes a pre-existing shared-array race, documented separately in
`2026-10-10-workspace-ready-snapshot-design.md`.

Tests cover parsing/membership/order/navigation, startup and reconnect recovery,
failed/canceled profile retries, late names/app classification, finder/status
repair without reconnect (edge and per-user, active and inactive workspaces),
idle-socket wake-up and serialized dispatch, workspace switching and concurrent
sweep integration. Independent cache, notification, profile, bot, finder and
cursor fixes are reviewed in separate PRs. All six prerequisites (#279–#284)
have merged; #279 merged on 2026-10-10, and its external-user cache fix is not
part of the integrated feature diff. Merge #307 before this feature, then #303.

No new configuration, subheaders, star/unstar commands, pagination rewrite or
new star WebSocket protocol. Large-list pagination and immediate star events
remain follow-ups.
