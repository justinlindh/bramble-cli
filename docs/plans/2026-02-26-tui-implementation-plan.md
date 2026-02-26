# Bramble TUI Implementation Plan

**Date:** 2026-02-26
**Repo:** bramble-cli (`github.com/justinlindh/bramble-cli`)
**Dependencies:** bramble-go SDK, Bubble Tea v2, Lip Gloss v2, Bubbles v2

---

## Overview

Add a `bramble tui` subcommand that provides a terminal UI for interacting with a Bramble mesh node. The TUI reuses the existing bramble-go SDK client (serial/WebSocket/BLE transports) and notification stream, presenting the same information architecture as the web client adapted for terminal constraints.

## Web Client → TUI Feature Mapping

### Chat Tab

| Web Feature | TUI Mapping | Phase |
|-------------|-------------|-------|
| Conversation list (DM/Broadcast/Channel) | Left pane list with unread indicators | 1 |
| Message list with auto-scroll | Right pane viewport, auto-scroll when at bottom | 1 |
| Compose bar with byte counter | Bottom textarea with live byte count + fragment warning | 1 |
| Delivery badges (●/✓/✓✓/✗) | Inline status chars after outgoing messages | 1 |
| Channel detail panel | Overlay panel toggled with `d` | 2 |
| Relay path display | Inline annotation toggled with `r` | 2 |
| Broadcast delivery panel | Expand on outgoing broadcast messages | 2 |
| Route visibility toggle | Global toggle `R` key | 2 |
| Share location button | Compose bar shortcut `Ctrl+L` | 2 |
| QR code share/scan | Skip — not useful in terminal | — |
| New message jump button | Status line "↓ N new" when scrolled up | 1 |

### Nodes Tab

| Web Feature | TUI Mapping | Phase |
|-------------|-------------|-------|
| Neighbor cards (addr, name, RSSI, SNR, last seen, tier) | Table with columns | 1 |
| Route table (dest, next hop, metric, hops, state) | Table below neighbors | 1 |
| Peer locations on cards | Location column if peer has shared location | 2 |
| Open DM action | `m` key on selected neighbor | 1 |
| Show on Map action | `l` key → jump to Location tab filtered | 2 |
| Status dot (online/reachable/unknown) | Colored status indicator in first column | 1 |
| Auto-refresh (neighbors 5s, routes 10s) | Same polling intervals | 1 |

### Location Tab (Map adaptation)

| Web Feature | TUI Mapping | Phase |
|-------------|-------------|-------|
| Leaflet map with markers | Skip — no map tiles in terminal | — |
| Own position display | Text: lat/lon/accuracy/altitude | 2 |
| Peer position list | Table: peer, lat/lon or grid square, distance, bearing | 2 |
| Route overlay on map | Textual: hop count + state per peer | 2 |
| Coarse vs exact indicator | Column showing tier per peer | 2 |
| GPS status | Header indicator: "GPS: fix" / "GPS: searching" / "No GPS" | 2 |
| Open external map | `o` key → open browser with coordinates | 3 |

### Config Tab

| Web Feature | TUI Mapping | Phase |
|-------------|-------------|-------|
| Identity section (name, address, pubkey, mailbox) | Form: editable name + mailbox toggle, read-only address/pubkey | 2 |
| Radio settings (freq, SF, BW, TX power) | Form with validated inputs | 2 |
| Channel manager (add/remove/set default) | List with `a`dd/`d`elete/`s`et-default actions | 2 |
| Peer manager (names, block/trust) | Skip for MVP — complex multi-field | 3 |
| Location config (sharing policy, contacts) | Form: mode toggle + interval + contact list | 3 |
| Traffic debug toggle | Simple on/off toggle | 2 |
| Clear message history | Confirm dialog action | 2 |
| Reboot node | Confirm dialog action | 2 |

### Stats Tab

| Web Feature | TUI Mapping | Phase |
|-------------|-------------|-------|
| Counter grid (TX/RX/dropped + deltas) | Lip Gloss panel with counters, delta arrows | 1 |
| Airtime budget bars (per tier) | Horizontal bar chars: `████░░░░ 62%` | 1 |
| System info (uptime, heap, firmware, address) | Key-value panel | 1 |
| Network reach (probe results) | Reachable count + probe trigger key `p` | 2 |
| Traffic monitor (live event log) | Scrollable viewport with event stream | 2 |
| Refresh button / auto-refresh 5s | Auto-refresh + `r` manual refresh | 1 |

### Global Chrome

| Web Feature | TUI Mapping | Phase |
|-------------|-------------|-------|
| Connection overlay (transport stages) | Header bar: transport + state + node identity | 0 |
| Tab navigation (Ctrl+1-5) | Tab bar: `1`-`5` number keys or `Tab`/`Shift+Tab` | 0 |
| Keyboard shortcuts (`/` focus, `Esc` blur) | Same bindings | 0 |
| Toast notifications | Status line messages with auto-dismiss | 1 |
| Reconnect UX | Header shows "Reconnecting..." + auto-retry | 1 |

---

## Architecture

### Directory Structure

