package core

import (
	"strings"
	"time"
)

type BrowserOptions struct {
	Headless bool
	Timeout  time.Duration
	PoolSize int
	Mode     string
	CDPURL   string
}

func DefaultBrowserOptions(settings Settings) BrowserOptions {
	timeout := time.Duration(settings.BrowserStreamTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 1800 * time.Second
	}
	poolSize := settings.BrowserPoolSize
	if poolSize <= 0 {
		poolSize = 1
	}
	mode := strings.TrimSpace(strings.ToLower(settings.BrowserMode))
	if mode == "" {
		mode = "embedded"
	}
	return BrowserOptions{
		Headless: true,
		Timeout:  timeout,
		PoolSize: poolSize,
		Mode:     mode,
		CDPURL:   strings.TrimSpace(settings.BrowserCDPURL),
	}
}
