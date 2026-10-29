# drp-notification-api

Notification + reports API (Go, Mongo later). **Hexagonal:** `internal/domain` has no web/Mongo imports. Migrations in [`drp-notification-db`](https://github.com/code-corhuila/drp-notification-db).

Walking HTTP for E-18/E-19 (`:8085`): lists `{data, meta}` of the caller's inbox. A `USER` and an `ADMIN` both see **only their own** rows (G-04). Foreign GET is **404**. RS256 against identity JWKS. The walking consumer is `POST /internal/events` on `:8085` (not on the gateway), gated by `X-Internal-Token`. It maps E-23 (`ReservationConfirmed` / `ReservationCancelled` / `PaymentConfirmed` / `PaymentFailed` / `user.registered`, plus `ReservationCreated` if it arrives) onto an in-memory `IN_APP` + `SENT` document. Duplicate `sourceEventId` is a no-op (BR-006).

E-20…E-22: **ADMIN** only (`USER` → **403**). Create requires `Idempotency-Key` and builds the snapshot by calling payment E-15, reservation E-11 and space E-05 (never their tables). Any non-2xx from those owners is **503** and stores nothing. Export is the stored snapshot (`csv` default or `json`), with `Content-Disposition: attachment`. AMQP + Mongo replace the hop later.

```bash
go test ./...
```

Child of `develop` named `hu-c3-*-dev`. Promote with `cherry-pick -x`.
