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

### Homebrew (macOS / Linux)

```sh
brew install DanielTangnes/azlocal/azlocal
```

### `go install`

```sh
go install github.com/DanielTangnes/azlocal/cmd/azlocal@latest
# or pin a specific version
go install github.com/DanielTangnes/azlocal/cmd/azlocal@v0.1.0
```

### Pre-built binaries

Download the archive for your OS/arch from the
[releases page](https://github.com/DanielTangnes/azlocal/releases) and extract
the `azlocal` binary onto your `PATH`.

```sh
# linux/amd64 example
curl -sSL https://github.com/DanielTangnes/azlocal/releases/latest/download/azlocal_$(curl -s https://api.github.com/repos/DanielTangnes/azlocal/releases/latest | grep tag_name | cut -d'"' -f4 | sed 's/^v//')_Linux_x86_64.tar.gz \
  | tar -xz -C /usr/local/bin azlocal
```

### From source

```sh
# requires Go 1.22+ and Docker
git clone https://github.com/DanielTangnes/azlocal
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

## Resources and seeding

The entities you declare under `services:` are created for you. Service Bus
queues/topics/subscriptions are baked into a `Config.json` mounted into the
emulator at startup; blob containers, queues, tables, and Cosmos
databases/containers are created over the wire once the emulators are healthy.

Optional `seed:` entries load data. Each entry has a `target` URI and a `from`
path:

```yaml
seed:
  - target: blob://uploads          # upload a file or a whole directory
    from: ./fixtures/sample-files/
  - target: cosmos://app/users      # JSON array or NDJSON of documents
    from: ./fixtures/users.json
  - target: queue://work-items      # JSON array of messages
    from: ./fixtures/messages.json
  - target: table://users           # JSON array of entities (PartitionKey/RowKey)
    from: ./fixtures/rows.json
  - target: servicebus://orders     # JSON array of messages (sb:// also works)
    from: ./fixtures/orders.json
```

`azlocal up -d` provisions resources and loads seed data automatically after the
suite is healthy. You can also run the steps on their own against a running
suite:

```sh
azlocal provision   # create declared resources
azlocal seed        # load seed data
```

## Connection strings

When `azlocal up -d` finishes it prints connection strings for the enabled
services. Defaults match the well-known emulator credentials so existing SDK
samples and `DefaultAzureCredential` paths Just Work.

## Roadmap

- [x] CLI skeleton (`up`, `down`, `status`, `logs`, `init`, `render`)
- [x] Compose generation for Azurite, Cosmos, Service Bus
- [x] Declarative resource creation (blob containers, queues, tables, Cosmos
      databases/containers, Service Bus queues/topics/subscriptions)
- [x] Declarative seeding (blob files, Cosmos documents, queue/Service Bus
      messages, table rows)
- [ ] Unified web UI (browse blobs, query Cosmos, peek Service Bus)
- [ ] CI mode polish (`--ci`, `--wait-healthy`, JUnit health output)
- [ ] Mocks for Key Vault, Event Grid, SignalR
- [ ] Record/replay against real Azure resources
- [x] Homebrew / `go install` distribution

## Development

```sh
make test    # run unit tests
make build   # produces bin/azlocal
make run ARGS="render"
```

## Releasing

Releases are produced by [GoReleaser](https://goreleaser.com/) on every
`v*` tag push. To cut a new release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow then:

1. Cross-compiles binaries for linux / macOS / windows on amd64 and arm64
2. Publishes a GitHub Release with archives, `checksums.txt`, and an
   auto-generated changelog
3. Updates the Homebrew formula in
   [`DanielTangnes/homebrew-azlocal`](https://github.com/DanielTangnes/homebrew-azlocal)

To dry-run locally:

```sh
goreleaser release --snapshot --clean --skip=publish
```

## License

MIT — see [LICENSE](./LICENSE).
