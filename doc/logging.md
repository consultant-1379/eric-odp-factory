# Logging

The ODP Factory implements the following use cases and patterns from the ADP
Architecture Framework

- [UCI.LOG.DESIGN.INSTRUMENT.JSON Instrument to produce logs in a JSON format](https://eteamspace.internal.ericsson.com/display/AA/UCI.LOG.DESIGN.INSTRUMENT.JSON+-+Instrument+to+produce+logs+in+a+JSON+format)
- [UCI.LOG.COLLECT.STDOUT Collect logs produced by cloud native service](https://eteamspace.internal.ericsson.com/display/AA/UCI.LOG.COLLECT.STDOUT+Collect+logs+produced+by+cloud+native+service)
- [UCI.LOG.APPEND.METADATA Enrich Logs with metadata](https://eteamspace.internal.ericsson.com/display/AA/UCI.LOG.APPEND.METADATA+Enrich+Logs+with+metadata)
- [UCI.LOG.STORE.JSONDOC Transform and Store logs for online monitoring and analysis](https://eteamspace.internal.ericsson.com/display/AA/UCI.LOG.STORE.JSONDOC+Transform+and+Store+logs+for+online+monitoring+and+analysis)
- [UCI.LOG.DEBUG.SELECTION Activate/Deactivate DEBUG logging level in a service](https://eteamspace.internal.ericsson.com/pages/viewpage.action?pageId=1161859608)

## Implementation details

[Logrus library](https://github.com/sirupsen/logrus) is used for metrics.

All logging configuration is configured in internal/logger and internal/logctl
directories. Logs are configured according to the ADP logging format (RFC5423).

There are some functions that can be used the most, they are:

- ```log.WithFields(log.fields<fields>).<severity>(<log message>)```.
log.WithFields can take a list of extra fields that shall be added to the log
message. The log is not written to stdout until a call to log.severity is
performed.
- ```log.<severity>(<log message>)```

The logger package is used in combination with the package named logctl.
Logctl contains the implementation of the configuration file monitoring
functionality. This monitoring is needed if the logging severity is
changed in runtime.

To start the monitoring of the configuration file a call to the Watch
function in the logctl package is made. That will start the monitoring
for changes of the defined file and when that file is changed the
defined callback, OnChangeCb, will be called. This callback will set the
appropriate log level for the logger. Every log entry that is the same or
higher severity as the define log level will be written to stdout.

### Enable debug log

Debug log can be enabled by changing the ConfigMap for
ODP Factory

logcontrol.json file example:

```json
[
    {
        "severity": "info",
        "container": "eric-odp-factory"
    }
]
```

Container name should match the ServiceID constant value defined in
constants.go file.

### Streaming methods

There are supported logging streaming methods:

- direct - a pattern in which a service Pod streams the logs directly to the ADP
logging backend.
- indirect - a pattern in which a service Pod sends logs to the K8s
infrastructure via stdout, as opposed to send them directly to the ADP logging backend.
- dual -  the ability to send each log record using the direct and indirect streaming
methods at the same time.

More details about Logging - [Log Collection Patterns](https://eteamspace.internal.ericsson.com/display/AA/Log+Collection+Patterns)

If direct or dual streaming  method is used, [syslog hook](https://eteamspace.internal.ericsson.com/display/AA/SYSLOG+direct+streaming)
will be created.
