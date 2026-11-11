package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/switchdesk/internal/aruba"
)

// Front panel photos come from the NetBox device type library (CC0), only when
// "Show front panel photos" is on in Settings (issue #20). Each photo is
// downloaded once and kept in <data folder>/hardware.
const photoBase = "https://raw.githubusercontent.com/netbox-community/devicetype-library/master/elevation-images/HPE/"

var (
	photoName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*\.front\.(png|jpg)$`)
	photoMu   sync.Mutex
)

// HardwarePhoto returns the front panel photo of a switch model as a data:
// URL, or "" when the setting is off or the library has no photo of it.
func (a *App) HardwarePhoto(model string) (string, error) {
	if !a.st.Settings().HardwarePhotos {
		return "", nil
	}
	dir := filepath.Join(a.st.Dir(), "hardware")
	// A photo of your own wins: <data folder>/hardware/JL256A.png (or .jpg),
	// for models the library has no picture of (issue #27).
	for _, pn := range aruba.PartNumbers(model) {
		for _, ext := range []string{".png", ".jpg", ".jpeg"} {
			if b, err := os.ReadFile(filepath.Join(dir, pn+ext)); err == nil && len(b) <= 8<<20 {
				if ct := http.DetectContentType(b); ct == "image/png" || ct == "image/jpeg" {
					return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(b), nil
				}
			}
		}
	}
	hw, ok := aruba.LookupHW(model)
	if !ok || hw.Photo == "" || !photoName.MatchString(hw.Photo) {
		return "", nil
	}
	photoMu.Lock()
	defer photoMu.Unlock()
	file := filepath.Join(dir, hw.Photo)
	b, err := os.ReadFile(file)
	if err != nil {
		if b, err = downloadPhoto(hw.Photo); err != nil {
			return "", err
		}
		if os.MkdirAll(dir, 0o700) == nil {
			_ = os.WriteFile(file, b, 0o600)
		}
	}
	mime := "image/png"
	if strings.HasSuffix(hw.Photo, ".jpg") {
		mime = "image/jpeg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

func downloadPhoto(name string) ([]byte, error) {
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Get(photoBase + name)
	if err != nil {
		return nil, fmt.Errorf("couldn't download the photo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("couldn't download the photo: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return nil, fmt.Errorf("couldn't download the photo: %w", err)
	}
	if len(b) > 8<<20 {
		return nil, fmt.Errorf("the photo is too large")
	}
	if ct := http.DetectContentType(b); ct != "image/png" && ct != "image/jpeg" {
		return nil, fmt.Errorf("the download isn't an image")
	}
	return b, nil
}
