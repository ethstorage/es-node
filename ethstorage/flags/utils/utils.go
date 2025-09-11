package utils

const envVarPrefix = "ES_NODE"

func PrefixEnvVar(name string) string {
	return envVarPrefix + "_" + name
}
