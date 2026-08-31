// cpa-plugin-usage-lens: a CLIProxyAPI plugin that persists and visualizes
// usage (requests/tokens/cost/latency) without a standalone service.
//
// Capabilities: UsagePlugin (event-driven ingest) + ManagementAPI (panel + JSON
// API). Data lives in a local SQLite file; no external sync or keys required.
package main

/*
#include <stdlib.h>
#include <stdint.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef void* cliproxy_host_ctx;
typedef int (*cliproxy_host_call_fn)(cliproxy_host_ctx, const char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	cliproxy_host_ctx host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

static int uk_call_host(cliproxy_host_api* api, const char* method, uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	return api->call(api->host_ctx, method, request, request_len, response);
}
static void uk_free_host_buffer(cliproxy_host_api* api, void* ptr, size_t len) {
	api->free_buffer(ptr, len);
}

#ifdef __cplusplus
extern "C" {
#endif
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
#ifdef __cplusplus
}
#endif
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const providerName = "usage-lens"

// version 由 Makefile 经 -ldflags "-X main.version=$(VERSION)" 注入;缺省 0.1.0。
var version = "0.1.0"

var hostAPI *C.cliproxy_host_api

func main() {}

// ---------------------------------------------------------------------------
// C ABI exports
// ---------------------------------------------------------------------------

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	hostAPI = host
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	// no-op: touching Go runtime state right before dlclose risks SIGSEGV.
}

// ---------------------------------------------------------------------------
// Envelope + dispatch
// ---------------------------------------------------------------------------

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func okEnvelope(v any) ([]byte, error) {
	result, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: result})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		configure(request)
		return okEnvelope(ukRegistration())
	case pluginabi.MethodManagementRegister:
		return handleManagementRegister(request)
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	case pluginabi.MethodUsageHandle:
		return handleUsage(request)
	default:
		return nil, fmt.Errorf("unsupported method %s", method)
	}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	ManagementAPI bool `json:"management_api"`
	UsagePlugin   bool `json:"usage_plugin"`
}

func ukRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             providerName,
			Version:          version,
			Author:           "Hex",
			GitHubRepository: "https://github.com/hex-ci/cpa-plugin-usage-lens",
			Logo:             "https://raw.githubusercontent.com/hex-ci/cpa-plugin-usage-lens/main/assets/logo.png",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "db_path",
					Type:        pluginapi.ConfigFieldTypeString,
					Description: "SQLite 数据库文件路径（默认 ~/.cli-proxy-api/usage-lens.db）",
				},
			},
		},
		Capabilities: registrationCapability{
			ManagementAPI: true,
			UsagePlugin:   true,
		},
	}
}
