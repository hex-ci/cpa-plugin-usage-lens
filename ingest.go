// ingest.go: UsagePlugin event handling. The host calls handleUsage
// synchronously on its usage dispatch thread — it must return immediately, so
// records land in a buffered channel and a worker goroutine persists them.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync/atomic"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var (
	recCh   = make(chan *pluginapi.UsageRecord, 4096)
	dropped atomic.Int64
)

// handleUsage is the UsagePlugin entry point (usage.handle). Fire-and-forget.
func handleUsage(raw []byte) ([]byte, error) {
	var rec pluginapi.UsageRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		fmt.Printf("[usage-lens] usage.handle unmarshal: %v\n", err)
		return errorEnvelope("bad_request", err.Error()), nil
	}
	select {
	case recCh <- &rec:
	default:
		dropped.Add(1)
	}
	return okEnvelope(map[string]any{"queued": true})
}

func runIngestWorker() {
	db := mustDB()
	if db == nil {
		return
	}
	for rec := range recCh {
		// Recover per record so a single bad payload never kills the worker.
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[usage-lens] ingest panic: %v\n%s", r, debug.Stack())
				}
			}()
			persistEvent(db, rec)
		}()
	}
}

func persistEvent(db *sql.DB, rec *pluginapi.UsageRecord) {
	ts := rec.RequestedAt.UnixMilli()
	if ts == 0 {
		// Fall back to now; a zero timestamp breaks range queries.
		ts = nowMillis()
	}
	_, err := db.Exec(`INSERT INTO usage_events(
		ts, provider, model, alias, api_key, auth_id, auth_index, auth_type, source,
		latency_ms, ttft_ms, failed, status_code,
		input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, raw
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ts, rec.Provider, rec.Model, rec.Alias, rec.APIKey, rec.AuthID, rec.AuthIndex, rec.AuthType, rec.Source,
		rec.Latency.Milliseconds(), rec.TTFT.Milliseconds(),
		boolInt(rec.Failed), rec.Failure.StatusCode,
		rec.Detail.InputTokens, rec.Detail.OutputTokens, rec.Detail.ReasoningTokens,
		rec.Detail.CachedTokens, rec.Detail.TotalTokens,
		"",
	)
	if err != nil {
		fmt.Printf("[usage-lens] insert: %v\n", err)
	}
}
