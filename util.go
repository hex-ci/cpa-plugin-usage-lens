// util.go: small shared helpers.
package main

import "time"

func nowMillis() int64 {
	return time.Now().UnixMilli()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
