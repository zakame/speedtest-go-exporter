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
