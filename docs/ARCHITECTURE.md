# Architecture 🧠

## 1) Design Goals

- **Decentralized consensus** via customized Babble/Hashgraph stack.
- **Federated operations** across origins without central gateway dependence.
- **Programmable protocol surface** through apps/machines/runtimes.
- **Signed, policy-guarded actions** for user/point scoped authorization.

## 2) Core Building Blocks

### Core (`module/core`)

`Core` orchestrates:
- action registry and secure execution
- transactional state mutations
- chain message/request/response lifecycle
- executor election coordination
- runtime dispatch for app transactions

Key flow:
1. action is parsed and guard-checked
2. state transaction opens (`ModifyState...`)
3. action mutates state + produces updates
4. updates may be signaled locally/federated
5. chain callbacks reconcile distributed responses

### Action Layer

Actions are registered via generated pluggers (`src/shell/api/pluggers/*`).
Action metadata is extracted from function comments and converted to path keys (example `/users/create`).

Secure actions support protocol-specific parsers for:
- `tcp`
- `chain`
- `fed`

## 3) Consensus Layer (Customized Babble/Hashgraph) ⛓️

Location: `src/drivers/network/chain/*`

- Hashgraph DAG + block projection inherited from Babble architecture.
- Work chains + shard chains can be created dynamically.
- A pipeline callback processes committed transactions and routes by type:
  - `baseRequest`
  - `appRequest`
  - `response`
  - `message`
  - `election`

### Executor election

Core periodically triggers an election flow (`choose-validator`) and updates `executors` used for distributed response agreement.

### Chain callback reconciliation

For distributed requests:
- responses are collected from expected executors
- payload/effects must agree before callback completion
- DB effects can be replayed on non-executing nodes

## 4) Federation Layer 🌍

Location: `src/drivers/network/federation/*`

Federation handles cross-origin:
- requests
- responses
- updates

Important behaviors:
- target origin is resolved and checked against known chain peers
- callbacks include timeout handling (120s in request-by-callback path)
- point/member updates can be materialized locally after federated events

## 5) Networking

### Client Transport

- TLS TCP server
- TLS WS server
- identical action semantics over both

### Federation Transport

- TLS TCP transport for origin-to-origin packets

### Service API

Babble-style HTTP service exposes chain stats/graph/peers/blocks.

## 6) Runtime & Compute Layer ⚙️

### Wasm driver

- assignment to machine listeners
- off-chain signal execution
- on-chain transaction group execution
- chain effect application hooks

### Elpis driver

- C/C++ bridge callbacks
- machine signal handling via runtime callback interface

### Docker driver

- image build/deploy
- container run/stop/exec
- dynamic nginx proxy config for active machine endpoints
- VM stream gateway integration

## 7) State & Storage

### KV state

- Badger-backed key/value + indexes + links model
- transactional operations through `trx` abstraction

### Time-series / logs

- QuestDB via PostgreSQL wire driver
- point signal logs + build logs

### Entity files

- global and point scoped file storage
- optional image post-processing (thumbnail/compression)

## 8) Real-time Signaling 🔔

Signaler supports:
- user-targeted signals
- group (point) signals
- federation forwarding for foreign members
- queued delivery after re-authentication

## 9) Security Model

- packet signatures verified with stored user public keys (RSA-PSS)
- guard checks include:
  - identity
  - membership/access to point
  - context-specific permissions
- server keypair generated/loaded from storage keys directory

## 10) Runtime Composition at Boot

`Core.Load(...)` wires components in order:
1. federation (phase 1)
2. storage
3. signaler
4. security
5. network
6. file
7. docker
8. wasm
9. elpis
10. federation (phase 2 fill)
11. firecracker control

Then chain pipeline + asynchronous chain submission loop are started.

