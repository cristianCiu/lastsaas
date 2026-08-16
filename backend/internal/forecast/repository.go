package forecast

import (
	"context"
	"errors"
	"sync"
)

// SealedDatasetRepository is intentionally small: implementations may persist
// the immutable dataset in Mongo, a file, or an in-memory test store without
// giving the forecast core access to mutable sale state.
type SealedDatasetRepository interface {
	PutSealed(context.Context, SealedDataset) error
	GetSealed(context.Context, string) (SealedDataset, error)
}

// MemoryRepository provides an immutable reference implementation and is also
// useful for deterministic unit and integration tests.
type MemoryRepository struct {
	mu       sync.RWMutex
	datasets map[string]SealedDataset
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{datasets: make(map[string]SealedDataset)}
}

func (r *MemoryRepository) PutSealed(ctx context.Context, dataset SealedDataset) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dataset.Manifest.RowCount != len(dataset.Rows) || dataset.Manifest.ContentHash == "" || dataset.Manifest.ContentHash != HashRows(dataset.Manifest, dataset.Rows) {
		return errors.New("sealed dataset manifest hash mismatch")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := dataset.Manifest.ContentHash
	if old, ok := r.datasets[key]; ok {
		// A hash-addressed dataset may be re-submitted, but never replaced.
		if old.Manifest != dataset.Manifest {
			return errors.New("sealed dataset hash collision")
		}
		return nil
	}
	dataset.Rows = cloneRows(dataset.Rows)
	r.datasets[key] = dataset
	return nil
}

func (r *MemoryRepository) GetSealed(ctx context.Context, hash string) (SealedDataset, error) {
	if err := ctx.Err(); err != nil {
		return SealedDataset{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.datasets[hash]
	if !ok {
		return SealedDataset{}, errors.New("sealed dataset not found")
	}
	d.Rows = cloneRows(d.Rows)
	return d, nil
}

func cloneRows(in []MaterializedRow) []MaterializedRow {
	out := make([]MaterializedRow, len(in))
	copy(out, in)
	for i := range out {
		out[i].SourceIDs = append([]string(nil), out[i].SourceIDs...)
	}
	return out
}

func MaterializeAndStore(ctx context.Context, repo SealedDatasetRepository, req MaterializeRequest) (SealedDataset, error) {
	if err := ctx.Err(); err != nil {
		return SealedDataset{}, err
	}
	dataset, err := Materialize(req)
	if err != nil {
		return SealedDataset{}, err
	}
	if err := repo.PutSealed(ctx, dataset); err != nil {
		return SealedDataset{}, err
	}
	return dataset, nil
}
