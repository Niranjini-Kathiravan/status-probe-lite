package store

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			timeout_ms INTEGER DEFAULT 4000,
			created_at TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			ts TIMESTAMP NOT NULL,
			status_code INTEGER NOT NULL,
			ok INTEGER NOT NULL,
			latency_ms INTEGER NOT NULL,
			error TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS outages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			started_at TIMESTAMP NOT NULL,
			ended_at TIMESTAMP,
			reason TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			api_key TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP NOT NULL
		);`,
		// NEW: logs table
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL,
			check_id INTEGER,
			ts TIMESTAMP NOT NULL,
			level TEXT NOT NULL,
			line TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_logs_target_ts ON logs(target_id, ts DESC);`,
	}
	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// targets

type TargetRow struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	TimeoutMs int       `json:"timeout_ms"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) InsertTarget(ctx context.Context, name, url string, timeoutMs int) (int64, error) {
	now := time.Now().UTC()
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO targets(name,url,timeout_ms,created_at) VALUES(?,?,?,?)`,
		name, url, timeoutMs, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListTargets(ctx context.Context) ([]TargetRow, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,name,url,timeout_ms,created_at FROM targets ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TargetRow
	for rows.Next() {
		var t TargetRow
		if err := rows.Scan(&t.ID, &t.Name, &t.URL, &t.TimeoutMs, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTarget(ctx context.Context, id int64) error {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM checks  WHERE target_id=?`, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM outages WHERE target_id=?`, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM logs    WHERE target_id=?`, id)
	_, err := s.DB.ExecContext(ctx, `DELETE FROM targets WHERE id=?`, id)
	return err
}

// checks

type CheckRow struct {
	ID         int64
	TargetID   int64
	TS         time.Time
	StatusCode int
	OK         bool
	LatencyMs  int
	Error      string
}

func (s *Store) InsertCheck(ctx context.Context, targetID int64, ts time.Time, status int, ok bool, latencyMs int, reason string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO checks(target_id,ts,status_code,ok,latency_ms,error) VALUES(?,?,?,?,?,?)`,
		targetID, ts, status, btoi(ok), latencyMs, reason)
	return err
}

func (s *Store) GetRecentChecks(ctx context.Context, targetID int64, limit int) ([]CheckRow, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,target_id,ts,status_code,ok,latency_ms,error
		 FROM checks WHERE target_id=? ORDER BY ts DESC LIMIT ?`, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CheckRow
	for rows.Next() {
		var r CheckRow
		var okInt int
		if err := rows.Scan(&r.ID, &r.TargetID, &r.TS, &r.StatusCode, &okInt, &r.LatencyMs, &r.Error); err != nil {
			return nil, err
		}
		r.OK = okInt == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// outages

type OutageRow struct {
	ID        int64
	TargetID  int64
	StartedAt time.Time
	EndedAt   sql.NullTime
	Reason    string
}

func (s *Store) GetOpenOutage(ctx context.Context, targetID int64) (*OutageRow, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id,target_id,started_at,ended_at,reason
		 FROM outages WHERE target_id=? AND ended_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`, targetID)
	var o OutageRow
	if err := row.Scan(&o.ID, &o.TargetID, &o.StartedAt, &o.EndedAt, &o.Reason); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

func (s *Store) OpenOutage(ctx context.Context, targetID int64, startedAt time.Time, reason string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO outages(target_id,started_at,reason) VALUES(?,?,?)`,
		targetID, startedAt, reason)
	return err
}

func (s *Store) CloseOutage(ctx context.Context, id int64, endedAt time.Time) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE outages SET ended_at=? WHERE id=?`, endedAt, id)
	return err
}

// aggregates

type ReasonCount struct {
	Reason string
	Count  int64
}

func (s *Store) CountChecksAgg(ctx context.Context, targetID int64, from, to time.Time) (total, success int64, err error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*), SUM(CASE WHEN ok=1 THEN 1 ELSE 0 END)
		 FROM checks WHERE target_id=? AND ts>=? AND ts<?`, targetID, from, to)
	if err := row.Scan(&total, &success); err != nil {
		return 0, 0, err
	}
	return total, success, nil
}

func (s *Store) AvgLatencyOK(ctx context.Context, targetID int64, from, to time.Time) (sql.NullFloat64, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT AVG(latency_ms)
		 FROM checks WHERE target_id=? AND ok=1 AND ts>=? AND ts<?`,
		targetID, from, to)
	var avg sql.NullFloat64
	if err := row.Scan(&avg); err != nil {
		return sql.NullFloat64{}, err
	}
	return avg, nil
}

func (s *Store) FailuresByReason(ctx context.Context, targetID int64, from, to time.Time) ([]ReasonCount, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT error, COUNT(*) FROM checks
		 WHERE target_id=? AND ok=0 AND ts>=? AND ts<? 
		 GROUP BY error ORDER BY COUNT(*) DESC`,
		targetID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReasonCount
	for rows.Next() {
		var rc ReasonCount
		if err := rows.Scan(&rc.Reason, &rc.Count); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

func (s *Store) ListOutagesOverlapping(ctx context.Context, targetID int64, from, to time.Time) ([]OutageRow, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,target_id,started_at,ended_at,reason
		 FROM outages
		 WHERE target_id=?
		   AND NOT (COALESCE(ended_at, ?) <= ? OR started_at >= ?)
		 ORDER BY started_at ASC`,
		targetID, to, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutageRow
	for rows.Next() {
		var r OutageRow
		if err := rows.Scan(&r.ID, &r.TargetID, &r.StartedAt, &r.EndedAt, &r.Reason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// agents

type AgentRow struct {
	ID        int64
	Name      string
	APIKey    string
	CreatedAt time.Time
}

func (s *Store) CreateAgent(ctx context.Context, name, apiKey string) (int64, error) {
	now := time.Now().UTC()
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO agents(name,api_key,created_at) VALUES(?,?,?)`,
		name, apiKey, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FindAgentByKey(ctx context.Context, key string) (*AgentRow, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id,name,api_key,created_at FROM agents WHERE api_key=?`, key)
	var a AgentRow
	if err := row.Scan(&a.ID, &a.Name, &a.APIKey, &a.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// -------- LOGS (NEW) --------

type LogRow struct {
	ID       int64
	TargetID int64
	CheckID  sql.NullInt64
	TS       time.Time
	Level    string
	Line     string
}

func (s *Store) InsertCheckLog(ctx context.Context, targetID int64, checkID *int64, ts time.Time, level, line string) error {
	var cid interface{}
	if checkID != nil {
		cid = *checkID
	} else {
		cid = nil
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO logs(target_id,check_id,ts,level,line) VALUES(?,?,?,?,?)`,
		targetID, cid, ts, level, line)
	return err
}

func (s *Store) ListLogs(ctx context.Context, targetID int64, limit int, before *time.Time) ([]LogRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	if before != nil {
		rows, err = s.DB.QueryContext(ctx,
			`SELECT id,target_id,check_id,ts,level,line
			 FROM logs WHERE target_id=? AND ts<? 
			 ORDER BY ts DESC LIMIT ?`, targetID, *before, limit)
	} else {
		rows, err = s.DB.QueryContext(ctx,
			`SELECT id,target_id,check_id,ts,level,line
			 FROM logs WHERE target_id=? 
			 ORDER BY ts DESC LIMIT ?`, targetID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogRow
	for rows.Next() {
		var r LogRow
		if err := rows.Scan(&r.ID, &r.TargetID, &r.CheckID, &r.TS, &r.Level, &r.Line); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
