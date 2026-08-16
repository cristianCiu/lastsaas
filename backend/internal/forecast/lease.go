package forecast

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrLeaseLost       = errors.New("forecast job lease lost")
	ErrJobNotFound     = errors.New("forecast job not found")
	ErrEnqueueConflict = errors.New("forecast enqueue idempotency conflict")
)

type LeaseConfig struct {
	LeaseDuration time.Duration
	MaxAttempts   int32
	BackoffBase   time.Duration
	BackoffMax    time.Duration
}

func DefaultLeaseConfig() LeaseConfig {
	return LeaseConfig{LeaseDuration: 2 * time.Minute, MaxAttempts: 5, BackoffBase: 5 * time.Second, BackoffMax: 5 * time.Minute}
}

type JobStore struct {
	DB     *db.MongoDB
	Config LeaseConfig
}

func NewJobStore(database *db.MongoDB, cfg LeaseConfig) *JobStore {
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = DefaultLeaseConfig().LeaseDuration
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultLeaseConfig().MaxAttempts
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = DefaultLeaseConfig().BackoffBase
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = DefaultLeaseConfig().BackoffMax
	}
	return &JobStore{DB: database, Config: cfg}
}

func (s *JobStore) Enqueue(ctx context.Context, scope Scope, datasetID, policyID primitive.ObjectID, key string, now time.Time) (models.ForecastJob, error) {
	if len(key) < 8 || len(key) > 128 {
		return models.ForecastJob{}, errors.New("invalid forecast enqueue key")
	}
	var existing models.ForecastJob
	filter := bson.M{"tenantId": scope.TenantID, "locationId": scope.LocationID, "idempotencyKey": key}
	if err := s.DB.ForecastJobs().FindOne(ctx, filter).Decode(&existing); err == nil {
		if existing.DatasetID != datasetID || existing.PolicyID != policyID {
			return models.ForecastJob{}, ErrEnqueueConflict
		}
		return existing, nil
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return models.ForecastJob{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	job := models.ForecastJob{ID: primitive.NewObjectID(), TenantID: scope.TenantID, LocationID: scope.LocationID, DatasetID: datasetID, PolicyID: policyID, RunID: objectIDPtr(primitive.NewObjectID()), Status: models.ForecastJobQueued, IdempotencyKey: key, Attempt: 0, MaxAttempts: s.Config.MaxAttempts, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	if _, err := s.DB.ForecastJobs().InsertOne(ctx, job); err != nil {
		if mongo.IsDuplicateKeyError(err) && s.DB.ForecastJobs().FindOne(ctx, filter).Decode(&existing) == nil {
			if existing.DatasetID != datasetID || existing.PolicyID != policyID {
				return models.ForecastJob{}, ErrEnqueueConflict
			}
			return existing, nil
		}
		return models.ForecastJob{}, err
	}
	return job, nil
}

type Lease struct {
	Job   models.ForecastJob
	Token string
}

func (s *JobStore) Claim(ctx context.Context, owner string, now time.Time) (Lease, error) {
	if owner == "" {
		return Lease{}, errors.New("forecast worker owner is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if _, err := s.RecoverStale(ctx, now); err != nil {
		return Lease{}, err
	}
	token, err := randomToken()
	if err != nil {
		return Lease{}, err
	}
	filter := bson.M{"$or": bson.A{
		bson.M{"status": models.ForecastJobQueued},
		bson.M{"status": models.ForecastJobRetryWait, "nextAttemptAt": bson.M{"$lte": now}},
		bson.M{"status": models.ForecastJobRunning, "leaseExpiresAt": bson.M{"$lte": now}},
	}, "attempt": bson.M{"$lt": s.Config.MaxAttempts}}
	expires := now.Add(s.Config.LeaseDuration)
	update := bson.M{"$set": bson.M{"status": models.ForecastJobRunning, "owner": owner, "leaseToken": token, "leaseExpiresAt": expires, "heartbeatAt": now, "updatedAt": now}, "$inc": bson.M{"attempt": int32(1)}}
	var job models.ForecastJob
	err = s.DB.ForecastJobs().FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}).SetReturnDocument(options.After)).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Lease{}, ErrJobNotFound
	}
	if err != nil {
		return Lease{}, err
	}
	return Lease{Job: job, Token: token}, nil
}

