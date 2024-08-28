# Metrics

The ODP Factory implements the following use cases and patterns from the ADP
Architecture Framework

- [UC.PM.DESIGN.METRIC Instrument Metrics](https://eteamspace.internal.ericsson.com/display/AA/UC.PM.DESIGN.METRIC+-+Instrument+Metrics)
- [UCI.PM.START Initialize Metric Collection](https://eteamspace.internal.ericsson.com/display/AA/UCI.PM.START+Initialize+Metric+Collection)
- [UCI.PM.COLLECT.METRICS Collect exposed metrics](https://eteamspace.internal.ericsson.com/display/AA/UCI.PM.COLLECT.METRICS+Collect+exposed+metrics)
- [UCI.PM.QUERY.TIMESERIES Query metric timeseries for online monitoring and analysis](https://eteamspace.internal.ericsson.com/display/AA/UCI.PM.QUERY.TIMESERIES+Query+metric+timeseries+for+online+monitoring+and+analysis)

## Implementation details

[Go Prometheus library](https://github.com/prometheus/client_golang) is used
for metrics.

Metrics are configured in cmd/eric-odp-factory/server.go
initMetricsProvider().
Metrics can be fetched by "/metrics" endpoint and "8003" port by default.
The port can be changed by METRICS_PORT environment variable.

Package "internal/metric" defines the prometheus metrics for the service and
provides method for setting them up.

_helpers.tpl contains configuration for Prometheus labels
("eric-odp-factory.prometheus").
