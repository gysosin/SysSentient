package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"sys-sentient/internal/models"

	// Pure Go. The cgo driver made CGO_ENABLED=0 impossible, which meant no
	// static binary and no Windows/macOS/arm64 build without a C toolchain per
	// target — the blocker on shipping an installable package at all.
	_ "modernc.org/sqlite"
)

// driverName is the database/sql name registered by modernc.org/sqlite. The
// cgo driver registered "sqlite3"; keeping this in one place stops the two
// from drifting apart in tests, which is exactly how the swap first broke.
const driverName = "sqlite"

// sqlTimeLayout is the textual form every timestamp is stored in.
//
// Timestamps must never be bound as a raw time.Time. database/sql leaves the
// encoding to the driver, and modernc.org/sqlite writes Go's String() form —
// "2026-09-03 17:52:20.885864934 +0000 UTC" — which SQLite's own date
// functions cannot parse. That silently breaks anything calling datetime() on
// the column, and it is not hypothetical: the timestamp-normalisation
// migration NULLed 158 rows of a nullable column and crashed the daemon
// outright on a NOT NULL one.
//
// Millisecond precision, because SQLite's datetime() accepts at most three
// fractional digits and samples are two seconds apart.
const sqlTimeLayout = "2006-01-02 15:04:05.999"

// sqlTime renders a timestamp in the one form the database stores.
func sqlTime(t time.Time) string {
	return t.UTC().Format(sqlTimeLayout)
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open(driverName, sqliteDSN(dbPath))
	if err != nil {
		return nil, err
	}
	// WAL allows one writer and many concurrent readers. Capping the pool at
	// one connection enabled WAL and then forfeited it: every dashboard read
	// serialised behind every write, which is the first thing that falls over
	// with several agents pushing.
	//
	// SQLite still permits only one writer, so concurrent writes serialise on
	// the busy timeout in the DSN rather than failing.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	// Bounded so a long-lived process does not hold file descriptors for
	// connections it stopped using hours ago.
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := createTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Run migrations for new columns
	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := createAgentTables(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := createRollupTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Indexes over migrated columns, once those columns exist.
	if err := createPostMigrationIndexes(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := createAuthTables(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Rewrite any pre-existing local-time timestamps into UTC.
	if err := normalizeStoredTimestamps(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func sqliteDSN(dbPath string) string {
	// modernc.org/sqlite takes pragmas as `_pragma=name(value)`; the cgo driver
	// used `_journal_mode=WAL` style keys. Silently different: unknown query
	// parameters are ignored rather than rejected, so getting this wrong loses
	// WAL and the busy timeout without any error at open time.
	return dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)"
}

func createTable(db *sql.DB) error {
	var err error
	queryMetrics := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		cpu_usage REAL,
		memory_used INTEGER,
		memory_total INTEGER,
		disk_read_bytes INTEGER,
		disk_write_bytes INTEGER,
		net_sent_bytes INTEGER,
		net_recv_bytes INTEGER,
		temperature REAL,
		top_processes TEXT,
		processes TEXT
	);
	`
	if _, err = db.Exec(queryMetrics); err != nil {
		return err
	}

	// Index for faster time-based queries
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);`); err != nil {
		return err
	}

	queryInsights := `
	CREATE TABLE IF NOT EXISTS insights (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		content TEXT
	);
	`
	if _, err = db.Exec(queryInsights); err != nil {
		return err
	}

	// insights is queried with ORDER BY timestamp DESC on every dashboard poll
	// and had no index at all — a full scan plus sort every time.
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_insights_timestamp ON insights(timestamp);`); err != nil {
		return fmt.Errorf("failed to create insights index: %w", err)
	}

	queryAlertEvents := `
	CREATE TABLE IF NOT EXISTS alert_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		occurred_at DATETIME NOT NULL,
		rule_id TEXT NOT NULL,
		rule_name TEXT NOT NULL,
		metric TEXT NOT NULL,
		state TEXT NOT NULL,
		severity TEXT NOT NULL,
		value REAL NOT NULL,
		threshold REAL NOT NULL,
		hostname TEXT NOT NULL DEFAULT '',
		host_id TEXT NOT NULL DEFAULT ''
	);
	`
	if _, err = db.Exec(queryAlertEvents); err != nil {
		return fmt.Errorf("failed to create alert_events table: %w", err)
	}

	// Existing installs predate host_id. Added separately from the CREATE
	// above, which only runs for a database that does not exist yet.
	if err := addColumnIfMissing(db, "alert_events", "host_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_alert_events_host ON alert_events(host_id, occurred_at);`); err != nil {
		return fmt.Errorf("failed to index alert_events by host: %w", err)
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_alert_events_occurred ON alert_events(occurred_at);`); err != nil {
		return fmt.Errorf("failed to create alert events index: %w", err)
	}

	// Fleet inventory. Kept separate from metrics so a host stays visible (and
	// its last-seen age meaningful) after its samples have been pruned.
	queryHosts := `
	CREATE TABLE IF NOT EXISTS hosts (
		host_id TEXT PRIMARY KEY,
		hostname TEXT NOT NULL DEFAULT '',
		first_seen DATETIME NOT NULL,
		last_seen DATETIME NOT NULL,
		agent_version TEXT NOT NULL DEFAULT '',
		labels TEXT NOT NULL DEFAULT '{}'
	);
	`
	if _, err = db.Exec(queryHosts); err != nil {
		return fmt.Errorf("failed to create hosts table: %w", err)
	}

	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_hosts_last_seen ON hosts(last_seen);`); err != nil {
		return fmt.Errorf("failed to create hosts index: %w", err)
	}

	return nil
}

