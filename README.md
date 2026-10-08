# drp-notification-api

Notification + reports API (Go, Mongo). **Hexagonal:** `internal/domain` has no web/Mongo imports. Migrations in [`drp-notification-db`](https://github.com/code-corhuila/drp-notification-db).

This increment is **Notification** (unique `sourceEventId`) and **Report** (immutable after `generatedAt`, HTTP aggregation only). HTTP adapters come later.

```bash
go test ./...
```

Child of `develop` named `feat/…`. Promote with `cherry-pick -x`.
