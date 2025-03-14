# o11y-canary

An o11y canary to test our metrics platform features via synthetic clients.

Planned Features:

* OTLP canary
  * Can push metrics to an OTLP endpoint and then query their availability
* Metrics canary
  * Will serve metrics then query their availability

## Installation

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

### Local Development

#### Docker

```console
sudo docker compose up -d --build --force-recreate
```

Then access the [VictoriaMetrics UI](http://localhost:8428/vmui). Canary metrics will appear under `o11y_canary_canaried_metric_total`.

[otel-tui](https://github.com/ymtdzzz/otel-tui) is used to view metrics and traces. Run `sudo docker attach otel-tui` to use it.

### Testing

[Venom](https://github.com/ovh/venom) is used for integration tests. Run `sudo venom run tests.yml` to spin up the docker compose stack.
