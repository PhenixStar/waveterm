---
title: PhenixStar/WaveTerm Fork - Future Feature Roadmap Brainstorm
date: 2026-03-24
author: brainstormer
type: brainstorm-report
---

# PhenixStar/WaveTerm Fork - Future Feature Roadmap

## Context

Fork of wavetermdev/waveterm targeting infrastructure operators. Already implemented:
- GPU VRAM native sysinfo widget (nvidia-smi integration)
- Custom widget scripts: SSH health, Docker manager, CF tunnel status, MikroTik dashboard, git-sync
- Context-aware companion tool system (btop/lazygit/etc inherit SSH+CWD)
- Shell alias auto-deployment to SSH hosts via cmd:initscript.bash
- Tab-level SSH context (in progress - tab:connection meta key)
- Compact dial-style sysinfo widget (in progress)

**Target user:** Single operator managing DGX1 V100 (30+ Docker containers, 14 CF tunnels), MikroTik routers, multiple VPS, mobile workstation.

---

## Scoring Key

- **Value:** H=High / M=Medium / L=Low
- **Effort:** S=Small (<1d) / M=Medium (1-3d) / L=Large (3-7d) / XL=Extra-large (>7d)
- **ROI Score:** H/S=5, H/M=4, H/L=3, M/S=3, M/M=2

---

## Feature Roadmap (Ranked by ROI)

### TIER 1 - High Value / Low-Medium Effort (Build First)

#### 1. Persistent Widget State Across Restarts
**Description:** Widgets remember last scroll position, filter state, and selected host between restarts. Currently all widget state resets on relaunch.

- **Effort:** S | **Value:** H | **ROI Score:** 5
- **Dependencies:** None
- **Implementation Sketch:**
  - wsh setvar widget:state:<key> <json> backed by existing block metadata store
  - Widget scripts read on init via wsh getvar widget:state:docker-filter
  - Completely userland - no Go changes needed
  - Document pattern in contrib/widgets/README.md for widget authors

---

#### 2. Alert-Triggered Tab Activation
**Description:** Widgets emit alert events that auto-focus a tab or flash a tab indicator when thresholds are crossed (GPU temp >85C, container down, tunnel offline).

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** Tab-level SSH context (in progress), widget event bus
- **Implementation Sketch:**
  - Add wave:alert OSC event type from widget scripts
  - Tab header renders amber/red dot on alert state
  - Optional: toast notification via Electron Notification API
  - Widget scripts emit alert payload via OSC escape sequence
  - No persistent config needed - purely event-driven

---

#### 3. Workspace Templates (Infra Operator Presets)
**Description:** Save and restore full tab+widget layouts as named templates. One-click launch DGX1 ops, MikroTik audit, deploy mode workspace configurations.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** Import/Export layout (upstream planned), tab SSH context
- **Implementation Sketch:**
  - JSON schema: { name, tabs: [{ title, connection, widgets: [...] }] }
  - Store in ~/.config/waveterm/workspaces/
  - Command palette entry: workspace:load <name>
  - Initial version: manual JSON file, no GUI editor needed
  - Reuse tab-restore logic already in Electron store

---

#### 4. Multi-Host Command Fan-Out
**Description:** Run a single command across multiple SSH hosts simultaneously. Results in split-pane view. Example: run docker ps on all VPS.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** Tab SSH context complete
- **Implementation Sketch:**
  - New wshcmd: wsh fan-out --hosts dgx1,vps1,vps2 -- <command>
  - Opens N terminal blocks, each SSHed to one host, command pre-populated
  - Layout: auto-split horizontally up to 4 panes, scrollable list beyond
  - No result aggregation in v1 - visual scanning is enough
  - Uses existing SSH connection pool from conncontroller

---

#### 5. Docker Container Quick-Actions Widget
**Description:** Extend the existing docker-manager widget with inline action buttons: restart, stop, view logs, exec shell - without leaving the terminal.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** Docker manager widget (exists), SSH context
- **Implementation Sketch:**
  - Tsunami widget (HTML+JS) polling docker ps --format json via SSH
  - Action row per container: [Restart] [Stop] [Logs] [Shell]
  - Shell action: spawns new terminal block with docker exec -it <id> bash
  - Log action: opens preview widget tailing docker logs -f <id>
  - Zero new Go code - pure widget layer with wsh calls