// createPostMigrationIndexes builds indexes over columns that are added by
// migrateSchema, so they cannot be created in createTable — at that point the
// columns do not exist yet on an upgraded database.
func createPostMigrationIndexes(db *sql.DB) error {
	// Host-scoped time queries are the common access pattern once more than one
	// machine reports; a bare timestamp index makes them scan every host.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_metrics_host_time ON metrics(host_id, timestamp);`); err != nil {
		return fmt.Errorf("failed to create metrics host/time index: %w", err)
	}
	return nil
}

// normalizeStoredTimestamps rewrites timestamps that were persisted as local
// wall time with a UTC offset into plain UTC, so all rows compare and sort
// consistently against datetime('now').
//
// SQLite's datetime() parses the offset form and returns UTC, so the conversion
// is done in SQL. Rows already in the plain UTC form contain no '+' and no
// trailing offset, and are left untouched — the WHERE clause makes this
// idempotent and cheap on subsequent starts.
func normalizeStoredTimestamps(db *sql.DB) error {
	statements := []struct {
		table  string
		column string
	}{
		{table: "metrics", column: "timestamp"},
		{table: "insights", column: "timestamp"},
		{table: "alert_events", column: "occurred_at"},
	}

	for _, stmt := range statements {
		// SQL identifiers cannot be bound as parameters. Both values come from
		// the hardcoded list above — never from config, a request or the
		// database — so there is no injection surface here.
		// COALESCE is load-bearing: datetime() returns NULL for anything it
		// cannot parse, and writing that back either destroys the value (on a
		// nullable column) or aborts the migration and stops the daemon
		// booting (on a NOT NULL one). Both happened. An unparseable value is
		// now left exactly as it was.
		//
		// The trailing rtrim handles Go's time.String() form,
		// "2026-09-03 17:52:20.885864934 +0000 UTC", which earlier builds
		// stored: strip the zone suffix and the sub-second digits SQLite will
		// not accept, then let datetime() parse what remains.
		query := fmt.Sprintf( // #nosec G201 -- identifiers from the fixed list above
			`UPDATE %s
			    SET %s = COALESCE(
			              datetime(%s),
			              datetime(substr(%s, 1, 19)),
			              %s)
			  WHERE %s LIKE '%%+%%' OR %s LIKE '%%Z' OR %s LIKE '%%_-__:__' OR %s LIKE '%% UTC'`,
			stmt.table,
			stmt.column, stmt.column, stmt.column, stmt.column,
			stmt.column, stmt.column, stmt.column, stmt.column,
		)
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to normalize %s.%s: %w", stmt.table, stmt.column, err)
		}
	}
	return nil
}

