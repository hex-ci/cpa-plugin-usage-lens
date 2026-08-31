// api.go: management-route dispatch + registration + HTTP helpers.
package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func handleManagementRegister(request []byte) ([]byte, error) {
	var req pluginapi.ManagementRegistrationRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return nil, err
	}
	_ = req.BasePath
	base := "/plugins/" + providerName
	api := base + "/api"
	return okEnvelope(pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{Method: "GET", Path: api + "/stats", Description: "Aggregate usage stats"},
			{Method: "GET", Path: api + "/trend", Description: "Time-bucketed usage series"},
			{Method: "GET", Path: api + "/models", Description: "Per-model usage"},
			{Method: "GET", Path: api + "/keys", Description: "Per-api-key usage"},
			{Method: "GET", Path: api + "/heatmap", Description: "Hourly activity heatmap"},
			{Method: "GET", Path: api + "/events", Description: "Request event log"},
			{Method: "GET", Path: api + "/events/sources", Description: "Distinct request sources"},
			{Method: "GET", Path: api + "/realtime", Description: "Realtime metrics (token velocity, ttft, latency, cache)"},
			{Method: "GET", Path: api + "/health", Description: "Service health"},
			{Method: "GET", Path: api + "/api-keys/options", Description: "API key filter options"},
			{Method: "PUT", Path: api + "/api-keys/alias", Description: "Update API key alias"},
			{Method: "GET", Path: api + "/pricing", Description: "Model pricing table"},
			{Method: "PUT", Path: api + "/pricing", Description: "Update model pricing"},
			{Method: "GET", Path: api + "/pricing/sync/preview", Description: "models.dev price sync preview"},
		},
		Resources: panelAssets(),
	})
}

func handleManagement(request []byte) ([]byte, error) {
	var req struct {
		Method  string
		Path    string
		Query   map[string][]string
		Headers map[string][]string
		Body    []byte
	}
	if err := json.Unmarshal(request, &req); err != nil {
		return errorEnvelope("bad_request", "malformed management request"), nil
	}
	method := strings.ToUpper(req.Method)
	path := strings.TrimRight(req.Path, "/")

	if strings.HasPrefix(path, "/v0/resource/plugins/"+providerName) {
		sub := strings.TrimPrefix(path, "/v0/resource/plugins/"+providerName)
		return okEnvelope(serveResource(sub))
	}

	if strings.HasPrefix(path, "/v0/management/plugins/"+providerName) {
		sub := strings.TrimPrefix(path, "/v0/management/plugins/"+providerName)
		if strings.HasPrefix(sub, "/api/") {
			return dispatchAPI(method, strings.TrimPrefix(sub, "/api"), req.Query, string(req.Body))
		}
		return okEnvelope(mgmtJSON(http.StatusNotFound, map[string]any{"error": "unknown route " + sub}))
	}
	return okEnvelope(mgmtJSON(http.StatusNotFound, map[string]any{"error": "unknown path"}))
}

func dispatchAPI(method, sub string, query map[string][]string, body string) ([]byte, error) {
	q := func(key string) string {
		if vs := query[key]; len(vs) > 0 {
			return strings.TrimSpace(vs[0])
		}
		return ""
	}
	var result any
	switch {
	case method == http.MethodGet && sub == "/stats":
		result = apiStats(q)
	case method == http.MethodGet && sub == "/trend":
		result = apiTrend(q)
	case method == http.MethodGet && sub == "/models":
		result = apiGroup(q, "model")
	case method == http.MethodGet && sub == "/keys":
		result = apiGroup(q, "api_key")
	case method == http.MethodGet && sub == "/heatmap":
		result = apiHeatmap(q)
	case method == http.MethodGet && sub == "/events":
		result = apiEvents(q)
	case method == http.MethodGet && sub == "/events/sources":
		result = apiEventSources(q)
	case method == http.MethodGet && sub == "/realtime":
		result = apiRealtime(q)
	case method == http.MethodGet && sub == "/health":
		result = apiHealth()
	case method == http.MethodGet && sub == "/api-keys/options":
		result = apiKeysOptions()
	case method == http.MethodPut && sub == "/api-keys/alias":
		return okEnvelope(mgmtJSON(http.StatusOK, apiKeyAliasPut(body)))
	case method == http.MethodGet && sub == "/pricing":
		result = apiPricingGet()
	case method == http.MethodPut && sub == "/pricing":
		return okEnvelope(mgmtJSON(http.StatusOK, apiPricingPut(body)))
	case method == http.MethodGet && sub == "/pricing/sync/preview":
		result = apiPricingSyncPreview()
	default:
		return okEnvelope(mgmtJSON(http.StatusNotFound, map[string]any{"error": "unknown api " + sub}))
	}
	return okEnvelope(mgmtJSON(http.StatusOK, result))
}

func mgmtJSON(status int, v any) pluginapi.ManagementResponse {
	body, _ := json.Marshal(v)
	return pluginapi.ManagementResponse{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: body}
}
