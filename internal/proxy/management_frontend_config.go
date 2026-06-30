package proxy

import (
	"strconv"
	"strings"
)

const (
	ManagementConfigUIFileName = "config-ui.yaml"
	ManagementConfigUIPath     = "/" + ManagementConfigUIFileName
)

// RenderManagementConfigUI renders the browser-facing llm-proxy, MPR UI, and TAuth YAML config.
func RenderManagementConfigUI(configuration ManagementConfiguration) string {
	var builder strings.Builder
	builder.WriteString("llmProxy:\n")
	builder.WriteString("  managementApiOrigin: ")
	builder.WriteString(strconv.Quote(configuration.ManagementAPIOrigin))
	builder.WriteString("\n")
	builder.WriteString("  proxyOrigin: ")
	builder.WriteString(strconv.Quote(configuration.ProxyOrigin))
	builder.WriteString("\n")
	builder.WriteString("environments:\n")
	builder.WriteString("  - description: ")
	builder.WriteString(strconv.Quote(configuration.UIDescription))
	builder.WriteString("\n")
	builder.WriteString("    origins:\n")
	for _, originValue := range configuration.UIOrigins {
		builder.WriteString("      - ")
		builder.WriteString(strconv.Quote(originValue))
		builder.WriteString("\n")
	}
	builder.WriteString("    auth:\n")
	builder.WriteString("      tauthUrl: ")
	builder.WriteString(strconv.Quote(configuration.TAuthURL))
	builder.WriteString("\n")
	builder.WriteString("      googleClientId: ")
	builder.WriteString(strconv.Quote(configuration.GoogleClientID))
	builder.WriteString("\n")
	builder.WriteString("      tenantId: ")
	builder.WriteString(strconv.Quote(configuration.TAuthTenantID))
	builder.WriteString("\n")
	builder.WriteString("      loginPath: ")
	builder.WriteString(strconv.Quote(configuration.LoginPath))
	builder.WriteString("\n")
	builder.WriteString("      logoutPath: ")
	builder.WriteString(strconv.Quote(configuration.LogoutPath))
	builder.WriteString("\n")
	builder.WriteString("      noncePath: ")
	builder.WriteString(strconv.Quote(configuration.NoncePath))
	builder.WriteString("\n")
	return builder.String()
}
