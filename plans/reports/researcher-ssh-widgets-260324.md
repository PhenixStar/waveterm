# SSH-Aware Widgets Feasibility Report
**Date:** 2026-03-24
**Branch:** feature/tab-ssh-context
**Researcher:** researcher agent

---

## Executive Summary

WaveTerm already has a mature SSH+wsh RPC infrastructure. Three of the four proposed concepts are buildable with moderate effort by extending existing patterns. X11 forwarding is the only infeasible concept in a clean Electron context — the best substitute is xpra HTML5 client embedded in a webview. The sysinfo widget system is the most powerful leverage point: its `TimeSeriesData` + `wps.Event_SysInfo` pub/sub pipeline already works over SSH and can absorb disk I/O, network throughput, and process lists with minimal plumbing. SSH tunnel management is doable but requires new Go state management and a dedicated widget.

---

## Concept 1 — Remote File Explorer Widget

**Feasibility: Easy (already works)**

### How the existing preview widget does it

The `PreviewModel` (`frontend/app/view/preview/preview-model.tsx`) reads `connection` from block meta and passes it to all file operations. The backend exposes a full `WshRpcRemoteFileInterface` (`pkg/wshrpc/wshrpctypes_file.go`):

- `RemoteListEntriesCommand` — streaming directory listing
- `RemoteFileInfoCommand` — stat a remote path
- `RemoteFileStreamCommand` — stream file bytes
- `RemoteFileCopyCommand`, `RemoteFileMoveCommand`, `RemoteFileDeleteCommand`, etc.

These route over the wsh domain socket that `conncontroller.go` establishes on the remote host. The `preview-directory.tsx` already calls `formatRemoteUri(path, connection)` and passes the connection string through the RPC calls. The directory view over SSH is functional today.

### What a dedicated "Remote Files" widget would add vs existing preview

The existing preview is a general-purpose viewer (file + directory). A dedicated remote-files widget would:
- Pin to a specific connection from block meta (`connection` key)
- Show breadcrumbs, bookmarks, drag-drop targets
- Dual-pane layout (local ↔ remote)
- SCP-style copy between connections

**Implementation sketch:** New view type `"remotefiles"` reusing all of `WshRpcRemoteFileInterface` plus a React component adapted from `preview-directory.tsx`. No new backend code needed. ~400 lines of frontend.

---

## Concept 2 — Reverse X11 GUI Forwarding

**Feasibility: Infeasible (native X11) / Medium (xpra HTML5)**

### Why native X11 is not viable

WaveTerm is Electron (Chromium). Chromium has no X11 display server capability — it is an X11 *client*, not a server. There is no way to receive X11 connections from a remote host into a WaveTerm block. Options like VcXsrv or XQuartz would require launching an external display server outside of WaveTerm, which breaks the embedded widget model.

### xpra HTML5 client — the viable path

xpra (`xpra.org`) is a persistent remote application framework with an HTML5 client that runs in a browser. The HTML5 client speaks xpra's own WebSocket protocol (not X11) and renders remote GTK/Qt/X11 apps as canvas in the browser. Key facts:

- xpra-html5 package ships a self-contained web server + static assets
- Electron's `<webview>` tag or `BrowserView` can embed any localhost URL
- Path: SSH port-forward xpra's WebSocket port (default 10000) → `localhost:XXXXX` → embed in a WaveTerm block via webview

**Implementation sketch:**
1. When opening an X11 block, WaveTerm SSH port-forwards remote xpra port to a random local port
2. Block renders `<webview src="http://localhost:{port}" />` or iframe in a dedicated view type `"x11"`
3. User must have xpra running on the remote with `--html=on`

**Constraints:**
- Requires xpra installed on remote host (not zero-setup)
- Electron webview sandbox policies may need relaxation
- Frame rate: acceptable for low-motion apps (terminals, editors), poor for video
- noVNC (VNC-in-browser) is an alternative with wider server support but lower quality

**Recommendation:** Implement as an opt-in "remote GUI" block type. Do not integrate xpra startup into WaveTerm — just handle the port-forward and webview embedding. Complexity is in the port-forward lifecycle management (see Concept 4).

---

## Concept 3 — Remote System Vitals Widget

**Feasibility: Easy (extend existing sysinfo)**

### How existing sysinfo remote works

`pkg/wshrpc/wshremote/sysinfo.go` runs `RunSysInfoLoop` on the remote host via wsh connserver. It collects CPU (`gopsutil`), mem (`gopsutil`), and GPU (`nvidia-smi`) every second and publishes `wps.Event_SysInfo` with `TimeSeriesData{Ts, Values map[string]float64}` scoped to `connName`. The frontend `SysinfoViewModel` subscribes via `waveEventSubscribeSingle` and renders with `@observablehq/plot`. The GPU extension (`getGpuData`) was added in this fork.

### Additional metrics that can be added

All via gopsutil v4 (already imported) or simple exec commands:

| Metric | Source | Key naming pattern |
|---|---|---|
| Disk I/O (read/write bytes/s) | `gopsutil/disk.IOCounters` | `disk:sda:read`, `disk:sda:write` |
| Network throughput (rx/tx bytes/s) | `gopsutil/net.IOCounters` | `net:eth0:rx`, `net:eth0:tx` |
| Per-process CPU/mem (top-N) | `gopsutil/process` | push as separate event type |
| Container stats | `docker stats --no-stream` or containerd API | `container:{name}:cpu` |
| Load average | `gopsutil/load.Avg` | `load:1`, `load:5`, `load:15` |
| Temp sensors | `gopsutil/host.SensorsTemperatures` | `temp:{label}` |

