package postgres

import (
	"context"
	"encoding/json"

	"themisy/internal/domain"
	"themisy/internal/store"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RebuildProjection(runID string) error {
	ctx := context.Background()
	var payload []byte
	err := s.pool.QueryRow(ctx, `
SELECT payload->'run' FROM outbox_events
WHERE aggregate_id=$1 AND event_type='release.run.project'
ORDER BY ((payload->'run'->>'state_version')::bigint) DESC LIMIT 1`, runID).Scan(&payload)
	if err == pgx.ErrNoRows {
		return store.ErrNotFound
	}
	if err != nil {
		return err
	}
	var run domain.ReleaseRun
	if err := json.Unmarshal(payload, &run); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	canonical, err := json.Marshal(run)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE release_runs SET status=$2,state_version=$3,payload=$4,updated_at=$5 WHERE id=$1`, run.ID, run.Status, run.StateVersion, canonical, run.UpdatedAt); err != nil {
		return err
	}
	if err := syncChildren(ctx, tx, &run); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
