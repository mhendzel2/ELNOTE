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