func (s *JobStore) Heartbeat(ctx context.Context, jobID primitive.ObjectID, owner, token string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := s.DB.ForecastJobs().UpdateOne(ctx, bson.M{"_id": jobID, "status": models.ForecastJobRunning, "owner": owner, "leaseToken": token, "leaseExpiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"heartbeatAt": now, "leaseExpiresAt": now.Add(s.Config.LeaseDuration), "updatedAt": now}})
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *JobStore) Complete(ctx context.Context, jobID primitive.ObjectID, owner, token string, runID primitive.ObjectID, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := s.DB.ForecastJobs().UpdateOne(ctx, bson.M{"_id": jobID, "status": models.ForecastJobRunning, "owner": owner, "leaseToken": token, "leaseExpiresAt": bson.M{"$gt": now}}, bson.M{"$set": bson.M{"status": models.ForecastJobSucceeded, "runId": runID, "updatedAt": now}, "$unset": bson.M{"owner": "", "leaseToken": "", "leaseExpiresAt": "", "heartbeatAt": "", "nextAttemptAt": ""}})
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *JobStore) Fail(ctx context.Context, jobID primitive.ObjectID, owner, token string, failure error, now time.Time) (models.ForecastJobStatus, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var current models.ForecastJob
	if err := s.DB.ForecastJobs().FindOne(ctx, bson.M{"_id": jobID, "status": models.ForecastJobRunning, "owner": owner, "leaseToken": token}).Decode(&current); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", ErrLeaseLost
		}
		return "", err
	}
	status := models.ForecastJobRetryWait
	set := bson.M{"status": status, "lastError": truncateError(failure), "updatedAt": now}
	if current.Attempt >= current.MaxAttempts {
		status = models.ForecastJobDeadLetter
		set["status"] = status
	} else {
		set["nextAttemptAt"] = now.Add(s.Backoff(current.Attempt))
	}
	_, err := s.DB.ForecastJobs().UpdateOne(ctx, bson.M{"_id": jobID, "status": models.ForecastJobRunning, "owner": owner, "leaseToken": token}, bson.M{"$set": set, "$unset": bson.M{"owner": "", "leaseToken": "", "leaseExpiresAt": "", "heartbeatAt": ""}})
	if err != nil {
		return "", err
	}
	return status, nil
}

func (s *JobStore) Backoff(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := s.Config.BackoffBase
	for i := int32(1); i < attempt && d < s.Config.BackoffMax; i++ {
		d *= 2
		if d > s.Config.BackoffMax {
			d = s.Config.BackoffMax
		}
	}
	return d
}

// RecoverStale marks exhausted stale leases dead-lettered. It does not delete
// or rewrite history, and non-exhausted stale leases remain claimable.
func (s *JobStore) RecoverStale(ctx context.Context, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := s.DB.ForecastJobs().UpdateMany(ctx, bson.M{"status": models.ForecastJobRunning, "leaseExpiresAt": bson.M{"$lte": now}, "$expr": bson.M{"$gte": bson.A{"$attempt", "$maxAttempts"}}}, bson.M{"$set": bson.M{"status": models.ForecastJobDeadLetter, "lastError": "lease expired after maximum attempts", "updatedAt": now}, "$unset": bson.M{"owner": "", "leaseToken": "", "leaseExpiresAt": "", "heartbeatAt": ""}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func objectIDPtr(id primitive.ObjectID) *primitive.ObjectID { return &id }
func truncateError(err error) string {
	if err == nil {
		return ""
	}
	text := fmt.Sprint(err)
	if len(text) > 2000 {
		return text[:2000]
	}
	return text
}
