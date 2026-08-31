// config.go: plugin configuration decoded from plugins.configs.<id> YAML.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type pluginConfig struct {
	DBPath string // SQLite path
}

var (
	cfgMu     sync.RWMutex
	cfg       = pluginConfig{DBPath: defaultDBPath()}
	startOnce sync.Once
)

// defaultDBPath 把数据库放在 CPA 的数据家目录 ~/.cli-proxy-api（auth 文件、
// 插件目录均在此，与 CPA 数据同生命周期，不受系统 /tmp 清理影响），
// 退而求其次用进程工作目录，最后 /tmp。
func defaultDBPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cli-proxy-api", "usage-lens.db")
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "usage-lens.db")
	}
	return "/tmp/usage-lens.db"
}

// configure decodes config_yaml (a base64/YAML subtree sent by the host on
// plugin.register / plugin.reconfigure).
func configure(raw []byte) {
	var req struct {
		ConfigYAML []byte `json:"config_yaml"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}

	next := cfg // reconfigure preserves previous values
	for _, line := range strings.Split(string(req.ConfigYAML), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "db_path:") {
			if v := yamlField(line, "db_path:"); v != "" {
				next.DBPath = v
			}
		}
	}

	cfgMu.Lock()
	cfg = next
	cfgMu.Unlock()

	// The ingest worker consumes usage events off recCh; start it exactly once.
	startOnce.Do(func() {
		go runIngestWorker()
	})
}

func yamlField(line, key string) string {
	return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, key)), "\"'")
}
