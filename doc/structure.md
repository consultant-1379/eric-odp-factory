# Structure

The structure is based on ADP Reference Application.
[More details about this pattern](https://github.com/golang-standards/project-layout).

## Directories

### - /charts

This directory contains Helm charts.

### - /ci

This directory contains CI configuration files.

### - /cmd

#### cmd Implementation details

The main logic of the microservice is implemented in the file cmd/eric-odp-factory/server.go.
At the end of the file, the main() function is located.

### - /internal

#### internal Implementation details

github.com/fsnotify/fsnotify library is used

In config.go, the structure and the values are specified for the configuration
holding object (in type Config struct ... and func getConfig() *Config ...,
respectively), aided by helper functions getOsEnvInt() and getOsEnvString()
that take the specific values from the environment - along with default values
if they are not present. The configuration object is ready to be used.