---

#### 6. SSH Credential Vault Integration
**Description:** Integrate with system keychain to auto-populate SSH passphrases. No plaintext secrets in config files.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** None (standalone)
- **Implementation Sketch:**
  - Electron: use keytar npm package (already common in VS Code)
  - Go backend: credential lookup via IPC before SSH connect
  - Connection config: auth: keychain field alongside existing ssh-identityfile
  - First version: only handles passphrase unlock for keys - no password auth
  - Enpass users: keytar reads from system keychain where Enpass syncs

---

#### 7. AI Command Explainer (Inline)
**Description:** Right-click any command in terminal scrollback to explain it - Claude/local LLM answers inline without leaving terminal context.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** Upstream AI terminal tool execution (in progress), context menu
- **Implementation Sketch:**
  - Context menu on selected terminal text: Explain with AI
  - Prepends selected text to AI widget with infra-operator system prompt
  - Reuses existing AI widget + terminal scrollback access already in upstream
  - Fork adds only the context menu entry wiring (~30 lines TS)

---

#### 8. Tab-Level Resource Budget Display
**Description:** Each tab shows live CPU%, RAM%, GPU% for its SSH host in the tab bar. Instant situational awareness without opening sysinfo.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** Tab SSH context complete
- **Implementation Sketch:**
  - Tab SSH context emits periodic metrics via OSC event from companion
  - Tab bar component reads metrics from tab meta store
  - Renders: DGX1 - CPU 34% - GPU 71% - RAM 58% in tab title area
  - Companion alias adds background poller tab-metrics.sh
  - Updates every 10s - negligible SSH overhead

---

#### 9. VPS Health Dashboard (Multi-Host Aggregate)
**Description:** Single widget showing health scores for all registered VPS hosts: uptime, load, disk usage, cert expiry days remaining. Red/yellow/green per host.

- **Effort:** M | **Value:** H | **ROI Score:** 4
- **Dependencies:** SSH health widget (exists), multi-host fan-out concept
- **Implementation Sketch:**
  - Extend existing ssh-health script to accept host list from config
  - JSON config: ~/.config/waveterm/vps-fleet.json with host+port+key entries
  - Tsunami widget: card grid, one card per host
  - Cert expiry: openssl s_client check piped through SSH
  - Refresh: 5min background polling with cached last-known state

---

### TIER 2 - High Value / Larger Effort (Build After Tier 1)

#### 10. Cloudflare Tunnel Topology Map Widget
**Description:** Visual graph of all 14 CF tunnels showing online/offline status, origin service, latency, recent error rate. Click tunnel to open SSH session to origin.

- **Effort:** L | **Value:** H | **ROI Score:** 3
- **Dependencies:** CF tunnel status widget (exists as basis), SSH context
- **Implementation Sketch:**
  - Tsunami widget using D3.js or plain SVG (no heavy deps)
  - Data source: cloudflared tunnel list --output json + per-tunnel metrics
  - Node click spawns new terminal block SSH to origin host
  - Color coding: green=healthy, yellow=degraded, red=offline
  - Refresh interval: 30s (CF API rate limits)

---

#### 11. Scheduled Task Runner
**Description:** Define cron-like tasks that run in the background and log output to a dedicated widget panel. Backup verification, cert expiry checks, health pings.

- **Effort:** L | **Value:** H | **ROI Score:** 3
- **Dependencies:** Alert system (Feature 2), persistent state (Feature 1)
- **Implementation Sketch:**
  - Config: ~/.config/waveterm/schedules.json with cron expressions
  - Go backend: lightweight scheduler using robfig/cron (standard Go library)
  - Output: per-task ring buffer (last 50 runs), viewable in Scheduler widget
  - Alert integration: failed runs emit wave:alert event
  - No UI for creating schedules in v1 - JSON file only

---

#### 12. Session Recording and Replay
**Description:** Record terminal sessions (input + output + timing) to a local file. Replay at original speed or scrub through. Useful for audit trails and incident post-mortems.

- **Effort:** L | **Value:** H | **ROI Score:** 3
- **Dependencies:** None (standalone)
- **Implementation Sketch:**
  - Uses asciinema format (well-supported open spec)
  - Go backend: opt-in recording per connection config
  - wsh record start/stop commands
  - Replay: new block type asciinema-player (existing JS players available)
  - Storage: ~/.waveterm/sessions/YYYY-MM-DD-host-hash.cast
  - Rotation: keep last 90 days

