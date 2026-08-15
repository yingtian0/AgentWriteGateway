package postgres

const (
	insertRunSQL = `
INSERT INTO release_runs (
  id, request_id, workflow_id, temporal_run_id, release_version, environment,
  requested_by, status, state_version, payload, created_at, updated_at
) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12)`

	selectRunSQL          = `SELECT payload FROM release_runs WHERE id = $1`
	selectRunByRequestSQL = `SELECT payload FROM release_runs WHERE request_id = $1`

	updateRunSQL = `
UPDATE release_runs SET
  temporal_run_id=NULLIF($2,''), status=$3, state_version=$4, payload=$5, updated_at=$6
WHERE id=$1 AND state_version=$7`

	upsertStepSQL = `
INSERT INTO release_steps (run_id,service,environment,phase,status,state_version,payload,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (run_id,service) DO UPDATE SET
  phase=EXCLUDED.phase, status=EXCLUDED.status, state_version=EXCLUDED.state_version,
  payload=EXCLUDED.payload, updated_at=EXCLUDED.updated_at`

	insertAuditSQL = `
INSERT INTO audit_events (
  id,correlation_id,actor_type,actor_id,delegated_by,action,resource_type,
  resource_id,result,details,occurred_at
) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11)`

	insertOutboxSQL = `
INSERT INTO outbox_events (
  id,aggregate_type,aggregate_id,event_type,payload,attempts,available_at,published_at,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
)
