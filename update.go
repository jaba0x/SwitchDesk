package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jaba0x/switchdesk/internal/appver"
)

const releasesAPI = "https://api.github.com/repos/jaba0x/SwitchDesk/releases/latest"

// UpdateInfo describes the newest release on GitHub.
type UpdateInfo struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checkedAt"`
}

var (
	updMu   sync.Mutex
	updLast *UpdateInfo
)

// CheckForUpdate asks GitHub for the latest release.
func (a *App) CheckForUpdate() (UpdateInfo, error) {
	info := UpdateInfo{Current: Version, CheckedAt: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", releasesAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SwitchDesk/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return info, errors.New("couldn't reach GitHub")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return info, errors.New("GitHub answered " + resp.Status)
	}
	var rel struct {
		Tag       string    `json:"tag_name"`
		URL       string    `json:"html_url"`
		Published time.Time `json:"published_at"`
		Draft     bool      `json:"draft"`
		Pre       bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return info, err
	}
	info.Latest = strings.TrimPrefix(rel.Tag, "v")
	// only ever link to this repository's release pages
	info.URL = "https://github.com/jaba0x/SwitchDesk/releases/latest"
	if strings.HasPrefix(rel.URL, "https://github.com/jaba0x/SwitchDesk/releases/") {
		info.URL = rel.URL
	}
	info.Published = rel.Published
	info.Available = !rel.Draft && !rel.Pre && appver.Newer(info.Latest, Version)
	updMu.Lock()
	updLast = &info
	updMu.Unlock()
	return info, nil
}

// LastUpdateCheck returns the result of the last background check, if any.
func (a *App) LastUpdateCheck() *UpdateInfo {
	updMu.Lock()
	defer updMu.Unlock()
	return updLast
}

func (a *App) updateLoop() {
	time.Sleep(5 * time.Second)
	for {
		// only when the user hasn't turned it off in Settings
		if !a.st.Settings().NoUpdateCheck {
			if info, err := a.CheckForUpdate(); err == nil && info.Available {
				runtime.EventsEmit(a.ctx, "update-available", info)
			}
		}
		time.Sleep(6 * time.Hour)
	}
}
