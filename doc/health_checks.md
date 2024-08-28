# Health checks

## Implementation details

Liveness and readiness probes are configured in
cmd/eric-odp-factory/server.go initHealthCheck().

- Liveness probe can be fetched by "/health/liveness" endpoint and "8002" port
by default. The default port can be changed by HEALTH_CHECK_PORT environment
variable.
- Readiness probe can be fetched by "/health/readiness" endpoint and "8002"
port by default. The default port can be changed by HEALTH_CHECK_PORT
environment variable.
