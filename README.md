# o11y-canary

An o11y canary to test our metrics platform features via synthetic clients.

Features:

* OTLP metrics canary
  * Can push metrics to an OTLP endpoint and then query the availability of those metrics
  * Reports on success and lag from sending and then retrieving metrics from a remote datasource like VictoriaMetrics
* Prometheus metrics export
  * Exposes internal canary and runtime metrics via a Prometheus-compatible endpoint (HTTP)
* Configurable targets and intervals
  * Supports multiple remote endpoints and customizable test intervals via configuration
* Tracing support
  * Optionally emits OTLP traces for canary operations
* Docker and local development support
  * Includes Docker Compose setup for local testing with VictoriaMetrics and otel-tui

## Metrics

The following Prometheus metrics are instrumented by o11y-canary:

| Metric Name                               | Type      | Labels                                                                                              | Description                                                                                                                       |
| ----------------------------------------- | --------- | --------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `o11y_canary_canaried_metric_total`       | Gauge     | target, canary, canary_request_id                                                                   | Synthetic metric written by the canary to test ingestion and querying. Not available on localhost:8080 - sent to remote endpoint. |
| `o11y_canary_info`                        | Gauge     | version, log_level, config_file, tracing_endpoint, service.name, service.version, service.namespace | Canary build and runtime information.                                                                                             |
| `o11y_canary_queries_total`               | Counter   | canary_name                                                                                         | Total number of query attempts, including successes and failures.                                                                 |
| `o11y_canary_query_successes_total`       | Counter   | canary_name                                                                                         | Total number of successful queries.                                                                                               |
| `o11y_canary_query_errors_total`          | Counter   | canary_name                                                                                         | Total number of failed queries.                                                                                                   |
| `o11y_canary_query_duration_seconds`      | Histogram | canary_name                                                                                         | Duration of successful queries in seconds.                                                                                        |
| `o11y_canary_lag_duration_seconds`        | Histogram | canary_name                                                                                         | Time from metric write to successful query (lag) in seconds.                                                                      |
| Various auto-exported GRPC metrics `rpc*` | Various   | Various                                                                                             | N/A                                                                                                                               |

## Config

See the [test](test) directory for example configurations including TLS options.

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

Then access the [VictoriaMetrics UI](https://localhost:8428/vmui). Canaried metrics will appear under `o11y_canary_canaried_metric_total`. Note that the metrics *of* the canary itself will not be in VictoriaMetrics. They can be found locally with:

```console
curl -s $(sudo docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' o11y-canary-o11y-canary-1):8080/metrics

# or otel-tui
sudo docker attach otel-tui
```

[otel-tui](https://github.com/ymtdzzz/otel-tui) is used to view metrics and traces of the canary itself. Run `sudo docker attach otel-tui` to use it.

### Testing

[Venom](https://github.com/ovh/venom) is used for integration tests. Run `sudo venom run tests.yml` to spin up the docker compose stack.

#### Local TLS/mTLS Testing with mkcert

Local certs are generated for mTLS testing with [mkcert](https://github.com/FiloSottile/mkcert) (used by o11y-canary and VictoriaMetrics for mTLS).
