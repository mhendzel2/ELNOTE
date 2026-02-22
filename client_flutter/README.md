# ELNOTE Flutter Client (Sprint 5 MVP)

Offline-first Flutter MVP implemented with:

1. Local SQLite schema for experiments, entries, comments, proposals, outbox, conflicts, and sync cursor.
2. Outbox replay flow for queued create/addendum/comment/proposal mutations.
3. Cursor-based sync pull and WebSocket listener integration against `/v1/sync/*`.
4. UI for:
   - experiment list/create
   - immutable entry history
   - addendum creation (no edit-in-place)
   - comments/proposals queueing
   - conflict artifact visibility

## Run

```bash
flutter pub get
flutter run
```

## Tablet LAN Web GUI

Run API on LAN (host machine):

```bash
cd ../server
go run ./cmd/api
```

Run Flutter web UI on LAN:

```bash
flutter run -d chrome --web-hostname 0.0.0.0 --web-port 8090
```

Open on tablets using:

```text
http://<HOST_LAN_IP>:8090
```

The login API URL defaults to:

- `http://<HOST_LAN_IP>:8080` when opened from a LAN host/IP
- `http://localhost:8080` when opened locally

Optional explicit override:

```bash
flutter run -d chrome --web-hostname 0.0.0.0 --web-port 8090 --dart-define=API_BASE_URL=http://<HOST_LAN_IP>:8080
```

Release build + static serving option:

```bash
flutter build web --release --dart-define=API_BASE_URL=http://<HOST_LAN_IP>:8080
cd build/web
python -m http.server 8090 --bind 0.0.0.0
```

### Quick Start Scripts (Windows PowerShell)

From repo root, run API and tablet web host in separate terminals:

```powershell
.\scripts\start_api.ps1
.\scripts\start_tablet_web.ps1 -HostIp 0.0.0.0 -Port 8090
```

Optional rebuild before serving web bundle:

```powershell
.\scripts\start_tablet_web.ps1 -HostIp 0.0.0.0 -Port 8090 -Rebuild
```

Single command to start both API + web host (Ctrl+C stops both):

```powershell
.\scripts\start_all.ps1
```

Optional web rebuild in the same command:

```powershell
.\scripts\start_all.ps1 -RebuildWeb
```

Stop default API + web ports:

```powershell
.\scripts\stop_all.ps1
```

Stop custom ports:

```powershell
.\scripts\stop_all.ps1 -Ports 8083,8092
```
