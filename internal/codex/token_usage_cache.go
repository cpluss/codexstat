package codex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const tokenUsageCacheVersion = 2

type tokenUsageFileInfo struct {
	SessionKey      string
	SessionDate     string
	Path            string
	Size            int64
	ModTimeUnixNano int64
}

type cachedTokenUsageFileInfo struct {
	Size            int64
	ModTimeUnixNano int64
}

type scannedTokenUsageFile struct {
	Info   tokenUsageFileInfo
	Events int
	Days   []tokenUsageDayAggregate
}

type tokenUsageDayAggregate struct {
	Date   string
	Events int
	Tokens TokenUsageTotal
}

func DefaultTokenUsageCachePath(env map[string]string) (string, error) {
	if env == nil {
		env = environMap()
	}
	if path := strings.TrimSpace(env["CODEXSTAT_TOKEN_USAGE_CACHE"]); path != "" {
		return expandHome(path, env), nil
	}
	if base := strings.TrimSpace(env["XDG_CACHE_HOME"]); base != "" {
		return filepath.Join(expandHome(base, env), "codexstat", "token_usage.sqlite"), nil
	}
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Caches", "codexstat", "token_usage.sqlite"), nil
	}
	return filepath.Join(home, ".cache", "codexstat", "token_usage.sqlite"), nil
}

func buildCachedTokenUsageReport(query TokenUsageQuery) (TokenUsageReport, error) {
	metricInput := query.Metric
	query = normalizeTokenUsageQuery(query)
	if !isTokenHistoryMetric(query.Metric) {
		return TokenUsageReport{}, fmt.Errorf("unknown token metric %q", metricInput)
	}
	empty := EmptyTokenUsageReport(query)
	if query.Env == nil {
		query.Env = environMap()
	}

	roots, err := codexSessionRoots(query)
	if err != nil {
		return empty, err
	}

	cachePath := strings.TrimSpace(query.CachePath)
	if cachePath == "" {
		cachePath, err = DefaultTokenUsageCachePath(query.Env)
		if err != nil {
			empty.Roots = roots
			return empty, err
		}
	}
	db, err := openTokenUsageCache(cachePath)
	if err != nil {
		empty.Roots = roots
		return empty, err
	}
	defer db.Close()

	scanSince, scanUntil, err := tokenUsageCacheRefreshRange(db, query.Now)
	if err != nil {
		empty.Roots = roots
		return empty, err
	}

	reportTokenUsageProgress(query, TokenUsageProgress{Phase: "discovering"})
	paths, err := listCodexTokenFiles(roots, scanSince, scanUntil)
	if err != nil {
		empty.Roots = roots
		return empty, err
	}
	files, err := tokenUsageFileInfos(paths)
	if err != nil {
		empty.Roots = roots
		return empty, err
	}

	reportTokenUsageProgress(query, TokenUsageProgress{
		Phase:      "scanning",
		FilesTotal: len(files),
	})
	if err := refreshTokenUsageCache(db, files, scanSince, scanUntil, query); err != nil {
		empty.Roots = roots
		return empty, err
	}
	report, err := tokenUsageReportFromCache(db, query, roots)
	if err != nil {
		empty.Roots = roots
		return empty, err
	}
	reportTokenUsageProgress(query, TokenUsageProgress{
		Phase:         "done",
		FilesTotal:    len(files),
		FilesVisited:  len(files),
		FilesScanned:  report.FilesScanned,
		EventsScanned: report.EventsScanned,
		Done:          true,
	})
	return report, nil
}

func tokenUsageFileInfos(paths []string) ([]tokenUsageFileInfo, error) {
	seenBase := make(map[string]bool)
	files := make([]tokenUsageFileInfo, 0, len(paths))
	for _, path := range paths {
		base := filepath.Base(path)
		if seenBase[base] {
			continue
		}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		seenBase[base] = true
		files = append(files, tokenUsageFileInfo{
			SessionKey:      base,
			SessionDate:     dateKeyFromCodexSessionPath(path),
			Path:            path,
			Size:            info.Size(),
			ModTimeUnixNano: info.ModTime().UnixNano(),
		})
	}
	return files, nil
}

