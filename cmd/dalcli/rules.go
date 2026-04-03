package main

import (
	"fmt"
	"log"
	"time"
)

// enforceNoMainPush blocks direct push to main branch.
func enforceNoMainPush(branch string) error {
	if branch == "main" || branch == "master" {
		return fmt.Errorf("direct push to %s forbidden — use branch + PR", branch)
	}
	return nil
}

// enforceAutoIssueLimit prevents excessive auto-created issues.
var autoIssueCount int
var autoIssueResetTime time.Time

func enforceAutoIssueLimit() error {
	now := time.Now()
	if now.Sub(autoIssueResetTime) > 24*time.Hour {
		autoIssueCount = 0
		autoIssueResetTime = now
	}
	if autoIssueCount >= 3 {
		return fmt.Errorf("auto issue limit reached (3/24h)")
	}
	autoIssueCount++
	log.Printf("[rules] auto issue %d/3 today", autoIssueCount)
	return nil
}
