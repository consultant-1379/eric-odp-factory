package config

import (
	"os"
	"strconv"
	"strings"
)

const (
	defaultRestListenAddr = ":8001"

	defaultHealthCheckPort = 8002
	defaultMetricsPort     = 8003

	defaultLogstashSyslogPort = "5014"
	defaultLogStreamingMethod = "indirect"

	defaultOdpErrorTime = 120

	defaultK8sQPS = 50

	defaultLogFileWrites = 10000

	defaultOdpPreix = "eric-odp"
)

type Config struct {
	RestCertFile   string
	RestKeyFile    string
	RestCAFile     string
	RestTLSEnabled bool
	RestListenAddr string

	FactoryErrorTime int
	FactoryOdpPrefix string

	K8sQPS int

	Namespace  string
	Kubeconfig string

	LdapURL         string
	LdapCAFile      string
	LdapUserBaseDN  string
	LdapUserQuery   string
	LdapUserAttrs   string
	LdapGroupBaseDN string
	LdapGroupQuery  string
	LdapGroupAttrs  string
	LdapCredsDir    string

	TokenServiceURL      string
	TokenServiceCAFile   string
	TokenServiceCertFile string
	TokenServiceKeyFile  string

	MetricsPort     int
	HealthCheckPort int

	LogControlFile     string
	LogStreamingMethod string
	LogFile            string
	LogFileWrites      int
}

var instance *Config

func GetConfig() *Config {
	if instance == nil {
		instance = &Config{
			RestTLSEnabled: getOsEnvBool("REST_TLS_ENABLED", false),
			RestCertFile:   getOsEnvString("REST_CERT_FILE", "certificate.pem"),
			RestKeyFile:    getOsEnvString("REST_KEY_FILE", "key.pem"),
			RestCAFile:     getOsEnvString("REST_CA_FILE", "ca.pem"),
			RestListenAddr: getOsEnvString("REST_LISTEN_ADDR", defaultRestListenAddr),

			FactoryErrorTime: getOsEnvInt("FACTORY_ERROR_TIME", defaultOdpErrorTime),
			FactoryOdpPrefix: getOsEnvString("FACTORY_ODP_PREIX", defaultOdpPreix),

			Namespace:  getOsEnvString("NAMESPACE", ""),
			Kubeconfig: getOsEnvString("KUBECONFIG", ""),
			K8sQPS:     getOsEnvInt("K8S_QPS", defaultK8sQPS),

			LdapURL:         getOsEnvString("LDAP_URL", ""),
			LdapCAFile:      getOsEnvString("LDAP_TLS_CA", ""),
			LdapUserBaseDN:  getOsEnvString("LDAP_USER_BASE_DN", ""),
			LdapUserQuery:   getOsEnvString("LDAP_USER_QUERY", ""),
			LdapUserAttrs:   getOsEnvString("LDAP_USER_ATTRS", ""),
			LdapGroupBaseDN: getOsEnvString("LDAP_GROUP_BASE_DN", ""),
			LdapGroupQuery:  getOsEnvString("LDAP_GROUP_QUERY", ""),
			LdapGroupAttrs:  getOsEnvString("LDAP_GROUP_ATTRS", ""),
			LdapCredsDir:    getOsEnvString("LDAP_CREDS_DIR", ""),

			TokenServiceURL:      getOsEnvString("TOKENSERVICE_URL", ""),
			TokenServiceCAFile:   getOsEnvString("TOKENSERVICE_CA_FILE", ""),
			TokenServiceCertFile: getOsEnvString("TOKENSERVICE_CERT_FILE", ""),
			TokenServiceKeyFile:  getOsEnvString("TOKENSERVICE_KEY_FILE", ""),

			// LogControlFile INT.LOG.CTRL for controlling log severity
			LogControlFile:     getOsEnvString("LOG_CTRL_FILE", ""),
			LogFile:            getOsEnvString("LOG_FILE", ""),
			LogFileWrites:      getOsEnvInt("LOG_FILE_WRITES", defaultLogFileWrites),
			LogStreamingMethod: getOsEnvString("LOG_STREAMING_METHOD", defaultLogStreamingMethod),

			HealthCheckPort: getOsEnvInt("HEALTH_CHECK_PORT", defaultHealthCheckPort),
			MetricsPort:     getOsEnvInt("METRICS_PORT", defaultMetricsPort),
		}
	}

	return instance
}

func getOsEnvString(envName string, defaultValue string) string {
	result := strings.TrimSpace(os.Getenv(envName))
	if result == "" {
		result = defaultValue
	}

	return result
}

func getOsEnvInt(envName string, defaultValue int) int {
	envValue := strings.TrimSpace(os.Getenv(envName))

	result, err := strconv.Atoi(envValue)
	if err != nil {
		result = defaultValue
	}

	return result
}

func getOsEnvBool(envName string, defaulValue bool) bool {
	envValue := strings.TrimSpace(os.Getenv(envName))

	value, err := strconv.ParseBool(envValue)
	if err != nil {
		return defaulValue
	}

	return value
}
