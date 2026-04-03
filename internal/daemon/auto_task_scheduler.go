package daemon

import (
	"context"
	"log"
	"time"
)

type autoTaskEntry struct {
	DalName  string
	Role     string
	Task     string
	Interval time.Duration
}

func (d *Daemon) startAutoTaskScheduler(ctx context.Context) {
	tasks := []autoTaskEntry{
		{
			DalName:  "scribe",
			Role:     "member",
			Task:     "cd /workspace && ls .dal/decisions/inbox/ 2>/dev/null | head -1 && echo check-inbox || echo no-inbox",
			Interval: 2 * time.Hour,
		},
		{
			DalName:  "reviewer",
			Role:     "member",
			Task:     "gh pr list --repo dalsoop/dalcenter --state open --limit 3 --json number,title",
			Interval: 2 * time.Hour,
		},
	}

	log.Printf("[auto-scheduler] starting with %d tasks", len(tasks))
	for _, entry := range tasks {
		go d.runAutoTask(ctx, entry)
	}
}

func (d *Daemon) runAutoTask(ctx context.Context, entry autoTaskEntry) {
	time.Sleep(5 * time.Minute)
	ticker := time.NewTicker(entry.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("[auto-scheduler] running %s", entry.DalName)
			tr := d.tasks.New(entry.DalName, entry.Task)
			d.execTaskOneShot(entry.DalName, entry.Role, entry.Task, tr)
			log.Printf("[auto-scheduler] %s: %s", entry.DalName, tr.Status)
		}
	}
}
