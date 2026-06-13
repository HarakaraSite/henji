package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const loadHTTPTimeout = 15 * time.Second

var loadHTTPClient = &http.Client{Timeout: loadHTTPTimeout}

func loadMsg(msg string) (string, error) {
	if strings.HasPrefix(msg, "https://") || strings.HasPrefix(msg, "http://") {
		resp, err := loadHTTPClient.Get(msg) //nolint:gosec,noctx
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			status := resp.Status
			if status == "" {
				status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			}
			return "", fmt.Errorf("load %s: %s", msg, status)
		}
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		return string(bts), nil
	}

	if strings.HasPrefix(msg, "file://") {
		bts, err := os.ReadFile(strings.TrimPrefix(msg, "file://"))
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		return string(bts), nil
	}

	return msg, nil
}
