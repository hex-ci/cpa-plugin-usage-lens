// query.go: SQL aggregations backing the panel API. All timestamps are Unix
// milliseconds. Cost joins model_pricing; models without a price count 0.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// rangeBounds parses start_ts/end_ts (ms). Defaults to the current calendar day
// so an empty request still shows something sensible.
func rangeBounds(q func(string) string) (int64, int64) {
	start := parseInt64(q("start_ts"))
	end := parseInt64(q("end_ts"))
	if start <= 0 && end <= 0 {
		now := time.Now()
		s := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return s.UnixMilli(), now.UnixMilli()
	}
	if end <= 0 {
		end = nowMillis()
	}
	if start <= 0 {
		start = end - 24*3600*1000
	}
	return start, end
}

func parseInt64(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

// apiKeyFilter 根据 api_key 查询参数返回追加的 WHERE 片段与参数；无则该列为空。
// api_key 列在 usage_events 唯一存在（model_pricing 等 JOIN 表均无），故无需别名前缀。
func apiKeyFilter(q func(string) string) (string, []any) {
	if k := q("api_key"); k != "" {
		return " AND api_key = ?", []any{k}
	}
	return "", nil
}

func apiStats(q func(string) string) map[string]any {
	start, end := rangeBounds(q)
	db := mustDB()
	out := map[string]any{"start_ts": start, "end_ts": end}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	frag, fargs := apiKeyFilter(q)
	var req, in, outT, total, cached, lat, ttft, failed int64
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(total_tokens),0), COALESCE(SUM(cached_tokens),0),
		COALESCE(SUM(latency_ms),0), COALESCE(SUM(ttft_ms),0), COALESCE(SUM(failed),0)
		FROM usage_events WHERE ts BETWEEN ? AND ?`+frag, append([]any{start, end}, fargs...)...).
		Scan(&req, &in, &outT, &total, &cached, &lat, &ttft, &failed)

	var cost float64
	db.QueryRow(`SELECT COALESCE(SUM(
		COALESCE(p.input_price,0)*e.input_tokens/1000000.0 +
		COALESCE(p.output_price,0)*e.output_tokens/1000000.0
	),0) FROM usage_events e LEFT JOIN model_pricing p ON p.model = e.model
		WHERE e.ts BETWEEN ? AND ?`+frag, append([]any{start, end}, fargs...)...).Scan(&cost)

	minutes := float64(end-start) / 60000.0
	if minutes < 1 {
		minutes = 1
	}
	successRate := 1.0
	if req > 0 {
		successRate = float64(req-failed) / float64(req)
	}
	cacheRate := 0.0
	if in+cached > 0 {
		cacheRate = float64(cached) / float64(in+cached)
	}
	avgLat := int64(0)
	avgTTFT := int64(0)
	if req > 0 {
		avgLat = lat / req
		avgTTFT = ttft / req
	}

	out["requests"] = req
	out["tokens"] = map[string]int64{"input": in, "output": outT, "total": total, "cached": cached}
	out["cost"] = round2(cost)
	out["success_rate"] = round4(successRate)
	out["cache_rate"] = round4(cacheRate)
	out["rpm"] = round2(float64(req) / minutes)
	out["tpm"] = round2(float64(total) / minutes)
	out["avg_latency_ms"] = avgLat
	out["avg_ttft_ms"] = avgTTFT
	return out
}

func apiTrend(q func(string) string) map[string]any {
	start, end := rangeBounds(q)
	bucket := q("bucket") // "hour" | "day"
	step := int64(3600 * 1000)
	if bucket == "day" {
		step = 24 * 3600 * 1000
	}
	db := mustDB()
	out := map[string]any{"start_ts": start, "end_ts": end, "bucket": bucket}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	frag, fargs := apiKeyFilter(q)
	rows, err := db.Query(`SELECT (ts / ?) * ? AS b, COUNT(*), COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(cached_tokens),0), COALESCE(SUM(failed),0), COALESCE(SUM(input_tokens),0),
		COALESCE(SUM(COALESCE(p.input_price,0)*e.input_tokens/1000000.0 + COALESCE(p.output_price,0)*e.output_tokens/1000000.0),0)
		FROM usage_events e LEFT JOIN model_pricing p ON p.model = e.model
		WHERE e.ts BETWEEN ? AND ?`+frag+` GROUP BY b ORDER BY b`, append([]any{step, step, start, end}, fargs...)...)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var b, cnt, tokens, cached, failed, input int64
		var cost float64
		if err := rows.Scan(&b, &cnt, &tokens, &cached, &failed, &input, &cost); err == nil {
			items = append(items, map[string]any{"ts": b, "requests": cnt, "tokens": tokens, "cached_tokens": cached, "failed": failed, "input_tokens": input, "cost": round2(cost)})
		}
	}
	out["series"] = items
	return out
}

func apiGroup(q func(string) string, col string) map[string]any {
	start, end := rangeBounds(q)
	db := mustDB()
	out := map[string]any{"group": col, "start_ts": start, "end_ts": end}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	// Column name comes from a fixed allowlist only; table-qualified to avoid
	// ambiguity with model_pricing.model in the JOIN.
	colName := "e.model"
	if col == "api_key" {
		colName = "e.api_key"
	}
	frag, fargs := apiKeyFilter(q)
	rows, err := db.Query(fmt.Sprintf(`SELECT %s, COUNT(*), COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(AVG(latency_ms),0), COALESCE(AVG(ttft_ms),0), COALESCE(SUM(failed),0),
		COALESCE(SUM(
			COALESCE(p.input_price,0)*e.input_tokens/1000000.0 +
			COALESCE(p.output_price,0)*e.output_tokens/1000000.0
		),0)
		FROM usage_events e LEFT JOIN model_pricing p ON p.model = e.model
		WHERE e.ts BETWEEN ? AND ?`+frag+` GROUP BY %s ORDER BY 3 DESC LIMIT 100`, colName, colName),
		append([]any{start, end}, fargs...)...)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var name string
		var cnt, tokens, in, outT int64
		var avgLat, avgTTFT, cost float64
		var failed int64
		if err := rows.Scan(&name, &cnt, &tokens, &in, &outT, &avgLat, &avgTTFT, &failed, &cost); err == nil {
			items = append(items, map[string]any{
				"name": name, "requests": cnt, "tokens": tokens, "input_tokens": in, "output_tokens": outT, "cost": round2(cost),
				"avg_latency_ms": int64(avgLat), "avg_ttft_ms": int64(avgTTFT), "failed": failed,
			})
		}
	}
	out["items"] = items
	return out
}

type metricDef struct {
	key, name string
}

func apiRealtime(q func(string) string) map[string]any {
	window := parseInt64(q("window"))
	if window <= 0 {
		window = 15
	}
	winMin := window
	now := nowMillis()
	nowStart := now - winMin*60000
	nowd := time.Now()
	dayStart := time.Date(nowd.Year(), nowd.Month(), nowd.Day(), 0, 0, 0, 0, nowd.Location()).UnixMilli()

	db := mustDB()
	out := map[string]any{"window": window, "ts": now}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}

	defs := []metricDef{
		{"token_velocity", "Token 速率"},
		{"ttft", "TTFT"},
		{"latency", "延迟"},
		{"request_level", "请求速率"},
		{"cache_level", "缓存水平"},
	}

	frag, fargs := apiKeyFilter(q)
	items := make([]map[string]any, 0)
	for _, d := range defs {
		nv := metricValue(db, d, nowStart, now, winMin, frag, fargs)
		av := metricValue(db, d, dayStart, now, winMin, frag, fargs)
		trend := 0.0
		if av > 0 {
			trend = (nv - av) / av * 100
		}
		items = append(items, map[string]any{
			"key": d.key, "name": d.name,
			"now": round2(nv), "avg": round2(av), "trend": round2(trend),
		})
	}
	out["metrics"] = items
	return out
}

// metricValue 计算某指标在 [start,end] 区间的值；速率型指标分母为区间分钟数。
func metricValue(db *sql.DB, d metricDef, start, end int64, winMin int64, frag string, fargs []any) float64 {
	denom := float64(end-start) / 60000.0
	if denom < 1 {
		denom = 1
	}
	args := append([]any{start, end}, fargs...)
	switch d.key {
	case "token_velocity", "request_level":
		col := "total_tokens"
		if d.key == "request_level" {
			col = "*"
		}
		var v float64
		if col == "*" {
			var c int64
			db.QueryRow(`SELECT COUNT(*) FROM usage_events WHERE ts BETWEEN ? AND ?`+frag, args...).Scan(&c)
			v = float64(c)
		} else {
			db.QueryRow(`SELECT COALESCE(SUM(`+col+`),0) FROM usage_events WHERE ts BETWEEN ? AND ?`+frag, args...).Scan(&v)
		}
		return v / denom
	case "ttft", "latency":
		col := "ttft_ms"
		if d.key == "latency" {
			col = "latency_ms"
		}
		var v float64
		db.QueryRow(`SELECT COALESCE(AVG(`+col+`),0) FROM usage_events WHERE ts BETWEEN ? AND ?`+frag, args...).Scan(&v)
		return v
	case "cache_level":
		var cached, input int64
		db.QueryRow(`SELECT COALESCE(SUM(cached_tokens),0), COALESCE(SUM(input_tokens),0) FROM usage_events WHERE ts BETWEEN ? AND ?`+frag, args...).Scan(&cached, &input)
		if cached+input == 0 {
			return 0
		}
		return float64(cached) / float64(cached+input) * 100
	}
	return 0
}

func apiHeatmap(q func(string) string) map[string]any {
	start, end := rangeBounds(q)
	db := mustDB()
	out := map[string]any{"start_ts": start, "end_ts": end}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	frag, fargs := apiKeyFilter(q)
	// Hour-of-day buckets across the range (local time), 0..23.
	var buckets [24]int64
	rows, err := db.Query(`SELECT ts FROM usage_events WHERE ts BETWEEN ? AND ?`+frag, append([]any{start, end}, fargs...)...)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			continue
		}
		h := time.UnixMilli(ts).Hour()
		buckets[h]++
	}
	out["hours"] = buckets
	return out
}

func apiEvents(q func(string) string) map[string]any {
	start, end := rangeBounds(q)
	limit := parseInt64(q("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := parseInt64(q("offset"))
	db := mustDB()
	out := map[string]any{"start_ts": start, "end_ts": end, "limit": limit, "offset": offset}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	where := "ts BETWEEN ? AND ?"
	args := []any{start, end}
	if v := q("model"); v != "" {
		where += " AND model = ?"
		args = append(args, v)
	}
	if v := q("api_key"); v != "" {
		where += " AND api_key = ?"
		args = append(args, v)
	}
	if v := q("failed"); v == "1" || v == "true" {
		where += " AND failed = 1"
	} else if v == "0" || v == "false" {
		where += " AND failed = 0"
	}
	if v := q("source"); v != "" {
		where += " AND source = ?"
		args = append(args, v)
	}
	var total int64
	db.QueryRow("SELECT COUNT(*) FROM usage_events WHERE "+where, args...).Scan(&total)

	rows, err := db.Query(`SELECT ts, provider, model, alias, api_key, auth_id, source, latency_ms, ttft_ms,
		failed, status_code, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens
		FROM usage_events WHERE `+where+` ORDER BY ts DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var ts, lat, ttft, failed, status, in, outT, reasoning, cached, total int64
		var provider, model, alias, apiKey, authID, source string
		if err := rows.Scan(&ts, &provider, &model, &alias, &apiKey, &authID, &source, &lat, &ttft, &failed, &status, &in, &outT, &reasoning, &cached, &total); err == nil {
			items = append(items, map[string]any{
				"ts": ts, "provider": provider, "model": model, "alias": alias, "api_key": apiKey, "auth_id": authID, "source": source,
				"latency_ms": lat, "ttft_ms": ttft, "failed": failed, "status_code": status,
				"input_tokens": in, "output_tokens": outT, "reasoning_tokens": reasoning, "cached_tokens": cached, "total_tokens": total,
			})
		}
	}
	out["total"] = total
	out["items"] = items
	return out
}