```
internal/tui/
├── tui.go              # Root model, tab switching, global keys
├── theme.go            # Lip Gloss v2 theme (colors, borders, styles)
├── store.go            # In-memory state store (normalized data from SDK)
├── bridge.go           # SDK notification → Tea.Msg bridge goroutine
├── tabs/
│   ├── chat.go         # Chat tab model + view
│   ├── chat_compose.go # Compose bar submodel
│   ├── chat_list.go    # Conversation list submodel
│   ├── nodes.go        # Nodes tab model + view
│   ├── location.go     # Location tab model + view
│   ├── config.go       # Config tab model + view
│   └── stats.go        # Stats tab model + view
└── widgets/
    ├── table.go        # Reusable styled table
    ├── bar.go          # Horizontal progress bar
    ├── confirm.go      # Confirmation dialog
    └── statusline.go   # Bottom status/help line
```

### Root Model

```go
type Model struct {
    activeTab   int           // 0-4
    tabs        [5]tea.Model  // Chat, Nodes, Location, Config, Stats
    store       *Store        // shared state
    client      *bramble.Client
    bridge      *Bridge       // notification → tea.Msg
    width       int
    height      int
    connected   bool
    transport   string        // "ws://...", "serial:/dev/...", "ble:..."
    err         error
}
```

### State Store

Normalized in-memory store mirroring web client's Zustand store:

```go
type Store struct {
    mu            sync.RWMutex
    Identity      bramble.IdentityResponse
    Status        bramble.StatusResponse
    Config        bramble.Config // if separate from status
    Neighbors     []bramble.Neighbor
    Routes        []bramble.Route
    Airtime       bramble.AirtimeStats
    PeerLocations []bramble.LocationPeer
    Messages      []bramble.Message       // all messages
    Conversations map[string]*Conversation // keyed by conv ID
    ActiveConvID  string
    ShowRoutes    bool
}
```

### Event Bridge

Goroutine that converts SDK callbacks into Bubble Tea messages:

```go
type Bridge struct {
    program *tea.Program
}

// Registers all SDK notification callbacks, each sending a tea.Msg
func (b *Bridge) Start(client *bramble.Client) {
    client.OnMessage(func(m bramble.Message) {
        b.program.Send(MsgReceived{Message: m})
    })
    client.OnAck(func(a bramble.Ack) {
        b.program.Send(AckReceived{Ack: a})
    })
    client.OnNeighborChange(func() {
        b.program.Send(NeighborChanged{})
    })
    // ... etc for all 8 notification types
}
```

### Poll Ticks

```go
type tickMsg struct{}

func tickCmd() tea.Cmd {
    return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
        return tickMsg{}
    })
}
```

On tick: refresh Status, Neighbors (5s), Routes (10s), Airtime (10s), PeerLocations (10s).

### Connection Lifecycle

1. `bramble tui` parses connection flags (same as existing CLI: `--addr`, `--serial`, `--ble`)
2. Create SDK client, connect
3. Initial data load: Status, Identity, Messages, Neighbors, Routes, Airtime, PeerLocations
4. Start Bridge (notification callbacks)
5. Start Bubble Tea program with alt screen
6. On disconnect: show reconnecting state, auto-retry with backoff
7. On `q`/`Ctrl+C`: clean shutdown

---

## Phased Delivery

### Phase 0 — Skeleton (1-2 tasks)

**Task 0.1: TUI command plumbing + tab shell**
- Add `internal/tui/` package
- Add `bramble tui` cobra command with same connection flags as other commands
- Root model with 5-tab bar, number key switching, `q` quit
- Header: connection state + node identity + transport
- Footer: context-sensitive key hints
- Empty tab views with placeholder text
- Wire SDK client connection + initial data fetch
- Add `github.com/charmbracelet/bubbletea/v2`, `lipgloss/v2`, `bubbles/v2` to go.mod
- Verify: `bramble tui --addr ws://192.168.4.1/ws` launches, shows tabs, quits cleanly

**Task 0.2: Theme + store + event bridge**
- Lip Gloss v2 theme with Bramble colors (dark theme default, light adaptation via background query)
- Store implementation with thread-safe read/write
- Bridge goroutine wiring all 8 notification types
- Poll tick infrastructure (5s/10s intervals)
- Reconnect logic with backoff + header state

### Phase 1 — Core MVP (4-5 tasks)

**Task 1.1: Stats tab**
- Counter grid panel: TX/RX packets with delta indicators (▲/▼)
- Airtime budget bars per tier (horizontal bar chars)
- System info panel: uptime, heap, firmware version, address, pubkey
- Auto-refresh on tick, manual `r` refresh
- *Rationale: Stats is the simplest tab — good for validating the full stack (store → bridge → view)*

**Task 1.2: Nodes tab**
- Neighbor table: status dot, address, name, RSSI, SNR, last seen
- Route table: dest, next hop, hops, metric, state (colored)
- `m` key on selected neighbor opens DM (switches to Chat tab + sets active conversation)
- Sortable by column (tab through sort keys)
- Auto-refresh on tick + neighbor change notification

