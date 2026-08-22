package agent

import (
	"testing"
	"time"

	v2 "github.com/monitor-monitor/monitor/protocol/v2"
	"github.com/stretchr/testify/require"
)

func TestRequestTrafficSnapshotMatchesOperationAndClient(t *testing.T) {
	const uuid = "traffic-snapshot-client"
	SetClientProtocolVersion(uuid, 2)
	SetPresence(uuid, 77, true)
	t.Cleanup(func() {
		SetPresence(uuid, 77, false)
		DeleteConnectedClients(uuid)
		v2EventMu.Lock()
		delete(v2EventQueues, uuid)
		v2EventMu.Unlock()
	})

	resultCh := make(chan v2.TrafficSnapshotResultParams, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := RequestTrafficSnapshot(uuid, time.Second)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	var event v2.Event
	require.Eventually(t, func() bool {
		events := TakeV2Events(uuid, nil, 1)
		if len(events) == 0 {
			return false
		}
		event = events[0]
		return true
	}, time.Second, 5*time.Millisecond)

	var request v2.TrafficSnapshotParams
	require.NoError(t, bindV2EventParams(event.Params, &request))
	require.NotEmpty(t, request.OperationID)
	require.Error(t, ResolveTrafficSnapshot("different-client", v2.TrafficSnapshotResultParams{
		OperationID: request.OperationID,
		TotalUp:     10,
		TotalDown:   20,
	}))
	require.NoError(t, ResolveTrafficSnapshot(uuid, v2.TrafficSnapshotResultParams{
		OperationID: request.OperationID,
		CapturedAt:  time.Now().Format(time.RFC3339Nano),
		TotalUp:     10,
		TotalDown:   20,
	}))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case result := <-resultCh:
		require.Equal(t, int64(10), result.TotalUp)
		require.Equal(t, int64(20), result.TotalDown)
	case <-time.After(time.Second):
		t.Fatal("snapshot request did not complete")
	}
}

func TestRequestTrafficResetWaitsForMatchingAgentResult(t *testing.T) {
	const uuid = "traffic-reset-client"
	SetClientProtocolVersion(uuid, 2)
	SetPresence(uuid, 78, true)
	t.Cleanup(func() {
		SetPresence(uuid, 78, false)
		DeleteConnectedClients(uuid)
		v2EventMu.Lock()
		delete(v2EventQueues, uuid)
		v2EventMu.Unlock()
	})

	resultCh := make(chan v2.TrafficResetResultParams, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := RequestTrafficReset(uuid, time.Second)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	var event v2.Event
	require.Eventually(t, func() bool {
		events := TakeV2Events(uuid, nil, 1)
		if len(events) == 0 {
			return false
		}
		event = events[0]
		return true
	}, time.Second, 5*time.Millisecond)
	require.Equal(t, v2.MethodAgentTrafficReset, event.Method)

	var request v2.TrafficResetParams
	require.NoError(t, bindV2EventParams(event.Params, &request))
	require.NotEmpty(t, request.OperationID)
	require.NoError(t, ResolveTrafficReset(uuid, v2.TrafficResetResultParams{
		OperationID:    request.OperationID,
		CapturedAt:     time.Now().Format(time.RFC3339Nano),
		CycleID:        "cycle-1",
		CycleStartedAt: time.Now().Add(-time.Hour).Format(time.RFC3339Nano),
		TotalUp:        0,
		TotalDown:      0,
	}))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case result := <-resultCh:
		require.Zero(t, result.TotalUp)
		require.Zero(t, result.TotalDown)
		require.Equal(t, "cycle-1", result.CycleID)
	case <-time.After(time.Second):
		t.Fatal("reset request did not complete")
	}
}

func TestTrafficConfigEventsCoalesceToLatest(t *testing.T) {
	const uuid = "traffic-config-client"
	t.Cleanup(func() {
		v2EventMu.Lock()
		delete(v2EventQueues, uuid)
		v2EventMu.Unlock()
	})

	EnqueueTrafficConfig(uuid, v2.TrafficConfigParams{Enabled: true, Day: 1, Timezone: "Asia/Shanghai"})
	EnqueueTrafficConfig(uuid, v2.TrafficConfigParams{Enabled: true, Day: 15, Hour: 8, Minute: 30, Timezone: "Asia/Shanghai"})

	events := TakeV2Events(uuid, nil, 0)
	require.Len(t, events, 1)
	require.Equal(t, v2.MethodAgentTrafficConfig, events[0].Method)
	var config v2.TrafficConfigParams
	require.NoError(t, bindV2EventParams(events[0].Params, &config))
	require.Equal(t, 15, config.Day)
	require.Equal(t, 8, config.Hour)
	require.Equal(t, 30, config.Minute)
}
