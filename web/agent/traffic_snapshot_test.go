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
