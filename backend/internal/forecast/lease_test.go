package forecast

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"lastsaas/internal/testutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestJobLeaseConcurrencyHeartbeatAndCompletion(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := Scope{TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID()}
	store := NewJobStore(database, LeaseConfig{LeaseDuration: time.Minute, MaxAttempts: 2, BackoffBase: time.Millisecond, BackoffMax: time.Second})
	job, err := store.Enqueue(ctx, scope, primitive.NewObjectID(), primitive.NewObjectID(), "lease-concurrency-1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	leases := make(chan Lease, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			lease, e := store.Claim(ctx, owner, time.Now().UTC())
			if e == nil {
				leases <- lease
			}
		}(owner)
	}
	wg.Wait()
	close(leases)
	claimed := make([]Lease, 0, 1)
	for lease := range leases {
		claimed = append(claimed, lease)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one atomic claimant, got %d", len(claimed))
	}
	lease := claimed[0]
	if err := store.Heartbeat(ctx, job.ID, lease.Job.Owner, lease.Token, time.Now().UTC()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := store.Complete(ctx, job.ID, lease.Job.Owner, lease.Token, *job.RunID, time.Now().UTC()); err != nil {
		t.Fatalf("complete: %v", err)
	}
	var saved bson.M
	if err := database.ForecastJobs().FindOne(ctx, bson.M{"_id": job.ID}).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved["status"] != "succeeded" {
		t.Fatalf("unexpected terminal status: %#v", saved["status"])
	}
}

func TestJobRetryBackoffAndDeadLetterAreBounded(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	ctx := context.Background()
	scope := Scope{TenantID: primitive.NewObjectID(), LocationID: primitive.NewObjectID()}
	store := NewJobStore(database, LeaseConfig{LeaseDuration: time.Minute, MaxAttempts: 1, BackoffBase: time.Second, BackoffMax: 2 * time.Second})
	job, err := store.Enqueue(ctx, scope, primitive.NewObjectID(), primitive.NewObjectID(), "retry-bound-1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.Claim(ctx, "worker-retry", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Fail(ctx, job.ID, lease.Job.Owner, lease.Token, errors.New("boom"), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if status != "dead_letter" {
		t.Fatalf("status = %s, want dead_letter", status)
	}
	if got := store.Backoff(100); got != 2*time.Second {
		t.Fatalf("backoff escaped bound: %s", got)
	}
}