**Task 1.3: Chat tab — message display**
- Conversation list (left pane): Broadcast, Channels, DMs with unread counts
- Message viewport (right pane): sender, timestamp, text, delivery badge
- Auto-scroll when at bottom, "↓ N new" indicator when scrolled up
- Conversation switching via list selection
- Messages populated from initial fetch + live OnMessage notifications
- Ack updates reflected in real-time (delivery badges update)

**Task 1.4: Chat tab — compose + send**
- Textarea input at bottom of chat pane
- Live byte counter with fragmentation warning at >200 bytes
- `Enter` sends (broadcast or DM depending on active conversation)
- Channel-aware send for channel conversations
- `Esc` clears compose, `/` focuses compose from anywhere
- Send result feedback (success/failure toast)

**Task 1.5: Global polish — help + toasts + reconnect**
- `?` key opens help overlay listing all keybindings per tab
- Toast messages (bottom status line): send success, errors, reconnect events
- Reconnect UX: header shows state, auto-retry, manual `Ctrl+R` retry

### Phase 2 — Feature Parity (5-6 tasks)

**Task 2.1: Chat enhancements**
- Route annotations toggle (`r` key): show relay path inline below messages
- Channel detail panel (`d` key): channel name, PSK status, member count
- Broadcast delivery expansion: show per-recipient delivery status
- Location sharing shortcut (`Ctrl+L`)

**Task 2.2: Location tab**
- Own position summary (lat/lon/accuracy/altitude)
- Peer location table: address, name, position (exact or grid square), tier, distance, bearing
- GPS status indicator in header
- `o` key: open browser with coordinates for selected peer

**Task 2.3: Config tab — identity + radio**
- Identity form: editable node name (with save), read-only address/pubkey, mailbox toggle
- Radio form: frequency, spreading factor, bandwidth, TX power (validated ranges)
- Save with confirmation dialog
- Reboot action with confirmation

**Task 2.4: Config tab — channels**
- Channel list with index, name, PSK status, default indicator
- `a` add channel (name + optional PSK input)
- `d` remove channel (with confirmation)
- `s` set as default channel
- Traffic debug toggle

**Task 2.5: Stats enhancements**
- Network reach: reachable node count + `p` key triggers probe, shows results
- Traffic monitor: scrollable event log viewport, category/direction filters
- Counter delta tracking (show change since last refresh)

**Task 2.6: Nodes enhancements**
- Peer location column (distance/bearing if location shared)
- `l` key jumps to Location tab filtered to selected node

### Phase 3 — Polish (2-3 tasks)

**Task 3.1: Config tab — advanced**
- Peer manager (names, annotations)
- Location config (sharing policy, contacts, intervals)
- Clear message history with confirmation

**Task 3.2: Testing + docs**
- Golden tests for key view renders
- Test store mutations with concurrent access
- `docs/cli/tui.md` with screenshots and keybinding reference
- Update README with TUI section

**Task 3.3: Performance hardening**
- Virtualized message viewport (trim to visible window for long logs)
- Throttle high-frequency notification renders (batch within 100ms)
- Profile memory usage with large neighbor/route tables

---

## Key Bindings Summary

### Global
| Key | Action |
|-----|--------|
| `1`-`5` | Switch tab |
| `Tab` / `Shift+Tab` | Next/prev tab |
| `q` / `Ctrl+C` | Quit |
| `?` | Help overlay |
| `Ctrl+R` | Force reconnect |

### Chat
| Key | Action |
|-----|--------|
| `/` | Focus compose |
| `Enter` | Send message |
| `Esc` | Blur compose / close panel |
| `r` | Toggle route annotations |
| `d` | Toggle channel detail |
| `Ctrl+L` | Attach location |
| `↑`/`↓` | Scroll messages |
| `j`/`k` | Select conversation |

### Nodes
| Key | Action |
|-----|--------|
| `↑`/`↓` / `j`/`k` | Navigate rows |
| `m` | Open DM with selected |
| `l` | Show on Location tab |
| `s` | Cycle sort column |

### Stats
| Key | Action |
|-----|--------|
| `r` | Refresh now |
| `p` | Send probe |
| `↑`/`↓` | Scroll traffic log |

### Config
| Key | Action |
|-----|--------|
| `↑`/`↓` / `j`/`k` | Navigate fields |
| `Enter` | Edit field / confirm |
| `Esc` | Cancel edit |
| `a` | Add channel |
| `d` | Delete channel |

---

## Dependencies

```
github.com/charmbracelet/bubbletea/v2   v2.0.0
github.com/charmbracelet/lipgloss/v2    v2.0.0
github.com/charmbracelet/bubbles/v2     v2.0.0
```

Existing: `github.com/spf13/cobra`, `github.com/justinlindh/bramble-go`

## Risks

1. **Charm v2 is fresh** — v2.0.0 just released. May hit edge cases. Mitigate: pin versions, report upstream.
2. **Concurrent state** — notifications arrive async. Mitigate: single-writer store, all mutations via tea.Msg.
3. **Terminal variance** — colors/keys differ. Mitigate: fallback keybindings, test in common terminals.
4. **Scope** — 5 tabs is a lot. Mitigate: strict phase gates, Stats + Nodes first for quick validation.