// migrateSchema adds new columns to existing tables
func migrateSchema(db *sql.DB) error {
	// List of new columns to add if they don't exist
	newColumns := []struct {
		name       string
		columnType string
	}{
		{"cpu_per_core", "TEXT"},
		{"swap_used", "INTEGER DEFAULT 0"},
		{"swap_total", "INTEGER DEFAULT 0"},
		{"disk_iops", "REAL DEFAULT 0"},
		{"load_avg_1", "REAL DEFAULT 0"},
		{"load_avg_5", "REAL DEFAULT 0"},
		{"load_avg_15", "REAL DEFAULT 0"},
		{"temperature", "REAL DEFAULT 0"},
		{"processes", "TEXT DEFAULT '[]'"},
		{"uptime_seconds", "INTEGER DEFAULT 0"},
		{"hostname", "TEXT DEFAULT ''"},
		{"filesystems", "TEXT DEFAULT '[]'"},
		{"host_id", "TEXT DEFAULT ''"},
		{"memory_cached", "INTEGER DEFAULT 0"},
		{"memory_buffers", "INTEGER DEFAULT 0"},
		{"process_count", "INTEGER DEFAULT 0"},
	}

	for _, col := range newColumns {
		exists, err := columnExists(db, "metrics", col.name)
		if err != nil {
			return fmt.Errorf("failed to inspect metrics.%s: %w", col.name, err)
		}
		if exists {
			continue
		}

		query := fmt.Sprintf("ALTER TABLE metrics ADD COLUMN %s %s", col.name, col.columnType)
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to add metrics.%s: %w", col.name, err)
		}
	}
	return nil
}

// addColumnIfMissing widens an existing table in place.
//
// SQLite has no ADD COLUMN IF NOT EXISTS, and re-adding a column is an error
// rather than a no-op, so every migration has to ask first.
func addColumnIfMissing(db *sql.DB, table, column, columnType string) error {
	exists, err := columnExists(db, table, column)
	if err != nil {
		return fmt.Errorf("failed to inspect %s.%s: %w", table, column, err)
	}
	if exists {
		return nil
	}

	// SQL identifiers cannot be bound as parameters. Every argument reaching
	// here is a literal from this package -- never config, a request, or the
	// database -- so there is no injection surface.
	if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType)); err != nil {
		return fmt.Errorf("failed to add %s.%s: %w", table, column, err)
	}
	return nil
}

func columnExists(db *sql.DB, table, columnName string) (bool, error) {
	// Same reasoning as above: table is a package literal.
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	// Rows are fully consumed below; a close error adds nothing.
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

// execer is satisfied by both *sql.DB and *sql.Tx, so one insert body serves
// the single-sample and batched paths and they cannot drift apart.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Save stores one sample.
func (s *Store) Save(m *models.SystemState) error {
	return saveTx(s.db, m)
}

func saveTx(db execer, m *models.SystemState) error {
	cpuPerCoreJSON, err := json.Marshal(m.CPUPerCore)
	if err != nil {
		return fmt.Errorf("failed to marshal cpu_per_core: %w", err)
	}
	processes := m.Processes
	if processes == nil {
		processes = []models.Process{}
	}
	processesJSON, err := json.Marshal(processes)
	if err != nil {
		return fmt.Errorf("failed to marshal processes: %w", err)
	}
	filesystemsJSON, err := marshalFilesystems(m.Filesystems)
	if err != nil {
		return fmt.Errorf("failed to marshal filesystems: %w", err)
	}

	query := `
	INSERT INTO metrics (
		timestamp, cpu_usage, cpu_per_core, memory_used, memory_total,
		swap_used, swap_total, disk_read_bytes, disk_write_bytes, disk_iops,
		net_sent_bytes, net_recv_bytes, load_avg_1, load_avg_5, load_avg_15,
		temperature, top_processes, processes, uptime_seconds, hostname, filesystems, host_id,
		memory_cached, memory_buffers, process_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = db.Exec(query,
		// Stored through one explicit layout. Binding a raw time.Time lets the
		// driver choose, and modernc.org/sqlite writes Go's String() form,
		// which SQLite's own date functions cannot parse.
		sqlTime(m.Timestamp), m.CPUUsage, string(cpuPerCoreJSON), m.MemoryUsed, m.MemoryTotal,
		m.SwapUsed, m.SwapTotal, m.DiskReadBytes, m.DiskWriteBytes, m.DiskIOPS,
		m.NetSentBytes, m.NetRecvBytes, m.LoadAvg1, m.LoadAvg5, m.LoadAvg15,
		// top_processes is written empty and derived on read: it is a pure
		// function of processes, and storing both wrote the same information
		// twice on every sample. The column stays for rows written earlier.
		m.Temperature, "", string(processesJSON), m.UptimeSeconds, m.Hostname, string(filesystemsJSON), m.HostID,
		m.MemoryCached, m.MemoryBuffers, m.ProcessCount,
	)
	return err
}

func (s *Store) PruneOldMetrics(hours int) error {
	query := `DELETE FROM metrics WHERE timestamp < datetime('now', ?)`
	// SQLite modifier: '-24 hours'
	modifier := fmt.Sprintf("-%d hours", hours)
	_, err := s.db.Exec(query, modifier)
	return err
}

func (s *Store) PruneOldInsights(hours int) error {
	query := `DELETE FROM insights WHERE timestamp < datetime('now', ?)`
	modifier := fmt.Sprintf("-%d hours", hours)
	_, err := s.db.Exec(query, modifier)
	return err
}

func (s *Store) SaveInsight(content string) error {
	query := `INSERT INTO insights (timestamp, content) VALUES (CURRENT_TIMESTAMP, ?)`
	_, err := s.db.Exec(query, content)
	return err
}

func (s *Store) GetRecent(limit int) ([]models.SystemState, error) {
	if limit < 1 {
		return []models.SystemState{}, nil
	}

	query := `SELECT timestamp, cpu_usage, COALESCE(cpu_per_core, '[]'),
		memory_used, memory_total, COALESCE(swap_used, 0), COALESCE(swap_total, 0),
		disk_read_bytes, disk_write_bytes, COALESCE(disk_iops, 0),
		net_sent_bytes, net_recv_bytes, COALESCE(load_avg_1, 0), COALESCE(load_avg_5, 0), COALESCE(load_avg_15, 0),
		temperature, top_processes, COALESCE(processes, '[]'), COALESCE(uptime_seconds, 0), COALESCE(hostname, ''), COALESCE(filesystems, '[]'), COALESCE(host_id, ''),
		COALESCE(memory_cached, 0), COALESCE(memory_buffers, 0), COALESCE(process_count, 0)
		FROM metrics ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	// Rows are fully consumed below; a close error adds nothing.
	defer func() { _ = rows.Close() }()

	return scanMetricRows(rows)
}

func decodeCPUPerCore(raw string) []float64 {
	var values []float64
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return []float64{}
	}
	return values
}

