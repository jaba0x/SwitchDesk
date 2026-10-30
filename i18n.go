package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Translation is a language file from the data folder's "translations"
// folder, so anyone can translate SwitchDesk or fix a translation without
// building it (discussion #5). Its format is the one in frontend/dist/i18n.
type Translation struct {
	Language string            `json:"language"`
	Code     string            `json:"code"`
	Strings  map[string]string `json:"strings"`
}

var transCode = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z]{2,4})?$`)

// UserTranslations reads <data folder>/translations/*.json.
func (a *App) UserTranslations() []Translation {
	dir := filepath.Join(a.st.Dir(), "translations")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return []Translation{}
	}
	out := []Translation{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		if info, err := e.Info(); err != nil || info.Size() > 2<<20 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var t Translation
		if json.Unmarshal(b, &t) != nil {
			continue
		}
		if t.Code == "" {
			t.Code = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		}
		if !transCode.MatchString(t.Code) || t.Code == "en" || len(t.Strings) == 0 {
			continue
		}
		if t.Language == "" {
			t.Language = t.Code
		}
		if len(t.Language) > 40 {
			t.Language = t.Language[:40]
		}
		out = append(out, t)
	}
	return out
}

// OpenTranslationsFolder creates and shows the folder for translation files.
func (a *App) OpenTranslationsFolder() error {
	dir := filepath.Join(a.st.Dir(), "translations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return openPath(dir)
}
