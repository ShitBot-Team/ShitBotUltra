package bootstrap

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

type MinecraftPerformance struct {
	NodeID    string                    `json:"node_id"`
	ServerID  string                    `json:"server_id"`
	Available bool                      `json:"available"`
	Metrics   *bridge.ServerPerformance `json:"metrics,omitempty"`
	ErrorCode string                    `json:"error_code,omitempty"`
}

// Performance reads the actual game process through Bridge. Proxy nodes without
// tps.read remain unavailable; neither Core load nor another server is substituted.
func (e *Engine) Performance(ctx context.Context, nodeIDs []string) []MinecraftPerformance {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result := make([]MinecraftPerformance, len(nodeIDs))
	slots := make(chan struct{}, 4)
	var workers sync.WaitGroup
	for i, id := range nodeIDs {
		result[i].NodeID = id
		node, ok := e.Config.Node(id)
		if !ok {
			result[i].ErrorCode = "NOT_FOUND"
			continue
		}
		result[i].ServerID = node.ServerID
		workers.Add(1)
		go func(index int, nodeID string) {
			defer workers.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				result[index].ErrorCode = "DEADLINE_EXCEEDED"
				return
			}
			data, err := e.Hub.Request(ctx, nodeID, "tps.read", struct{}{})
			if err != nil {
				result[index].ErrorCode = fault.Public(err).Code
				return
			}
			var metrics bridge.ServerPerformance
			if json.Unmarshal(data, &metrics) != nil || metrics.TPS == nil || metrics.MeasuredAt.IsZero() || time.Since(metrics.MeasuredAt) > time.Minute || metrics.MeasuredAt.After(time.Now().Add(30*time.Second)) {
				result[index].ErrorCode = "METRIC_UNAVAILABLE"
				return
			}
			for _, value := range []*float64{metrics.TPS, metrics.TPS5Minutes, metrics.TPS15Minutes, metrics.MSPT} {
				if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0) {
					result[index].ErrorCode = "INVALID_RESPONSE"
					return
				}
			}
			if metrics.OnlinePlayers < 0 || metrics.MaxPlayers < 0 || metrics.UsedMemoryBytes < 0 || metrics.MaxMemoryBytes < 0 {
				result[index].ErrorCode = "INVALID_RESPONSE"
				return
			}
			result[index].Metrics = &metrics
			result[index].Available = true
		}(i, id)
	}
	workers.Wait()
	return result
}
