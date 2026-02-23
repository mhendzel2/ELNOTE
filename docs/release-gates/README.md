# Pilot/UAT Release Gate

Release evidence for pilot/go-live is recorded as a versioned JSON artifact.

1. Copy `pilot-uat-go-live.template.json` to `pilot-uat-go-live.json`.
2. Fill every checklist item as `true` only when verified.
3. Add signer metadata (`signedBy`, `signedRole`, `signedAtUtc`, `signatureRef`).
4. Link evidence entries to concrete files or URLs.
5. Validate before release:

```bash
./scripts/verify_go_live_gate.sh
```

Tag/release workflows enforce this validation.