### How the wsh event system works for custom metrics

The pattern from `sysinfo.go:generateSingleServerData`:

```go
event := wps.WaveEvent{
    Event:   wps.Event_SysInfo,          // existing event type
    Scopes:  []string{connName},         // connName scopes to the connection
    Data:    tsData,                     // wshrpc.TimeSeriesData
    Persist: 1024,                       // ring buffer size for history
}
wshclient.EventPublishCommand(client, event, &wshrpc.RpcOpts{NoResponse: true})
```

For custom metric types (e.g., process list), define a new event name in `pkg/wps/wpstypes.go` alongside `Event_SysInfo`, add a corresponding TypeScript type in `frontend/types/waveevent.d.ts`, and register a subscriber in the widget model. The wsh router automatically propagates events from remote to local over the domain socket.

**Implementation sketch for disk+net:** Add `getDiskIOData` and `getNetIOData` functions to `sysinfo.go` following `getCpuData`'s pattern. Sample deltas (bytes since last sample / interval). Add `TimeSeries_Disk` and `TimeSeries_Net` constants. Frontend: add plot types "Disk I/O" and "Network" to `PlotTypes` in `sysinfo.tsx`. Total: ~80 lines Go, ~40 lines TS.

---

## Concept 4 — SSH Tunnel Management Widget

**Feasibility: Medium**

### Current SSH implementation

`SSHConn` struct (`pkg/remote/conncontroller/conncontroller.go`) manages the `*ssh.Client`. It currently only opens one tunnel: the wsh domain socket (`client.ListenUnix(sockName)` in `OpenDomainSocketListener`). There is no LocalForward/RemoteForward state.

The `golang.org/x/crypto/ssh` client exposes:
- `client.Dial("tcp", remoteAddr)` — local forward (proxy local port to remote endpoint)
- `client.Listen("tcp", remoteAddr)` — remote forward (proxy remote port back to local)

`connections.json` config currently supports: `conn:wshenabled`, `conn:moshenabled`, `conn:wshpath`, `conn:shellpath`, `conn:ignoresshconfig`. No tunnel config exists.

### Adding LocalForward/RemoteForward to connections.json

**Schema addition** to `ConnMetaType` in `pkg/wconfig/settingsconfig.go`:

```go
ConnLocalForwards  []string `json:"conn:localforwards,omitempty"`   // ["8080:localhost:8080"]
ConnRemoteForwards []string `json:"conn:remoteforwards,omitempty"`  // ["9090:localhost:9090"]
```

Format mirrors OpenSSH: `localPort:remoteHost:remotePort`.

**Backend:** On connection, parse the forward specs and for each:
- LocalForward: `net.Listen("tcp", fmt.Sprintf(":%d", localPort))` + goroutine that `client.Dial()` per accepted conn
- RemoteForward: `client.Listen("tcp", fmt.Sprintf("localhost:%d", remotePort))` + goroutine that `net.Dial()` per accepted conn

Store active tunnel state in `SSHConn` struct. Publish `wps.WaveEvent` with a new `Event_TunnelChange` event when tunnels start/stop.

**Frontend widget** (`view type: "sshtunnels"`):
- Subscribe to `Event_TunnelChange` + `Event_ConnChange`
- List active connections with their configured tunnels
- Toggle buttons to start/stop individual tunnels
- "Add tunnel" form: localPort / remoteHost / remotePort
- Status indicators (active / error / stopped)

**Complexity drivers:**
- Tunnel goroutine lifecycle management (reconnect on SSH disconnect, cleanup)
- Per-connection tunnel config storage in `connections.json`
- New RPC command `GetTunnelStatusCommand` to query active tunnel state
- Frontend state sync when tunnels fail

**Implementation sketch size:** ~250 lines Go (tunnel manager), ~300 lines TSX (widget), schema changes in 2 files.

---

## Recommendations (Priority Order)

1. **Disk I/O + Network metrics for sysinfo** — highest ROI, ~120 lines total, zero new infrastructure. Add to existing `sysinfo.go` + `sysinfo.tsx`.

2. **Dedicated Remote Files widget** — no backend work, reuse all existing RPC. Good UX win for SSH-heavy workflows.

3. **SSH Tunnel Management widget** — medium effort but high value for power users. Implement config schema + backend lifecycle first, widget second.

4. **xpra HTML5 forwarding** — requires xpra on remote. Implement as an advanced/experimental block type only after tunnel management is solid (it depends on the same port-forward plumbing).

---

## Unresolved Questions

- Electron `<webview>` sandbox restrictions: does the default CSP in WaveTerm allow loading arbitrary `localhost:PORT` origins? Needs testing before committing to xpra approach.
- Tunnel reconnect behavior: should tunnels auto-reconnect after SSH disconnect? Need product decision on UX.
- `gopsutil` container stats support: v4 does not have native containerd support. Docker stats via exec is the fallback — need to verify `docker` is on PATH on target hosts.
- Process list event: pushing per-second snapshots of top-N processes is high bandwidth. Need to decide on sampling rate and event structure before implementing.