func openTokenUsageCache(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("missing token usage cache path")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" {
		if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := ensureTokenUsageCacheSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureTokenUsageCacheSchema(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > tokenUsageCacheVersion {
		return fmt.Errorf("unsupported token usage cache version %d", version)
	}
	if version == 1 {
		if err := migrateTokenUsageCacheV1ToV2(db); err != nil {
			return err
		}
		version = 2
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS files (
			session_key TEXT PRIMARY KEY,
			session_date TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL,
			size INTEGER NOT NULL,
			mod_time_unix_nano INTEGER NOT NULL,
			events INTEGER NOT NULL,
			scanned_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS day_sessions (
			session_key TEXT NOT NULL,
			date TEXT NOT NULL,
			events INTEGER NOT NULL,
			input_tokens INTEGER NOT NULL,
			cached_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			reasoning_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			PRIMARY KEY (session_key, date),
			FOREIGN KEY (session_key) REFERENCES files(session_key) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_day_sessions_date ON day_sessions(date)`,
		fmt.Sprintf(`PRAGMA user_version = %d`, tokenUsageCacheVersion),
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func migrateTokenUsageCacheV1ToV2(db *sql.DB) error {
	if _, err := db.Exec(`ALTER TABLE files ADD COLUMN session_date TEXT NOT NULL DEFAULT ''`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	_, err := db.Exec(`PRAGMA user_version = 2`)
	return err
}

func tokenUsageCacheRefreshRange(db *sql.DB, now time.Time) (time.Time, time.Time, error) {
	var lastCached sql.NullString
	if err := db.QueryRow(`SELECT MAX(date) FROM day_sessions`).Scan(&lastCached); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !lastCached.Valid || strings.TrimSpace(lastCached.String) == "" {
		return time.Time{}, time.Time{}, nil
	}
	lastDay, err := time.ParseInLocation("2006-01-02", lastCached.String, now.Local().Location())
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	today := startOfLocalDay(now.Local())
	start := lastDay.AddDate(0, 0, -1)
	if start.After(today) {
		start = today.AddDate(0, 0, -1)
	}
	return start, today.AddDate(0, 0, 1), nil
}

func refreshTokenUsageCache(db *sql.DB, files []tokenUsageFileInfo, scanSince, scanUntil time.Time, query TokenUsageQuery) error {
	ctx := context.Background()
	cached, err := loadCachedTokenUsageFiles(ctx, db)
	if err != nil {
		return err
	}

	scanned := make([]scannedTokenUsageFile, 0)
	filesScanned := 0
	eventsScanned := 0
	for index, file := range files {
		if cachedFile, ok := cached[file.SessionKey]; ok &&
			cachedFile.Size == file.Size &&
			cachedFile.ModTimeUnixNano == file.ModTimeUnixNano {
			reportTokenUsageProgress(query, TokenUsageProgress{
				Phase:         "scanning",
				FilesTotal:    len(files),
				FilesVisited:  index + 1,
				FilesScanned:  filesScanned,
				EventsScanned: eventsScanned,
				CurrentFile:   file.Path,
			})
			continue
		}

		scannedFile, err := scanTokenUsageFileForCache(file)
		if err != nil {
			return err
		}
		scanned = append(scanned, scannedFile)
		if scannedFile.Events > 0 {
			filesScanned++
		}
		eventsScanned += scannedFile.Events
		reportTokenUsageProgress(query, TokenUsageProgress{
			Phase:         "scanning",
			FilesTotal:    len(files),
			FilesVisited:  index + 1,
			FilesScanned:  filesScanned,
			EventsScanned: eventsScanned,
			CurrentFile:   file.Path,
		})
	}
	return writeTokenUsageCacheRefresh(ctx, db, files, scanned, scanSince, scanUntil)
}

func loadCachedTokenUsageFiles(ctx context.Context, db *sql.DB) (map[string]cachedTokenUsageFileInfo, error) {
	rows, err := db.QueryContext(ctx, `SELECT session_key, size, mod_time_unix_nano FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cached := make(map[string]cachedTokenUsageFileInfo)
	for rows.Next() {
		var sessionKey string
		var file cachedTokenUsageFileInfo
		if err := rows.Scan(&sessionKey, &file.Size, &file.ModTimeUnixNano); err != nil {
			return nil, err
		}
		cached[sessionKey] = file
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cached, nil
}

func scanTokenUsageFileForCache(file tokenUsageFileInfo) (scannedTokenUsageFile, error) {
	dayMap := make(map[string]*tokenUsageDayBuilder)
	events, err := scanCodexTokenFile(file.Path, time.Time{}, time.Time{}, dayMap, true)
	if err != nil {
		return scannedTokenUsageFile{}, err
	}

	keys := make([]string, 0, len(dayMap))
	for key := range dayMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	days := make([]tokenUsageDayAggregate, 0, len(keys))
	for _, key := range keys {
		day := dayMap[key].day
		if day.Events == 0 {
			continue
		}
		days = append(days, tokenUsageDayAggregate{
			Date:   key,
			Events: day.Events,
			Tokens: day.Tokens,
		})
	}
	return scannedTokenUsageFile{
		Info:   file,
		Events: events,
		Days:   days,
	}, nil
}

func writeTokenUsageCacheRefresh(ctx context.Context, db *sql.DB, files []tokenUsageFileInfo, scanned []scannedTokenUsageFile, scanSince, scanUntil time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS current_token_usage_files (session_key TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM current_token_usage_files`); err != nil {
		return err
	}
	currentStmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO current_token_usage_files (session_key) VALUES (?)`)
	if err != nil {
		return err
	}
	for _, file := range files {
		if _, err := currentStmt.ExecContext(ctx, file.SessionKey); err != nil {
			currentStmt.Close()
			return err
		}
	}
	if err := currentStmt.Close(); err != nil {
		return err
	}
	if scanSince.IsZero() || scanUntil.IsZero() {
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE session_key NOT IN (SELECT session_key FROM current_token_usage_files)`); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(
			ctx,
			`DELETE FROM files
			WHERE session_date >= ?
				AND session_date < ?
				AND session_key NOT IN (SELECT session_key FROM current_token_usage_files)`,
			scanSince.Format("2006-01-02"),
			scanUntil.Format("2006-01-02"),
		); err != nil {
			return err
		}
	}

	fileStmt, err := tx.PrepareContext(ctx, `INSERT INTO files (
		session_key,
		session_date,
		path,
		size,
		mod_time_unix_nano,
		events,
		scanned_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(session_key) DO UPDATE SET
		session_date = excluded.session_date,
		path = excluded.path,
		size = excluded.size,
		mod_time_unix_nano = excluded.mod_time_unix_nano,
		events = excluded.events,
		scanned_at = excluded.scanned_at`)
	if err != nil {
		return err
	}
	dayStmt, err := tx.PrepareContext(ctx, `INSERT INTO day_sessions (
		session_key,
		date,
		events,
		input_tokens,
		cached_tokens,
		output_tokens,
		reasoning_tokens,
		total_tokens
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		fileStmt.Close()
		return err
	}
	scannedAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, file := range scanned {
		if _, err := fileStmt.ExecContext(
			ctx,
			file.Info.SessionKey,
			file.Info.SessionDate,
			file.Info.Path,
			file.Info.Size,
			file.Info.ModTimeUnixNano,
			file.Events,
			scannedAt,
		); err != nil {
			fileStmt.Close()
			dayStmt.Close()
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM day_sessions WHERE session_key = ?`, file.Info.SessionKey); err != nil {
			fileStmt.Close()
			dayStmt.Close()
			return err
		}
		for _, day := range file.Days {
			if _, err := dayStmt.ExecContext(
				ctx,
				file.Info.SessionKey,
				day.Date,
				day.Events,
				day.Tokens.Input,
				day.Tokens.Cached,
				day.Tokens.Output,
				day.Tokens.Reasoning,
				day.Tokens.Total,
			); err != nil {
				fileStmt.Close()
				dayStmt.Close()
				return err
			}
		}
	}
	if err := fileStmt.Close(); err != nil {
		dayStmt.Close()
		return err
	}
	if err := dayStmt.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE current_token_usage_files`); err != nil {
		return err
	}
	return tx.Commit()
}

func tokenUsageReportFromCache(db *sql.DB, query TokenUsageQuery, roots []string) (TokenUsageReport, error) {
	ctx := context.Background()
	today := startOfLocalDay(query.Now.Local())
	dayMap := make(map[string]*tokenUsageDayBuilder)

	var since, until time.Time
	if !query.All {
		since = today.AddDate(0, 0, -(query.Days - 1))
		until = today.AddDate(0, 0, 1)
		for day := since; day.Before(until); day = day.AddDate(0, 0, 1) {
			key := day.Format("2006-01-02")
			dayMap[key] = newTokenUsageDayBuilder(key)
		}
	}

	rows, err := queryCachedTokenUsageDays(ctx, db, query.All, since, until)
	if err != nil {
		return TokenUsageReport{}, err
	}
	for _, row := range rows {
		dayMap[row.Date] = &tokenUsageDayBuilder{
			day: TokenUsageDay{
				Date:     row.Date,
				Sessions: row.Sessions,
				Events:   row.Events,
				Tokens:   row.Tokens,
			},
		}
	}

	filesScanned, eventsScanned, err := queryCachedTokenUsageTotals(ctx, db, query.All, since, until)
	if err != nil {
		return TokenUsageReport{}, err
	}

	days, total, reportSince, reportUntil := tokenUsageDaysFromMap(dayMap, query.Metric, query.All)
	if !query.All {
		reportSince = since.Format("2006-01-02")
		reportUntil = until.AddDate(0, 0, -1).Format("2006-01-02")
	}

	return TokenUsageReport{
		Metric:        query.Metric,
		Since:         reportSince,
		Until:         reportUntil,
		Roots:         roots,
		FilesScanned:  filesScanned,
		EventsScanned: eventsScanned,
		Total:         total,
		Days:          days,
	}, nil
}

type cachedTokenUsageDayRow struct {
	Date     string
	Sessions int
	Events   int
	Tokens   TokenUsageTotal
}

func queryCachedTokenUsageDays(ctx context.Context, db *sql.DB, all bool, since, until time.Time) ([]cachedTokenUsageDayRow, error) {
	query := `SELECT
		date,
		COUNT(*) AS sessions,
		COALESCE(SUM(events), 0) AS events,
		COALESCE(SUM(input_tokens), 0) AS input_tokens,
		COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		COALESCE(SUM(output_tokens), 0) AS output_tokens,
		COALESCE(SUM(reasoning_tokens), 0) AS reasoning_tokens,
		COALESCE(SUM(total_tokens), 0) AS total_tokens
	FROM day_sessions`
	var args []any
	if !all {
		query += ` WHERE date >= ? AND date < ?`
		args = append(args, since.Format("2006-01-02"), until.Format("2006-01-02"))
	}
	query += ` GROUP BY date ORDER BY date`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]cachedTokenUsageDayRow, 0)
	for rows.Next() {
		var row cachedTokenUsageDayRow
		if err := rows.Scan(
			&row.Date,
			&row.Sessions,
			&row.Events,
			&row.Tokens.Input,
			&row.Tokens.Cached,
			&row.Tokens.Output,
			&row.Tokens.Reasoning,
			&row.Tokens.Total,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func queryCachedTokenUsageTotals(ctx context.Context, db *sql.DB, all bool, since, until time.Time) (int, int, error) {
	query := `SELECT COUNT(DISTINCT session_key), COALESCE(SUM(events), 0) FROM day_sessions`
	var args []any
	if !all {
		query += ` WHERE date >= ? AND date < ?`
		args = append(args, since.Format("2006-01-02"), until.Format("2006-01-02"))
	}
	var files int
	var events int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&files, &events); err != nil {
		return 0, 0, err
	}
	return files, events, nil
}