---

#### 13. MikroTik Live Topology Widget
**Description:** Real-time network topology map from MikroTik API - shows connected clients, VLAN assignments, interface throughput, DHCP leases. Click device to SSH.

- **Effort:** L | **Value:** H | **ROI Score:** 3
- **Dependencies:** MikroTik dashboard widget (exists as basis)
- **Implementation Sketch:**
  - Data: MikroTik REST API (RouterOS 7+) or SSH + /ip neighbor print
  - Tsunami widget: SVG diagram auto-generated from neighbor discovery data
  - Device click spawns SSH terminal to device IP
  - VLAN color coding per existing infra scheme (100-110, 40, 50, 60)
  - Refresh: 60s topology, 5s throughput sparklines

---

#### 14. AI-Generated Runbook Assistant
**Description:** When a command fails, AI searches internal runbooks (markdown files in configured dir) for matching procedures and suggests next steps.

- **Effort:** L | **Value:** H | **ROI Score:** 3
- **Dependencies:** AI inline (Feature 7), persistent context markdown (upstream planned)
- **Implementation Sketch:**
  - Config: ai:runbook-dir pointing to local markdown runbook folder (can point to D:/mapping/)
  - Trigger: right-click Diagnose with runbooks on terminal selection (not automatic)
  - System prompt instructs AI to search runbooks for relevant procedures
  - Runbook dir indexed at startup, passed as context to AI call

---

### TIER 3 - Medium Value or Speculative

#### 15. Clipboard History Widget
**Description:** Persistent clipboard ring buffer showing last 50 copied items. Click to paste into active terminal.

- **Effort:** S | **Value:** M | **ROI Score:** 3
- **Dependencies:** None (Electron clipboard API)
- **Implementation Sketch:**
  - Electron: clipboard polling every 500ms (no native change event on all platforms)
  - Store: in-memory array + localStorage for persistence across restarts
  - Widget: scrollable list, click item pastes into active terminal via wsh
  - Sensitive string detection: items matching password patterns marked as non-persistent

---

#### 16. Git Worktree Manager Widget
**Description:** Visual panel showing all git worktrees across the machine. Create/delete worktrees, see branch status, open terminal in worktree with one click.

- **Effort:** M | **Value:** M | **ROI Score:** 2
- **Dependencies:** Git sync widget (exists as basis)
- **Implementation Sketch:**
  - Tsunami widget: git worktree list --porcelain parsed to table
  - Actions: [New worktree] [Remove] [Open terminal]
  - Open terminal: spawns block with CWD set to worktree path
  - Local-only in v1

---

#### 17. Theming Engine with Operator Presets
**Description:** Full CSS variable-based theme system with presets: Night Ops (dark red accents), Terminal Green (classic), High Contrast (accessibility). Live preview.

- **Effort:** M | **Value:** M | **ROI Score:** 2
- **Dependencies:** None
- **Implementation Sketch:**
  - Extend existing CSS variable system (WaveTerm already has theme vars)
  - Theme file: JSON to CSS variable map
  - Settings UI: theme picker with live preview pane
  - Ship 3 curated presets for infrastructure operators (dark-focused)
  - No custom font bundling - system fonts only

---

#### 18. Infrastructure Change Audit Log
**Description:** Append-only local log of every command run in WaveTerm terminals, tagged with host, user, CWD, timestamp, exit code. Searchable from command palette.

- **Effort:** M | **Value:** M | **ROI Score:** 2
- **Dependencies:** Tab SSH context (for host tagging)
- **Implementation Sketch:**
  - Shell integration hook: PROMPT_COMMAND appends to ~/.waveterm/audit.jsonl
  - Companion deploy: installs audit hook on all SSH hosts
  - Pure shell-side - no backend changes needed
  - Search widget: jq query interface over the JSONL file
  - Rotation: keep last 90 days, gzip compress older files

---

#### 19. Multi-User Shared Session (Read-Only Spectator Mode)
**Description:** Share a terminal session URL (LAN only) that others can watch in real-time via browser. Incident response without VPN complexity.

