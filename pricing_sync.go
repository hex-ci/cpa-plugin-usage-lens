// pricing_sync.go: models.dev 价格同步(预览→勾选→应用;手动价优先,不自动覆盖)。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"time"
)

const modelsDevURL = "https://models.dev/api.json"

// modelsDevEntry 只取成本所需字段;api.json 模型条目还有大量元数据且个别是
// 字符串占位,故 cost 用宽松 map 解析,unmarshal 不会因异常条目整体失败。
type modelsDevEntry struct {
	Cost map[string]json.Number `json:"cost"`
}

// modelsDevCatalog: provider → {models: {model id: 原始 JSON}}。个别模型条目是
// 字符串占位,先收 RawMessage,匹配到单条时再宽松解码。
type modelsDevProvider struct {
	Models map[string]json.RawMessage `json:"models"`
}

type modelsDevCatalog map[string]modelsDevProvider

var dateSuffixRe = regexp.MustCompile(`-\d{3,4}$`) // 日期后缀,如 -0813 / -0731

// findModel 先精确匹配模型 id;未中则剥离日期后缀再试(deepseek-v4-pro-0813 → deepseek-v4-pro)。
// 同一模型 id 可能出现在多个 provider(官方 + 聚合商),遍历时优先取有有效价格的条目。
func (c modelsDevCatalog) findModel(model string) (modelsDevEntry, bool) {
	if e, ok := tryModel(c, model); ok {
		return e, true
	}
	base := dateSuffixRe.ReplaceAllString(model, "")
	if base != "" && base != model {
		if e, ok := tryModel(c, base); ok {
			return e, true
		}
	}
	return modelsDevEntry{}, false
}

func tryModel(doc modelsDevCatalog, id string) (modelsDevEntry, bool) {
	var fallback modelsDevEntry
	have := false
	for _, p := range doc {
		raw, ok := p.Models[id]
		if !ok {
			continue
		}
		var e modelsDevEntry
		if json.Unmarshal(raw, &e) != nil {
			continue // 字符串占位等非对象条目
		}
		if !have {
			fallback, have = e, true
		}
		if v, ok := e.Cost["input"]; ok {
			if f, err := v.Float64(); err == nil && f > 0 {
				return e, true
			}
		}
	}
	return fallback, have
}

func fetchModelsDev() (modelsDevCatalog, error) {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(modelsDevURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	var cat modelsDevCatalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, err
	}
	return cat, nil
}

// apiPricingSyncPreview 比对「已出现模型」与 models.dev 目录。
// matched: 目录里有价的模型(带 manual 标记,应用时前端跳过手动价);
// unmatched: 目录缺失的模型。同步失败仅返回 error,不影响本地定价。
func apiPricingSyncPreview() map[string]any {
	db := mustDB()
	out := map[string]any{}
	if db == nil {
		out["error"] = "db unavailable"
		return out
	}
	used := make(map[string]string) // model -> 现有 source
	rows, err := db.Query(`SELECT DISTINCT e.model, COALESCE(p.source,'') FROM usage_events e
		LEFT JOIN model_pricing p ON p.model = e.model
		WHERE e.model IS NOT NULL AND e.model != ''`)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	for rows.Next() {
		var m, src string
		if rows.Scan(&m, &src) == nil {
			used[m] = src
		}
	}
	rows.Close()
	if len(used) == 0 {
		out["matched"] = []any{}
		out["unmatched"] = []any{}
		return out
	}

	cat, err := fetchModelsDev()
	if err != nil {
		out["error"] = "models.dev 同步失败: " + err.Error()
		return out
	}
	matched := make([]map[string]any, 0, len(used))
	unmatched := make([]string, 0)
	for m, src := range used {
		e, ok := cat.findModel(m)
		if !ok {
			unmatched = append(unmatched, m)
			continue
		}
		// 缺失/非法价格一律按 0 处理,不阻塞其余模型
		var in, outT float64
		if v, ok := e.Cost["input"]; ok {
			if f, err := v.Float64(); err == nil {
				in = f
			}
		}
		if v, ok := e.Cost["output"]; ok {
			if f, err := v.Float64(); err == nil {
				outT = f
			}
		}
		matched = append(matched, map[string]any{
			"model":        m,
			"input_price":  in,
			"output_price": outT,
			"manual":       src == "manual",
		})
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i]["model"].(string) < matched[j]["model"].(string) })
	sort.Strings(unmatched)
	out["matched"] = matched
	out["unmatched"] = unmatched
	return out
}
