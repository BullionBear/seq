package rotate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// cleanup removes expired backups. Never touches the active file named by
// activeName. Safe to run concurrently with writes: it only unlinks older files.
func cleanup(pol Policy, activeName string) {
	if pol.MaxBackups <= 0 && pol.MaxAgeDays <= 0 {
		return
	}
	entries, err := os.ReadDir(pol.Dir)
	if err != nil {
		return
	}

	prefix := pol.BaseName + "_"
	suffix := "." + pol.Ext
	type backup struct {
		name    string
		modTime time.Time
	}
	var backups []backup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == activeName {
			continue
		}
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backup{name: name, modTime: info.ModTime()})
	}

	// Newest first so we keep the most recent MaxBackups.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})

	cutoff := time.Time{}
	if pol.MaxAgeDays > 0 {
		cutoff = time.Now().UTC().AddDate(0, 0, -pol.MaxAgeDays)
	}

	for i, b := range backups {
		byCount := pol.MaxBackups > 0 && i >= pol.MaxBackups
		byAge := pol.MaxAgeDays > 0 && b.modTime.Before(cutoff)
		if !byCount && !byAge {
			continue
		}
		_ = os.Remove(filepath.Join(pol.Dir, b.name))
	}
}
