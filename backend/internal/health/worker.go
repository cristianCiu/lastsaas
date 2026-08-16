package health

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"
	"lastsaas/internal/syslog"
	"lastsaas/internal/version"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WorkerHeartbeat publishes a forecast worker as a normal system node. Using
// the same node collection as the API health service makes worker liveness
// visible through the existing health endpoints without introducing another
// operational registry.
type WorkerHeartbeat struct {
	db        *db.MongoDB
	logger    *syslog.Logger
	nodeID    string
	component string
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewWorkerHeartbeat(database *db.MongoDB, logger *syslog.Logger, nodeID string) *WorkerHeartbeat {
	return &WorkerHeartbeat{db: database, logger: logger, nodeID: nodeID, component: "forecast-worker", stopCh: make(chan struct{})}
}

func (h *WorkerHeartbeat) Start() {
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.safeBeat(true)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.safeBeat(false)
			case <-h.stopCh:
				return
			}
		}
	}()
}

func (h *WorkerHeartbeat) Stop() {
	close(h.stopCh)
	h.wg.Wait()
}

func (h *WorkerHeartbeat) safeBeat(starting bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.beat(ctx); err != nil {
		slog.Warn("forecast worker heartbeat failed", "node", h.nodeID, "error", err)
		if h.logger != nil {
			h.logger.High(ctx, "forecast worker heartbeat failed: "+err.Error())
		}
		return
	}
	if starting && h.logger != nil {
		h.logger.Medium(ctx, "forecast worker heartbeat registered")
	}
}

func (h *WorkerHeartbeat) beat(ctx context.Context) error {
	now := time.Now().UTC()
	_, err := h.db.SystemNodes().UpdateOne(ctx, bson.M{"machineId": h.nodeID}, bson.M{
		"$set": bson.M{
			"component": h.component,
			"hostname":  hostname(),
			"status":    models.NodeStatusActive,
			"lastSeen":  now,
			"version":   version.Current,
			"goVersion": runtime.Version(),
		},
		"$setOnInsert": bson.M{"_id": primitive.NewObjectID(), "machineId": h.nodeID, "startedAt": now},
	}, options.Update().SetUpsert(true))
	return err
}
