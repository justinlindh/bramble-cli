# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

## [Unreleased]

## [2026-03-01]

_Changes captured from `git log --oneline --since=2026-02-01`._

### Added

- add --version flag to bramble CLI (b4cddf3)
- (cli): add wifi status command (395f575)
- (tui): add /mouse toggle command to enable/disable mouse capture (83e8663)
- (tui): render map URLs as OSC 8 terminal links (7303ba4)
- (tui): add mouse click support for tab switching and nickname DMs (3133dd5)
- (tui): enable mousewheel scrolling for chat scrollback (442b305)
- (tui): add /critical command for priority sends (47fbec0)
- (tui): add inline /msg command for direct DM send (d6ef694)
- channel refactor (b9d9ce6)
- /me action messages (IRC CTCP ACTION convention) (5048dba)
- IRC-style nick tags — <Name[8FE3]> for incoming messages (2773d61)
- add /nick command for changing node name (IRC-style) (41b3f7d)
- (tui): polish — byte counter, confirm flow, auto-buffers, neighbor departure, scroll indicator (22ab75a)
- (tui): IRC-style rewrite — single buffer + slash commands (409b63b)
- (tui): add IRC-style scrollback buffer model (0662b4f)
- (tui): add IRC-style status bar (3859b63)
- (tui): add IRC-style input line model (0b39b20)
- (tui): peer name resolution + local aliases + SQLite persistence (9c76382)
- (tui): Phase 2 — location, config, chat enhancements, stats upgrades (06d8439)
- (tui): add terminal UI with Bubble Tea v2 (eeb1b18)
- (cli): surface delivery replay sync capability in status (411950c)
- (cli): report OTA outcome via reboot/reconnect wait (04d48fe)
- (cli): add ota command for URL-based firmware update (057ba24)
- (cli): add protocol-native monitor tail filters (e4b3434)
- (cli): add location set-config/get-config protocol parity (0c52672)
- (cli): add broadcast --wait-delivery telemetry reporting (8e54914)
- (cli): expose broadcast delivery telemetry in send and monitor (83f4995)
- (cli): support channel-scoped broadcast via --channel (f578811)
- (cli): show channel PSK lock indicator (bdbc6a3)
- add traffic debug CLI commands (7b3010a)
- mDNS discovery + shell completion commands (06c050d)
- add --ble flag for BLE transport (3ac55db)
- bramble-cli v0.1.0 — full CLI for Bramble mesh nodes (62392ba)

### Fixed

- preserve message timestamps across reconnects (edab72a)
- remove duplicate ungrouped delivery line causing multi-line receipt rendering (547a379)
- (tui): style status bar separators with bar background (032b9a3)
- use %s instead of %d for string PacketID in monitor.go (9239055)
- use pointer scrollback + return actions from commands (ce14079)
- focus textarea at construction time, not via Init cmd (ab0bd08)
- forward unhandled msgs to input line (fixes textarea focus) (604204e)
- brighten borders/separators + treat FFFFFFFF as broadcast (2b4d647)
- pad viewport lines to full width in chat split pane (c2c739f)
- forward SendResultMsg to chat tab so sent messages appear (6ff5a2b)
- config tab arrow keys now cross section boundaries (122df63)
- tab bar single-line layout + add / key for compose (fd6246f)
- pad tab bar and content area to full terminal size (b0b09c8)
- wire PreloadFromDB into chat tab so persisted messages render (5cf07be)
- wire Nodes tab view into renderContent switch (3d84e72)
- (ota): detect reboot success via uptime reset during reconnect (9a32b42)
- align CLI with firmware wire format (7d463af)

### Documentation

- add TUI section to README with commands, layout, navigation (b7818ac)
- fix node name max length (8 → 32) in README (bd81587)
- (cli): document location config parity and monitor topics (6a5e6ee)
- (cli): sync README with current command capabilities (d989747)

### Changed

- replace ad-hoc maps with typed structs for JSON output (6e572d8)
