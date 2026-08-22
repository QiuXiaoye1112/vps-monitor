package agent

import v2 "github.com/monitor-monitor/monitor/protocol/v2"

func EnqueueTrafficConfig(uuid string, config v2.TrafficConfigParams) {
	EnqueueV2Event(uuid, v2.MethodAgentTrafficConfig, config)
}

func DispatchTrafficConfig(uuid string, config v2.TrafficConfigParams) {
	if IsV2Client(uuid) {
		DispatchV2Event(uuid, v2.MethodAgentTrafficConfig, config)
	}
}