- **Effort:** XL | **Value:** M | **ROI Score:** 1
- **Dependencies:** Session recording infrastructure (Feature 12)
- **Implementation Sketch:**
  - WebSocket server in Electron serving terminal stream to local network
  - Browser client: xterm.js (no auth needed for LAN-only)
  - wsh share --readonly generates http://192.168.x.x:PORT/session/<id>
  - Write access deliberately excluded in v1 (security)
  - LAN-only binding - never exposed to internet

---

#### 20. Mobile Companion PWA (Read-Only Dashboard)
**Description:** Progressive Web App served from Electron, accessible from phone on LAN. Shows widget states, alerts, triggers pre-defined safe actions (restart container).

- **Effort:** XL | **Value:** M | **ROI Score:** 1
- **Dependencies:** Alert system (Feature 2), VPS health dashboard (Feature 9), scheduled tasks (Feature 11)
- **Implementation Sketch:**
  - Electron exposes local HTTP server (wave:// protocol infra already exists)
  - PWA: React app reading widget states via WebSocket
  - Auth: simple token in URL (LAN-only, short-lived)
  - Safe actions whitelist: only pre-approved commands triggerable from mobile
  - No terminal access from mobile - view-only + whitelisted actions

---

## Priority Matrix Summary

| Rank | Feature | Value | Effort | ROI |
|------|---------|-------|--------|-----|
| 1 | Persistent Widget State | H | S | 5 |
| 2 | Alert-Triggered Tab Activation | H | M | 4 |
| 3 | Workspace Templates | H | M | 4 |
| 4 | Multi-Host Command Fan-Out | H | M | 4 |
| 5 | Docker Container Quick-Actions | H | M | 4 |
| 6 | SSH Credential Vault | H | M | 4 |
| 7 | AI Command Explainer (Inline) | H | M | 4 |
| 8 | Tab-Level Resource Budget | H | M | 4 |
| 9 | VPS Health Dashboard | H | M | 4 |
| 10 | AI Runbook Assistant | H | L | 3 |
| 11 | Session Recording and Replay | H | L | 3 |
| 12 | CF Tunnel Topology Map | H | L | 3 |
| 13 | MikroTik Live Topology Widget | H | L | 3 |
| 14 | Scheduled Task Runner | H | L | 3 |
| 15 | Clipboard History Widget | M | S | 3 |
| 16 | Git Worktree Manager | M | M | 2 |
| 17 | Theming Engine | M | M | 2 |
| 18 | Infrastructure Audit Log | M | M | 2 |
| 19 | Multi-User Shared Session | M | XL | 1 |
| 20 | Mobile Companion PWA | M | XL | 1 |

---

## Recommended Sprint Order

### Sprint 1 - Quick Wins
1. Persistent Widget State (S) - immediately useful for all existing widgets, zero risk
2. Clipboard History Widget (S) - standalone, no deps

### Sprint 2 - Core Operator UX
3. Alert-Triggered Tab Activation (M)
4. Workspace Templates (M) - unlocks daily workflow dramatically
5. Docker Container Quick-Actions (M) - extends existing widget

### Sprint 3 - Intelligence Layer
6. Multi-Host Fan-Out (M)
7. VPS Health Dashboard (M)
8. AI Command Explainer (M)
9. Tab-Level Resource Budget (M) - after tab:connection lands

### Sprint 4 - Advanced Monitoring
10. CF Tunnel Topology Map (L)
11. MikroTik Live Topology Widget (L)
12. Session Recording and Replay (L)

### Deferred
- Scheduled Task Runner, SSH Credential Vault, Audit Log, Theming
- Mobile PWA, Shared Sessions - only if team grows beyond single operator

---

## Architecture Notes

**Recurring patterns across all features:**
1. A shared widget event bus (OSC events from shell to tab bar) unlocks 6+ features
2. Workspace Templates + Tab SSH Context = foundational pair unlocking 60% of value features
3. Tsunami widget framework handles all visualization without new Go block types
4. wsh setvar/getvar as the persistence primitive keeps state management simple and userland

**What NOT to build:**
- GUI settings editors for schedules/workspaces - JSON files are correct for a single operator
- Real-time collaboration features until single-operator UX is complete
- Full pub/sub system for widget communication - OSC events are sufficient
