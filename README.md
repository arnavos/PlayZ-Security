# PlayZ Security (Go Discord Anti-Nuke)

Go-based Discord anti-nuke bot with SQLite storage and fast audit processing.

## What is upgraded
- Migrated DB to SQLite (`data/playz-security.db`) - no MongoDB required.
- Added `anti-prune` protection (audit action 21).
- Standardized anti-nuke reason format:
  - `PlayZ Security | {reason}`
- Improved audit handling with in-memory dedupe + offense window for faster moderation decisions.
- Added polished help menu and confirmed `ping` command route.

## Run
1. Install Go 1.22+
2. Set token:
```bash
export DISCORD_TOKEN="YOUR_BOT_TOKEN"
```
3. Set bot owners (`OwnerOnly` commands ke liye):
```bash
export BOT_OWNER_IDS="OWNER_ID_1,OWNER_ID_2,OWNER_ID_3"
```

Examples:
- Single owner: `export BOT_OWNER_IDS="123456789012345678"`
- Multiple owners: `export BOT_OWNER_IDS="123...,456...,789..."`
- Spaces optional hain, but avoid karna better hai.

4. Start bot:
```bash
go run main.go
```

If `DISCORD_TOKEN` is not set, bot will ask token in terminal.

## Important Discord setup
- Enable `Server Members Intent`.
- Keep bot role above staff roles so moderation actions can execute.

## Useful commands
- `>help`
- `>ping`
- `>toggle anti-prune on`
- `>toggle anti-ban on`
- `>threshold 1`
- `>logchannel`

## Notes
- Existing structure is kept (`api`, `commands`, `database`, `events`, `utils`).
- SQLite file is auto-created on startup.


- Owner-only commands tabhi chalenge jab aapka Discord ID `BOT_OWNER_IDS` me ho.