func decodeFilesystems(raw string) []models.Filesystem {
	return unmarshalFilesystems([]byte(raw))
}

func decodeProcesses(raw string) []models.Process {
	var values []models.Process
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return []models.Process{}
	}
	return values
}

type Insight struct {
	Timestamp string
	Content   string
}

func (s *Store) GetRecentInsights(limit int) ([]Insight, error) {
	if limit < 1 {
		return []Insight{}, nil
	}

	query := `SELECT timestamp, content FROM insights ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	// Rows are fully consumed below; a close error adds nothing.
	defer func() { _ = rows.Close() }()

	// Return an empty slice, not nil: nil marshals to JSON `null`, which forces
	// every client to defensively null-check an endpoint that returns a list.
	results := make([]Insight, 0, limit)
	for rows.Next() {
		var i Insight
		if err := rows.Scan(&i.Timestamp, &i.Content); err != nil {
			return nil, err
		}
		results = append(results, i)
	}
	return results, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	return s.db.Ping()
}

// AlertEvent is one persisted alert transition.
type AlertEvent struct {
	OccurredAt time.Time `json:"occurred_at"`
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	Metric     string    `json:"metric"`
	State      string    `json:"state"`
	Severity   string    `json:"severity"`
	Value      float64   `json:"value"`
	Threshold  float64   `json:"threshold"`
	Hostname   string    `json:"hostname"`
	// HostID identifies the machine. Hostname alone cannot: two hosts sharing
	// a name render as duplicate alerts and their history cannot be joined
	// back to the hosts table.
	HostID string `json:"host_id"`
}

// SaveAlertEvent records one alert transition. Only transitions are stored, so
// a rule breached for an hour produces two rows (fired, resolved) rather than
// one per poll.
func (s *Store) SaveAlertEvent(event AlertEvent) error {
	_, err := s.db.Exec(`
		INSERT INTO alert_events (occurred_at, rule_id, rule_name, metric, state, severity, value, threshold, hostname, host_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sqlTime(event.OccurredAt), event.RuleID, event.RuleName, event.Metric,
		event.State, event.Severity, event.Value, event.Threshold, event.Hostname, event.HostID,
	)
	return err
}

