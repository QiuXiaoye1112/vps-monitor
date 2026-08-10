package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/database/clients"
	"github.com/monitor-monitor/monitor/pkg/rpc"
	"github.com/monitor-monitor/monitor/web/realtime"
)

// StreamRealtime streams small change notifications to the browser. The
// notification only identifies the changed node/task; the frontend decides
// which targeted RPC or chart store needs to be updated.
func StreamRealtime(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Status(http.StatusInternalServerError)
		return
	}

	allowedUUIDs, unrestricted := realtimeVisibility(c)
	events, unsubscribe := realtime.Subscribe()
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	fmt.Fprint(c.Writer, ": connected\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprint(c.Writer, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, open := <-events:
			if !open {
				return
			}
			if !unrestricted {
				if _, visible := allowedUUIDs[event.UUID]; !visible {
					continue
				}
			}

			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Kind, payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func realtimeVisibility(c *gin.Context) (map[string]struct{}, bool) {
	principal := GetPrincipal(c)
	if principal != nil && principal.HasRole(rpc.RoleAdmin) {
		return nil, true
	}

	visible := make(map[string]struct{})
	allClients, err := clients.GetAllClientBasicInfo()
	if err != nil {
		return visible, false
	}
	for _, client := range allClients {
		if !client.Hidden {
			visible[client.UUID] = struct{}{}
		}
	}
	return visible, false
}
