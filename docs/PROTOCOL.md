# Spade rendezvous — wire protocol

JSON text frames over WebSocket (`/v1/agent`, `/v1/client`). Blind relay
(`/v1/relay`) carries opaque binary frames after ticket admission. UDP reflex
is a separate echo (`SPADE-REFLEX` → `SPADE-REFLEX-OK <ip> <port>`).

Unknown JSON fields are ignored (forward compatible).

## Agent register — `/v1/agent`

1. Client → `hello`
2. Server → `challenge`
3. Client → `prove`
4. Server → `ready` (registered) or `error`
5. Idle: client `ping` ↔ server `pong`; server may send `offer` (see lookup)

### `hello`

```json
{ "t": "hello", "aid": "<base32>", "rids": ["<base32>", "…"], "proto": 2 }
```

- `aid` — **required** — Ed25519-derived agent id (host identity)
- `rid` / `rids` — optional blinded hourly handles (`HMAC(rvSecret, epoch)`);
  lookup aliases on the same slot, not a substitute for `aid`

### `challenge` / `prove` / `ready`

```json
{ "t": "challenge", "nonce": "<base64>" }
{ "t": "prove", "pub": "<base64 ed25519>", "sig": "<base64>", "mac": "<base64 32B>" }
{ "t": "ready", "ttl": 300 }
```

- `pub` + `sig` over the challenge nonce (Ed25519) — **always required**
- With `rids`: `mac` is 32 raw bytes (length-checked only; server never holds
  `rvSecret`). Authorisation is Ed25519, not the mac — an unpaired tablet that
  still holds `rvSecret` cannot register without the agent private key
- A live slot for the same rid/aid cannot be stolen by a second connection (`slot taken`)

### `offer` / `offer-ack` / `ping`

```json
{ "t": "offer", "sid": "<uuid>", "ticket": "<relay-ticket>", "mode": "full"|"punch" }
{ "t": "offer-ack", "sid": "<uuid>", "direct": "<ip:port or empty>" }
{ "t": "ping" }
{ "t": "pong" }
```

## Client lookup — `/v1/client`

1. Client → `lookup`
2. Server → `absent` or `peer` (and optionally late `direct`)

```json
{ "t": "lookup", "aid": "…", "rid": "…", "mode": "full"|"punch" }
{ "t": "absent", "notice": { … } }
{ "t": "peer", "sid": "<uuid>", "ticket": "<relay-ticket>", "direct": "…", "notice": { … } }
{ "t": "direct", "direct": "<ip:port>" }
{ "t": "error", "msg": "…" }
```

`notice` is optional (operator announcement). Prefer `rid` when using blinded handles.

## Relay — `/v1/relay?ticket=…`

- Ticket is single-use, ~30s TTL, role `agent`|`client`, binds a session id
- Both roles must dial; first application bytes are preserved (nested TLS)
- Byte cap `SPADE_RV_SESSION_MAX_BYTES` is **per direction** (each pump goroutine)

## Errors / admission

- HTTP `429` + `Retry-After` before upgrade when banned or over soft rate
- Soft rate windows: `SPADE_RV_RATE_*=0` means unlimited
- Protocol faults close the socket with `{"t":"error",…}`; bans only after a soft-window trip
