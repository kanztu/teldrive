package events

import (
	"context"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/tgdrive/teldrive/pkg/models"
)

type EventType string

const (
	OpCreate EventType = "file_create"
	OpUpdate EventType = "file_update"
	OpDelete EventType = "file_delete"
	OpMove   EventType = "file_move"
	OpCopy   EventType = "file_copy"
)

type Recorder struct {
	db     *gorm.DB
	events chan models.Event
	logger *zap.Logger
	ctx    context.Context
	done   chan struct{} // Signals when processEvents goroutine exits
}

func NewRecorder(ctx context.Context, db *gorm.DB, logger *zap.Logger) *Recorder {
	r := &Recorder{
		db:     db,
		events: make(chan models.Event, 1000),
		logger: logger,
		ctx:    ctx,
		done:   make(chan struct{}),
	}

	go r.processEvents()
	return r
}

func (r *Recorder) Record(eventType EventType, userID int64, source *models.Source) {

	evt := models.Event{
		Type:   string(eventType),
		UserID: userID,
		Source: datatypes.NewJSONType(source),
	}

	select {
	case r.events <- evt:
	default:
		r.logger.Warn("event queue full, dropping event",
			zap.String("type", string(eventType)),
			zap.Int64("user_id", userID))
	}
}

func (r *Recorder) processEvents() {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("panic in processEvents", zap.Any("panic", rec))
		}
		close(r.done)
	}()

	for {
		select {
		case <-r.ctx.Done():
			// Process remaining events before exiting
			r.logger.Info("draining remaining events before shutdown")
			for evt := range r.events {
				if err := r.db.Create(&evt).Error; err != nil {
					r.logger.Error("failed to save event during shutdown",
						zap.Error(err),
						zap.String("type", string(evt.Type))) //nolint:unconvert
				}
			}
			return
		case evt, ok := <-r.events:
			if !ok {
				// Channel closed
				return
			}
			if err := r.db.Create(&evt).Error; err != nil {
				r.logger.Error("failed to save event",
					zap.Error(err),
					zap.String("type", string(evt.Type)), //nolint:unconvert
					zap.Int64("user_id", evt.UserID))
			}
		}
	}
}

func (r *Recorder) Shutdown() {
	close(r.events)
	<-r.done // Wait for processEvents goroutine to finish
	r.logger.Info("event recorder shutdown complete")
}