func apiEventSources(q func(string) string) map[string]any {
	start, end := rangeBounds(q)
	db := mustDB()
	out := map[string]any{"start_ts": start, "end_ts": end}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	rows, err := db.Query(`SELECT DISTINCT source FROM usage_events
		WHERE ts BETWEEN ? AND ? AND source != '' ORDER BY source`, start, end)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	sources := make([]string, 0)
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			sources = append(sources, s)
		}
	}
	out["sources"] = sources
	return out
}

func apiHealth() map[string]any {
	db := mustDB()
	out := map[string]any{
		"version": version,
		"dropped": dropped.Load(),
		"status":  "ok",
		"ts":      nowMillis(),
	}
	if db == nil {
		out["status"] = "db unavailable"
		return out
	}
	var total, lastTs int64
	db.QueryRow(`SELECT COUNT(*) FROM usage_events`).Scan(&total)
	db.QueryRow(`SELECT COALESCE(MAX(ts),0) FROM usage_events`).Scan(&lastTs)
	out["events"] = total
	out["last_event_ts"] = lastTs
	return out
}

func apiKeysOptions() map[string]any {
	db := mustDB()
	out := map[string]any{}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	rows, err := db.Query(`SELECT DISTINCT e.api_key, COALESCE(a.alias, '')
		FROM usage_events e LEFT JOIN api_key_aliases a ON a.api_key = e.api_key
		WHERE e.api_key IS NOT NULL AND e.api_key != '' ORDER BY e.api_key`)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	options := make([]map[string]any, 0)
	for rows.Next() {
		var key, alias string
		if err := rows.Scan(&key, &alias); err == nil {
			label := key
			if alias != "" {
				label = alias
			}
			options = append(options, map[string]any{"id": key, "label": label, "alias": alias})
		}
	}
	out["options"] = options
	return out
}

