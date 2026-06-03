# azlocal

> A unified local Azure emulator suite. One binary, one config, one command.

`azlocal` orchestrates Azurite, the Cosmos DB Linux emulator, the Service Bus
emulator, and (soon) lightweight mocks for services that have no official
emulator — all behind a single CLI and a single declarative config file.

> **Status:** early development. Storage + Cosmos + Service Bus orchestration
> works; seeding, the unified web UI, and Event Grid / Key Vault mocks are on
> the roadmap.

## Why

Azure devs currently juggle a mess of emulators with inconsistent quality and
no shared story for configuration, seeding, or CI. AWS has LocalStack; Azure
has nothing equivalent. `azlocal` aims to be that.

## Install

```sh
# from source (requires Go 1.22+ and Docker)
git clone https://github.com/yourusername/azlocal
cd azlocal
make install
```

## Quick start

```sh
azlocal init           # writes a starter azlocal.yaml
azlocal up -d          # starts the suite in the background
azlocal status
azlocal logs -f cosmos
azlocal down           # stop (keeps data); add --volumes to wipe
```

`azlocal render` prints the generated `docker-compose.yaml` without starting
anything — useful for inspection or committing to a repo.

## Configuration

```yaml
# azlocal.yaml
services:
  blob:
    containers: [uploads, thumbnails]
  queue:
    queues: [work-items]
  cosmos:
    databases:
      - name: app
        containers:
          - { name: users,  partitionKey: /tenantId }
          - { name: orders, partitionKey: /customerId }
  servicebus:
    queues: [orders, notifications]
    topics:
      - name: events
        subscriptions: [audit, billing]
```

## Connection strings

When `azlocal up -d` finishes it prints connection strings for the enabled
services. Defaults match the well-known emulator credentials so existing SDK
samples and `DefaultAzureCredential` paths Just Work.

## Roadmap

- [x] CLI skeleton (`up`, `down`, `status`, `logs`, `init`, `render`)
- [x] Compose generation for Azurite, Cosmos, Service Bus
- [ ] Declarative seeding (blob files, Cosmos containers, SB queues/topics)
- [ ] Unified web UI (browse blobs, query Cosmos, peek Service Bus)
- [ ] CI mode polish (`--ci`, `--wait-healthy`, JUnit health output)
- [ ] Mocks for Key Vault, Event Grid, SignalR
- [ ] Record/replay against real Azure resources
- [ ] Homebrew / Scoop / `go install` distribution

## Development

```sh
make test    # run unit tests
make build   # produces bin/azlocal
make run ARGS="render"
```

## License

MIT — see [LICENSE](./LICENSE).
