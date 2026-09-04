package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"themisy/internal/domain"
	"themisy/internal/store"

	"github.com/jackc/pgx/v5"
)

func (s *Store) PendingOutbox(limit int) ([]domain.OutboxEvent, error) {
	return s.pendingOutbox("", limit)
}

func (s *Store) PendingOutboxByType(eventType string, limit int) ([]domain.OutboxEvent, error) {
	return s.pendingOutbox(eventType, limit)
}

func (s *Store) pendingOutbox(eventType string, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(context.Background(), `
SELECT id,aggregate_type,aggregate_id,event_type,payload,attempts,available_at,published_at,created_at
FROM outbox_events WHERE published_at IS NULL AND available_at <= now()
  AND ($1 = '' OR event_type = $1)
ORDER BY available_at,created_at LIMIT $2`, eventType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.OutboxEvent
	for rows.Next() {
		var event domain.OutboxEvent
		var payload []byte
		if err := rows.Scan(&event.ID, &event.AggregateType, &event.AggregateID, &event.EventType, &payload,
			&event.Attempts, &event.AvailableAt, &event.PublishedAt, &event.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &event.Payload); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) MarkOutboxPublished(id string, publishedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(), `UPDATE outbox_events SET published_at=$2, attempts=attempts+1 WHERE id=$1 AND published_at IS NULL`, id, publishedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) ReserveExecution(record store.ExecutionRecord, audit domain.AuditEvent, outbox domain.OutboxEvent) (store.ExecutionRecord, bool, error) {
	ctx := context.Background()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return store.ExecutionRecord{}, false, err
	}
	defer tx.Rollback(ctx)
	record.Status = store.ExecutionReserved
	record.StateVersion = 1
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return store.ExecutionRecord{}, false, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO executions (id,run_id,service,adapter,idempotency_key,status,external_execution_id,payload,state_version,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11)`, record.ID, record.RunID, record.Service, record.Adapter,
		record.IdempotencyKey, record.Status, record.ExternalExecutionID, payload, record.StateVersion, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			existing, getErr := s.GetExecution(record.Adapter, record.IdempotencyKey)
			return existing, false, getErr
		}
		return store.ExecutionRecord{}, false, err
	}
	if err := insertEvents(ctx, tx, []domain.AuditEvent{audit}, []domain.OutboxEvent{outbox}); err != nil {
		return store.ExecutionRecord{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.ExecutionRecord{}, false, err
	}
	return record, true, nil
}

func (s *Store) CompleteExecution(record store.ExecutionRecord, expectedVersion int64, audit domain.AuditEvent, outbox domain.OutboxEvent) error {
	ctx := context.Background()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE executions SET status=$3,external_execution_id=NULLIF($4,''),payload=$5,state_version=$6,updated_at=$7 WHERE adapter=$1 AND idempotency_key=$2 AND state_version=$8`,
		record.Adapter, record.IdempotencyKey, record.Status, record.ExternalExecutionID, payload, expectedVersion+1, record.UpdatedAt, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrConflict
	}
	if err := insertEvents(ctx, tx, []domain.AuditEvent{audit}, []domain.OutboxEvent{outbox}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetExecution(adapter, idempotencyKey string) (store.ExecutionRecord, error) {
	var record store.ExecutionRecord
	var payload []byte
	err := s.pool.QueryRow(context.Background(), `SELECT id,run_id,service,adapter,idempotency_key,status,COALESCE(external_execution_id,''),payload,state_version,created_at,updated_at FROM executions WHERE adapter=$1 AND idempotency_key=$2`, adapter, idempotencyKey).Scan(
		&record.ID, &record.RunID, &record.Service, &record.Adapter, &record.IdempotencyKey, &record.Status, &record.ExternalExecutionID, &payload, &record.StateVersion, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ExecutionRecord{}, store.ErrNotFound
	}
	if err != nil {
		return store.ExecutionRecord{}, err
	}
	if err := json.Unmarshal(payload, &record.Payload); err != nil {
		return store.ExecutionRecord{}, err
	}
	return record, nil
}
