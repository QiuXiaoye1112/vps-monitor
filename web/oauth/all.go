package oauth

import (
	_ "github.com/monitor-monitor/monitor/web/oauth/cloudflare"
	_ "github.com/monitor-monitor/monitor/web/oauth/factory"
	_ "github.com/monitor-monitor/monitor/web/oauth/generic"
	_ "github.com/monitor-monitor/monitor/web/oauth/github"
	_ "github.com/monitor-monitor/monitor/web/oauth/qq"
)

func All() {
	//empty function to ensure all OIDC providers are registered
}