// GetRecentAlertEvents returns the newest alert transitions first.
func (s *Store) GetRecentAlertEvents(limit int) ([]AlertEvent, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT occurred_at, rule_id, rule_name, metric, state, severity, value, threshold, hostname, COALESCE(host_id, '')
		FROM alert_events ORDER BY occurred_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	// Rows are fully consumed below; a close error adds nothing.
	defer func() { _ = rows.Close() }()

	events := make([]AlertEvent, 0, limit)
	for rows.Next() {
		var e AlertEvent
		if err := rows.Scan(&e.OccurredAt, &e.RuleID, &e.RuleName, &e.Metric,
			&e.State, &e.Severity, &e.Value, &e.Threshold, &e.Hostname, &e.HostID); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// PruneOldAlertEvents drops alert history beyond the retention window.
func (s *Store) PruneOldAlertEvents(retentionHours int) error {
	if retentionHours <= 0 {
		return nil
	}
	_, err := s.db.Exec(
		`DELETE FROM alert_events WHERE occurred_at < ?`,
		sqlTime(time.Now().Add(-time.Duration(retentionHours)*time.Hour)),
	)
	return err
}

// Host is one machine in the fleet inventory.
type Host struct {
	HostID       string    `json:"host_id"`
	Hostname     string    `json:"hostname"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	AgentVersion string    `json:"agent_version"`
}

// UpsertHost records that a host reported, creating it on first contact.
//
// first_seen is preserved via COALESCE on conflict so a long-running host keeps
// its original enrolment time.
func (s *Store) UpsertHost(hostID, hostname, agentVersion string, seenAt time.Time) error {
	if hostID == "" {
		// Refuse to create an unattributable inventory row; a sample without a
		// host id is still stored, it just does not register a host.
		return nil
	}

	_, err := s.db.Exec(`
		INSERT INTO hosts (host_id, hostname, first_seen, last_seen, agent_version)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(host_id) DO UPDATE SET
			hostname = excluded.hostname,
			last_seen = excluded.last_seen,
			agent_version = excluded.agent_version`,
		hostID, hostname, sqlTime(seenAt), sqlTime(seenAt), agentVersion,
	)
	return err
}

// ListHosts returns the fleet inventory, most recently seen first.
func (s *Store) ListHosts() ([]Host, error) {
	rows, err := s.db.Query(`
		SELECT host_id, hostname, first_seen, last_seen, agent_version
		FROM hosts ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	// Rows are fully consumed below; a close error adds nothing.
	defer func() { _ = rows.Close() }()

	hosts := make([]Host, 0, 8)
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.HostID, &h.Hostname, &h.FirstSeen, &h.LastSeen, &h.AgentVersion); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// GetRecentForHost returns recent samples for one host.
//
// An empty hostID means "any host", which keeps single-node deployments and
// pre-multi-host databases working unchanged.
func (s *Store) GetRecentForHost(hostID string, limit int) ([]models.SystemState, error) {
	if hostID == "" {
		return s.GetRecent(limit)
	}
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT timestamp, cpu_usage, COALESCE(cpu_per_core, '[]'),
		memory_used, memory_total, COALESCE(swap_used, 0), COALESCE(swap_total, 0),
		disk_read_bytes, disk_write_bytes, COALESCE(disk_iops, 0),
		net_sent_bytes, net_recv_bytes, COALESCE(load_avg_1, 0), COALESCE(load_avg_5, 0), COALESCE(load_avg_15, 0),
		temperature, top_processes, COALESCE(processes, '[]'), COALESCE(uptime_seconds, 0),
		COALESCE(hostname, ''), COALESCE(filesystems, '[]'), COALESCE(host_id, ''),
		COALESCE(memory_cached, 0), COALESCE(memory_buffers, 0), COALESCE(process_count, 0)
		FROM metrics WHERE host_id = ? ORDER BY timestamp DESC LIMIT ?`

	rows, err := s.db.Query(query, hostID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanMetricRows(rows)
}

// SaveBatch stores many samples in one transaction.
//
// The ingest handler previously called Save once per sample, so a batch of 60
// from one agent was 60 separate autocommit transactions — 60 fsyncs, each
// contending for the single write lock. With several agents pushing
// concurrently that is the first thing to fall over.
//
// Returns the number stored. A sample that fails is skipped rather than
// aborting the batch: one malformed row from one agent must not discard the
// other fifty-nine, and the caller counts rejections either way.
func (s *Store) SaveBatch(samples []*models.SystemState) (int, error) {
	if len(samples) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin batch: %w", err)
	}
	// Rollback is a no-op once Commit succeeds, so this is safe unconditionally
	// and covers every early return.
	defer func() { _ = tx.Rollback() }()

	stored := 0
	for _, m := range samples {
		if err := saveTx(tx, m); err != nil {
			continue
		}
		stored++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit batch: %w", err)
	}
	return stored, nil
}
