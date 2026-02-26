# Getting Started 🚀

This guide covers local prerequisites, env setup, and the common run paths for `node/`.

## 1) Prerequisites

- Go `1.24.x` (module declares `go 1.24.1`)
- Rust/Cargo (for `node/appengine`)
- Docker (plus access to `/var/run/docker.sock` for container runtime features)
- TLS cert/key files
- Firebase service account JSON
- QuestDB runtime (PostgreSQL wire used by storage driver)

Optional but used by scripts/runtime stack:
- Firecracker / FCVMM scripts (`scripts/install-fcvmm.sh`, `scripts/run-fcvmm.sh`)
- gVisor (`scripts/install-gvisor.sh`)
- ImageMagick (`convert`) for entity image transforms

## 2) Environment File

```bash
cd node
cp sample.env .env
```

### Required env vars

| Variable | Purpose |
|---|---|
| `OWNER_ID` | owner identity (example: `keyhan@origin`) |
| `OWNER_PRIVATE_KEY` | PEM private key (PKCS8) |
| `ORIGIN` | current node origin/domain |
| `STORAGE_ROOT_PATH` | storage root |
| `BASE_DB_PATH` | Badger base DB path |
| `APPLET_DB_PATH` | applet DB path |
| `SEARCH_INDEX_PATH` | search index path |
| `POINT_LOGS_DB` | present in env template (storage currently uses local QuestDB/PG wire in code) |
| `CLIENT_WS_API_PORT` | client WS TLS port |
| `CLIENT_TCP_API_PORT` | client TCP TLS port |
| `FEDERATION_API_PORT` | federation TLS port |
| `BLOCKCHAIN_API_PORT` | chain gossip/API bind port |
| `ENTITY_API_PORT` | HTTPS entity/stream API port |
| `VM_API_PORT` | HTTPS VM gateway port |
| `IPADDR` | advertise address for chain config |
| `IS_HEAD` | node mode used by shard/bootstrap logic |
| `ROOT_NODE` | root origin used in bootstrap/election defaults |

Legacy/unused in current code path:
- `AdminPassword` (kept in template)

## 3) Required Runtime Files

The node currently expects these absolute locations:

- TLS certs:
  - `/app/certs/fullchain.pem`
  - `/app/certs/privkey.pem`
- Firebase credentials:
  - `/app/serviceAccounts.json`

If running outside container, mount/symlink paths or adapt source.

## 4) Build

```bash
cd node/appengine
cargo build

cd ../
CGO_ENABLED=1 go build -o kasper .
```

## 5) Run Options

### Option A: direct node binary

```bash
cd node
./kasper
```

For full feature availability in direct mode, also run:
- QuestDB (`scripts/run-questdb.sh`) or equivalent compatible endpoint
- appengine service (`node/appengine/target/debug/appengine`)

What starts:
- pprof server on `:9999`
- TCP/WS/federation/chain listeners from env ports
- entity + stream HTTPS APIs
- VM gateway HTTPS API

### Option B: script/container flow (production-style)

Common scripts:
- `scripts/prepare-testnet.sh`
- `scripts/build-conf.sh`
- `scripts/run-testnet.sh`
- `scripts/stop-testnet.sh`

These scripts assume host paths like `/home/kasper/...` and a Docker network `kasper`.

## 6) Default Ports (sample)

From `sample.env` / scripts:

- `CLIENT_WS_API_PORT=8076`
- `CLIENT_TCP_API_PORT=8077`
- `FEDERATION_API_PORT=8078`
- `BLOCKCHAIN_API_PORT=1337`
- `ENTITY_API_PORT=3000`
- `VM_API_PORT=3001`
- pprof: `9999`
- hashgraph service API default bind: `8000` (mapped to `8079` by `run-testnet.sh`)

## 7) First Validation Checklist ✅

- node process starts without panic
- certs loaded successfully
- Firebase init succeeds
- `/auths/getServerPublicKey` is callable via action protocol
- chain service endpoints (`/stats`, `/peers`) respond on service port

## 8) Troubleshooting

- `failed to decode PEM block`:
  - `OWNER_PRIVATE_KEY` format is invalid or not PKCS8.
- TLS listen failures:
  - missing `/app/certs/*` files.
- Firebase startup crash:
  - missing/invalid `/app/serviceAccounts.json`.
- storage insert/query issues:
  - QuestDB not running or PG wire endpoint unavailable.
