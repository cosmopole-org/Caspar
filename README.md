# Caspar Protocol 🌐

A decentralized universal protocol built on:
- a customized **Babble/Hashgraph** foundation
- **federation-first** cross-origin interoperability
- multi-runtime compute (`wasm`, `elpis`, `docker`) for programmable points/apps/machines

This repository contains the node implementation in `node/`.

## Why Caspar ✨

- **Hashgraph-backed ordering**: deterministic consensus flow via the customized Babble stack.
- **Federated state operations**: actions can execute locally, on-chain, or remotely via federation.
- **Signed action model**: request signatures are verified per user public key.
- **Real-time signaling**: live updates for users/groups over TLS sockets.
- **Programmable runtime layer**: deploy and run machine code in multiple runtimes.
- **Storage + entities + streams**: binary/file/object workflows for user/point/app assets.

## System Snapshot 🧩

```text
Clients (TCP/WS TLS, signed packets)
   -> Action Router (secure actions)
      -> Core State + Guards + Transactions
         -> Hashgraph Chain (custom Babble)
         -> Federation Bridge (cross-origin requests/updates)
         -> Runtime Drivers (Wasm | Elpis | Docker)
         -> Storage (Badger + QuestDB/PG wire)
         -> Entity/Stream HTTPS Gateways
```

## Protocol Interfaces 📡

- **Client action transport**: TLS `tcp` + `ws` binary protocol.
- **Federation transport**: TLS TCP between origins.
- **Action key format**: path-like keys (example: `/points/signal`).
- **Secondary HTTPS gateways**:
  - entity API (`ENTITY_API_PORT`)
  - VM stream gateway (`VM_API_PORT`)
- **Hashgraph service API** (Babble service): stats/blocks/graph/peers endpoints.

See full details in `docs/API_REFERENCE.md`.

## Quick Start 🚀

```bash
cd node
cp sample.env .env
```

Then configure `.env` values, TLS certs, and Firebase service account as documented in `docs/GETTING_STARTED.md`.
If you run the binary directly, make sure supporting services (QuestDB + appengine) are running too.

## Main Features

- **Users**: login/authentication, profile metadata, transfer/mint, lock/consume token flows.
- **Points**: create/join/leave, role/access policies, members, signaling, history.
- **Invites**: create/cancel/accept/decline + point/user invite lists.
- **Apps/Machines**: app lifecycle, machine lifecycle, deploy/build/log/run/stop.
- **Storage**: point file IO, entity upload/download (user/point/app), stream relay.
- **Chain ops**: create work chains, submit base transactions, register origins.
- **PC tools**: VM command execution endpoints.

## Docs

- `docs/GETTING_STARTED.md` - setup, dependencies, environment, run flow
- `docs/API_REFERENCE.md` - binary packet protocol + route catalog
- `docs/ARCHITECTURE.md` - internals of core, chain, federation, runtime, storage

## Important Runtime Notes ⚠️

- The node expects certs at `/app/certs/fullchain.pem` and `/app/certs/privkey.pem`.
- Firebase login initialization expects `/app/serviceAccounts.json`.
- QuestDB is used via PostgreSQL wire protocol (`localhost:8812` in code path).
- A pprof server is started on `0.0.0.0:9999`.
