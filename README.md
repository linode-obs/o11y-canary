# o11y-canary

An o11y canary to test our metrics platform features via synthetic clients.

Planned Features:

* OTLP canary
  * Can push metrics to an OTLP endpoint and then query their availability
* Metrics canary
  * Will serve metrics then query their availability

## Installation

### Docker

```console
sudo docker run \
--privileged \
ghcr.io/linode-obs/o11y-canary
```

### Binary

```bash
wget https://github.com/linode-obs/o11y-canary/releases/download/v{{ version }}/o11y-canary_{{ version }}_Linux_x86_64.tar.gz
tar xvf o11y-canary_{{ version }}_Linux_x86_64.tar.gz
./o11y-canary/o11y-canary
```

### Source

```bash
wget https://github.com/linode-obs/o11y-canary/archive/refs/tags/v{{ version }}.tar.gz
tar xvf o11y-canary-{{ version }}.tar.gz
cd ./o11y-canary-{{ version }}
go build o11y-canary.go
./o11y-canary.go
```

## Releasing

1. Merge commits to main.
2. Tag release `git tag -a v1.0.X -m "message"`
3. `git push origin v1.0.X`
4. `goreleaser release`

## Contributors

Contributions welcome! Make sure to `pre-commit install`.

### Testing

[Venom](https://github.com/ovh/venom) is used for integration tests. Run `sudo venom run tests.yml` to spin up the docker compose stack.
