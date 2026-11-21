package task

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
)

// Run intentionally omits message/command bodies and remote error text.
type Run struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"task_id"`
	TaskName    string     `json:"task_name"`
	Revision    string     `json:"revision"`
	ActionKind  string     `json:"action_kind"`
	TargetID    string     `json:"target_id"`
	Trigger     string     `json:"trigger"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Status      string     `json:"status"`
	ErrorCode   string     `json:"error_code"`
	Message     string     `json:"message"`
}

type repository struct{ db storage.Database }

const columns = `id,task_id,task_name,revision,action_kind,target_id,trigger,scheduled_at,created_at,started_at,finished_at,status,error_code,message`

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var run Run
	var due, created int64
	var started, finished sql.NullInt64
	err := row.Scan(&run.ID, &run.TaskID, &run.TaskName, &run.Revision, &run.ActionKind, &run.TargetID, &run.Trigger, &due, &created, &started, &finished, &run.Status, &run.ErrorCode, &run.Message)
	if err != nil {
		return Run{}, err
	}
	run.ScheduledAt, run.CreatedAt = time.UnixMilli(due).UTC(), time.UnixMilli(created).UTC()
	if started.Valid {
		at := time.UnixMilli(started.Int64).UTC()
		run.StartedAt = &at
	}
	if finished.Valid {
		at := time.UnixMilli(finished.Int64).UTC()
		run.FinishedAt = &at
	}
	return run, nil
}

func (r repository) get(ctx context.Context, id string) (Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, "SELECT "+columns+" FROM task_runs WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, fault.New("NOT_FOUND", "没有找到这次任务记录")
	}
	return run, err
}

func (r repository) list(ctx context.Context, taskID string, limit, offset int) ([]Run, error) {
	query := "SELECT " + columns + " FROM task_runs"
	args := []any{}
	if taskID != "" {
		query += " WHERE task_id=?"
		args = append(args, taskID)
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

// Claims and durable high-water marks commit before any effect. Manual request
// timestamps have a separate cursor, so log pruning followed by clock rollback
// cannot resurrect an old request. Configuration task IDs cannot contain ':'.
func (r repository) insert(ctx context.Context, run Run, key string, cursorAt time.Time) (bool, error) {
	inserted := false
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		cursorID := run.TaskID
		if run.Trigger == "manual" {
			cursorID = "manual:" + run.TaskID
		}
		{
			var due int64
			var slot string
			err := tx.QueryRowContext(ctx, "SELECT last_due_at,last_slot FROM task_cursors WHERE task_id=?", cursorID).Scan(&due, &slot)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil && (due >= cursorAt.UnixMilli() || slot == key) {
				return nil
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO task_cursors(task_id,last_due_at,last_slot) VALUES(?,?,?)
				ON CONFLICT(task_id) DO UPDATE SET last_due_at=excluded.last_due_at,last_slot=excluded.last_slot`, cursorID, cursorAt.UnixMilli(), key); err != nil {
				return err
			}
		}
		var finished any
		if run.FinishedAt != nil {
			finished = run.FinishedAt.UnixMilli()
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO task_runs(id,task_id,task_name,revision,action_kind,target_id,trigger,request_key,scheduled_at,created_at,finished_at,status,error_code,message)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, run.ID, run.TaskID, run.TaskName, run.Revision, run.ActionKind, run.TargetID, run.Trigger, key, run.ScheduledAt.UnixMilli(), run.CreatedAt.UnixMilli(), finished, run.Status, run.ErrorCode, run.Message)
		inserted = err == nil
		return err
	})
	return inserted && err == nil, err
}

func (r repository) start(ctx context.Context, run Run) error {
	result, err := r.db.ExecContext(ctx, "UPDATE task_runs SET status='running',started_at=? WHERE id=? AND status='queued'", run.StartedAt.UnixMilli(), run.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("task run is no longer queued")
	}
	return err
}

func (r repository) finish(ctx context.Context, run Run) error {
	result, err := r.db.ExecContext(ctx, "UPDATE task_runs SET status=?,finished_at=?,error_code=?,message=? WHERE id=? AND finished_at IS NULL", run.Status, run.FinishedAt.UnixMilli(), run.ErrorCode, run.Message, run.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("task run was already finished or removed")
	}
	return err
}

func (r repository) recover(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE task_runs SET finished_at=?,
		status=CASE WHEN status='queued' THEN 'canceled' ELSE 'unknown' END,
		error_code=CASE WHEN status='queued' THEN 'CANCELED' ELSE 'RESULT_UNKNOWN' END,
		message=CASE WHEN status='queued' THEN 'Core 重启，尚未发送的任务已取消' ELSE 'Core 在记录结果前停止；请确认目标状态，未自动重试' END
		WHERE status IN ('queued','running')`, time.Now().UnixMilli())
	return err
}

func (r repository) cleanup(ctx context.Context) error {
	now := time.Now()
	if _, err := r.db.ExecContext(ctx, "DELETE FROM task_runs WHERE created_at<? AND finished_at IS NOT NULL", now.Add(-30*24*time.Hour).UnixMilli()); err != nil {
		return err
	}
	// Preserve the most recent 10,000 records, all active runs, and every record
	// young enough to accept a manual request ID again (five minutes plus skew).
	_, err := r.db.ExecContext(ctx, `DELETE FROM task_runs WHERE finished_at IS NOT NULL AND created_at<?
		AND id NOT IN (SELECT id FROM task_runs ORDER BY created_at DESC,id DESC LIMIT 10000)`, now.Add(-10*time.Minute).UnixMilli())
	return err
}
