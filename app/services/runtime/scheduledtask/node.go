package scheduledtask

import (
	"fmt"
	"net"
	"os"
	"strings"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/support/token"
)

func ScheduledTaskTargetsNode(targetIPs []string, nodeIP string) bool {
	if len(targetIPs) == 0 {
		return true
	}
	nodeIP = strings.TrimSpace(nodeIP)
	for _, target := range targetIPs {
		if strings.TrimSpace(target) == nodeIP {
			return true
		}
	}
	return false
}

func SchedulerNodeIP() string {
	if configured := strings.TrimSpace(facades.Config().GetString("scheduler.node_ip")); configured != "" {
		return configured
	}
	if configured := strings.TrimSpace(os.Getenv("SCHEDULER_NODE_IP")); configured != "" {
		return configured
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil {
			return ip.String()
		}
	}
	return "127.0.0.1"
}

func normalizeTargetIPs(values models.JSONSlice) models.JSONSlice {
	result := make(models.JSONSlice, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func stringSliceFromJSON(values models.JSONSlice) []string {
	result := make([]string, 0, len(values))
	for _, raw := range values {
		result = append(result, strings.TrimSpace(fmt.Sprint(raw)))
	}
	return result
}

func randomRunToken() string {
	return token.RandomHexWithFallback(16, func() string {
		return fmt.Sprintf("%d", scheduledTaskNow().UnixNano())
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return trimString(value, 255)
		}
	}
	return ""
}
