### speedtest-go-exporter - track internet speed with Prometheus & speedtest-go

This is a Prometheus exporter that measures internet speed and ping/jitter
using [speedtest-go], providing a compatible interface to the Python
[speedtest-exporter] in a much smaller Go package and Docker image.

[speedtest-go]: https://github.com/showwin/speedtest-go
[speedtest-exporter]: https://github.com/MiguelNdeCarvalho/speedtest-exporter

##### Features:

- Fast and more reliable speedtest results using speedtest-go
- API and Grafana dashboards compatibility with Python speedtest-exporter
- Smaller Docker image size
- logfmt logging by default

## Installation

### Using Docker/Container

Pull the container image from GitHub Container Registry:

```bash
docker pull ghcr.io/zakame/speedtest-go-exporter:master
```

Run the exporter:

```bash
docker run -p 9798:9798 ghcr.io/zakame/speedtest-go-exporter:master
```

### Using Pre-built Binaries

Download the latest release from the [releases page](https://github.com/zakame/speedtest-go-exporter/releases) for your platform.

### Building from Source

Requirements:
- Go 1.26 or later

```bash
git clone https://github.com/zakame/speedtest-go-exporter.git
cd speedtest-go-exporter
go build -o speedtest-go-exporter ./cmd/speedtest-go-exporter
./speedtest-go-exporter
```

## Usage

Once running, the exporter exposes metrics on port 9798 (by default):

- `http://localhost:9798/` - Simple HTML page with a link to metrics
- `http://localhost:9798/metrics` - Prometheus metrics endpoint

## Configuration

Configuration is done via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SPEEDTEST_PORT` | Port to listen on | `9798` |
| `SPEEDTEST_SERVER` | Speedtest server ID to use (optional, auto-selects if not set) | `` |
| `SPEEDTEST_EXPORTER_DEBUG` | Enable debug mode — any non-empty value enables it (adds Go runtime and process metrics) | `` |

### Example

```bash
export SPEEDTEST_PORT=8080
export SPEEDTEST_SERVER=12345
export SPEEDTEST_EXPORTER_DEBUG=1
./speedtest-go-exporter
```

## Metrics

The exporter provides the following Prometheus metrics. None carry labels.

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `speedtest_server_id` | Gauge | — | Numeric ID of the speedtest server used for the test |
| `speedtest_jitter_latency_milliseconds` | Gauge | milliseconds | Jitter latency |
| `speedtest_ping_latency_milliseconds` | Gauge | milliseconds | Ping (round-trip) latency |
| `speedtest_download_bits_per_second` | Gauge | bits/second | Download speed |
| `speedtest_upload_bits_per_second` | Gauge | bits/second | Upload speed |
| `speedtest_up` | Gauge | — | `1` if the last speedtest succeeded, `0` if it failed |

### Example output

```
# HELP speedtest_download_bits_per_second Speedtest download speed in bits per second.
# TYPE speedtest_download_bits_per_second gauge
speedtest_download_bits_per_second 9.4e+08
# HELP speedtest_jitter_latency_milliseconds Speedtest jitter latency in milliseconds.
# TYPE speedtest_jitter_latency_milliseconds gauge
speedtest_jitter_latency_milliseconds 0.512
# HELP speedtest_ping_latency_milliseconds Speedtest ping latency in milliseconds.
# TYPE speedtest_ping_latency_milliseconds gauge
speedtest_ping_latency_milliseconds 7.25
# HELP speedtest_server_id Speedtest server ID.
# TYPE speedtest_server_id gauge
speedtest_server_id 12345
# HELP speedtest_up Speedtest up status.
# TYPE speedtest_up gauge
speedtest_up 1
# HELP speedtest_upload_bits_per_second Speedtest upload speed in bits per second.
# TYPE speedtest_upload_bits_per_second gauge
speedtest_upload_bits_per_second 5.2e+08
```

### Failure behaviour

When a speedtest fails (network error, server unreachable, timeout), `speedtest_up` is set to `0`
and all other metrics are set to `0`. Use `speedtest_up == 0` as the signal in alerts — the
zeroed values for speed/latency metrics on failure should be ignored.

When `SPEEDTEST_EXPORTER_DEBUG` is set, additional Go runtime metrics (`go_*`) and process
metrics (`process_*`) from the standard Prometheus Go client collectors are also exposed.

## Kubernetes Deployment

Example Kubernetes manifests are available in the [examples/standard/](examples/standard/) directory:

```bash
kubectl apply -k examples/standard/
```

This creates:
- A Deployment running the exporter
- A Service exposing the metrics endpoint
- Resource limits and security context for production use

## Prometheus Configuration

### Using ServiceMonitor (Recommended for Kubernetes)

If you're using [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator), create a `ServiceMonitor` resource:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: speedtest-go-exporter
  labels:
    app.kubernetes.io/name: speedtest-go-exporter
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: speedtest-go-exporter
  endpoints:
  - port: http
    interval: 60m        # Adjust based on how often you want to run speed tests
    scrapeTimeout: 2m    # Speed tests can take time
    relabelings:
    - targetLabel: job
      replacement: speedtest-go-exporter
    - sourceLabels: [__meta_kubernetes_namespace]
      targetLabel: namespace
    - sourceLabels: [__meta_kubernetes_service_name]
      targetLabel: service
```

### Using PodMonitor (Alternative)

Alternatively, you can use a `PodMonitor` to scrape pods directly:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: speedtest-go-exporter
  labels:
    app.kubernetes.io/name: speedtest-go-exporter
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: speedtest-go-exporter
  podMetricsEndpoints:
  - port: http
    interval: 60m
    scrapeTimeout: 2m
    relabelings:
    - targetLabel: job
      replacement: speedtest-go-exporter
    - sourceLabels: [__meta_kubernetes_namespace]
      targetLabel: namespace
    - sourceLabels: [__meta_kubernetes_pod_name]
      targetLabel: pod
```

### Using prometheus.yml (Traditional)

For non-Kubernetes deployments, add this to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'speedtest'
    scrape_interval: 60m  # Adjust based on how often you want to run speed tests
    scrape_timeout: 2m    # Speed tests can take time
    static_configs:
      - targets: ['localhost:9798']
```

**Note:** Set appropriate `scrape_interval` values as speed tests consume bandwidth and may be rate-limited by test servers. Running tests too frequently may also impact your network performance.

The exporter has a built-in **90-second collection timeout** per scrape. If the speedtest server does
not respond within that window, the scrape returns `speedtest_up 0` with all other metrics zeroed.
Set `scrape_timeout` to a value greater than 90s (e.g. `2m`) so Prometheus waits for the exporter to
return the failure metrics rather than timing out the scrape itself.

## License

See [LICENSE](LICENSE) file for details.
