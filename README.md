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
