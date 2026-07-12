package config

import (
	"strings"

	"goravel/app/facades"
)

func init() {
	config := facades.Config()
	config.Add("module_admission", map[string]any{
		"allowed_index_hosts": splitModuleAdmissionHosts(config.EnvString("MODULE_ADMISSION_ALLOWED_INDEX_HOSTS", "")),
		"max_bundle_bytes":    config.Env("MODULE_ADMISSION_MAX_BUNDLE_BYTES", 33554432),
		"download_timeout":    config.Env("MODULE_ADMISSION_DOWNLOAD_TIMEOUT", "30s"),
	})
}

func splitModuleAdmissionHosts(value string) []string {
	items := strings.Split(value, ",")
	hosts := make([]string, 0, len(items))
	for _, item := range items {
		if host := strings.TrimSpace(item); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}