// apiKeyAliasPut 设置 API Key 别名，body = {api_key, alias}。
func apiKeyAliasPut(body string) map[string]any {
	db := mustDB()
	out := map[string]any{}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	var req struct {
		APIKey string `json:"api_key"`
		Alias  string `json:"alias"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		out["error"] = "bad body: " + err.Error()
		return out
	}
	if req.APIKey == "" {
		out["error"] = "api_key is required"
		return out
	}
	_, err := db.Exec(`INSERT INTO api_key_aliases(api_key, alias, updated_at)
		VALUES(?,?,?) ON CONFLICT(api_key) DO UPDATE SET alias=excluded.alias, updated_at=excluded.updated_at`,
		req.APIKey, req.Alias, nowMillis())
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	out["updated"] = 1
	return out
}

func apiPricingGet() map[string]any {
	db := mustDB()
	out := map[string]any{}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	rows, err := db.Query(`SELECT model, COALESCE(input_price,0), COALESCE(output_price,0), COALESCE(source,''), COALESCE(updated_at,0) FROM (
		SELECT e.model, p.input_price, p.output_price, p.source, p.updated_at
			FROM (SELECT DISTINCT model FROM usage_events WHERE model IS NOT NULL AND model != '') e
			LEFT JOIN model_pricing p ON p.model = e.model
		UNION
		SELECT p2.model, p2.input_price, p2.output_price, p2.source, p2.updated_at
			FROM model_pricing p2
			WHERE p2.model NOT IN (SELECT DISTINCT model FROM usage_events WHERE model IS NOT NULL AND model != '')
	) ORDER BY model`)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var m string
		var in, outT float64
		var src string
		var upd int64
		if err := rows.Scan(&m, &in, &outT, &src, &upd); err == nil {
			items = append(items, map[string]any{"model": m, "input_price": in, "output_price": outT, "source": src, "updated_at": upd})
		}
	}
	out["items"] = items
	return out
}

// apiPricingPut expects a JSON body of [{model, input_price, output_price, source?}].
// source 缺省为 manual;同步应用(source=models.dev)与手动编辑共用。
func apiPricingPut(body string) map[string]any {
	db := mustDB()
	out := map[string]any{}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	var rows []struct {
		Model       string  `json:"model"`
		InputPrice  float64 `json:"input_price"`
		OutputPrice float64 `json:"output_price"`
		Source      string  `json:"source"`
	}
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		out["error"] = "bad body: " + err.Error()
		return out
	}
	now := nowMillis()
	n := 0
	for _, r := range rows {
		src := r.Source
		if src == "" {
			src = "manual"
		}
		if _, err := db.Exec(`INSERT INTO model_pricing(model, input_price, output_price, source, updated_at)
			VALUES(?,?,?,?,?) ON CONFLICT(model) DO UPDATE SET input_price=excluded.input_price,
			output_price=excluded.output_price, source=excluded.source, updated_at=excluded.updated_at`,
			r.Model, r.InputPrice, r.OutputPrice, src, now); err == nil {
			n++
		}
	}
	out["updated"] = n
	return out
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
func round4(v float64) float64 { return float64(int(v*10000+0.5)) / 10000 }

var _ = sql.ErrNoRows
