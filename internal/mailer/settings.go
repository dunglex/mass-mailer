package mailer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

var dataFile string

func init() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	dataFile = filepath.Join(dataDir, "mass-mailer.json")
}

type Settings struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Encryption string `json:"encryption"`
}

func (app *App) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		app.mu.Lock()
		defer app.mu.Unlock()
		if err := app.tmpl.ExecuteTemplate(w, "settings", PageData{Store: app.store, Flash: r.URL.Query().Get("flash")}); err != nil {
			log.Printf("render settings: %v", err)
		}
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	app.store.Settings = Settings{Host: r.FormValue("host"), Username: r.FormValue("username"), Password: r.FormValue("password"), Encryption: r.FormValue("encryption")}
	fmt.Sscanf(r.FormValue("port"), "%d", &app.store.Settings.Port)
	app.redirect(w, r, app.save())
}

func (app *App) load() error {
	contents, err := os.ReadFile(dataFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(contents, &app.store); err != nil {
			return err
		}
	}
	app.seedSettingsFromEnv()
	return nil
}

// seedSettingsFromEnv fills in the SMTP configuration from SMTP_* environment
// variables when nothing has been saved yet, so a fresh container (the Mailpit
// stack in compose.yaml, for instance) can send without anyone first filling in
// the Server settings page. The environment only supplies defaults: once
// settings are saved from the web interface they live in the data file and win
// on every later boot. The seeded values are never written to disk, so clearing
// the data file returns to the environment's defaults.
func (app *App) seedSettingsFromEnv() {
	if app.store.Settings.Host != "" {
		return
	}
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return
	}
	seeded := Settings{
		Host:       host,
		Username:   os.Getenv("SMTP_USERNAME"),
		Password:   os.Getenv("SMTP_PASSWORD"),
		Encryption: os.Getenv("SMTP_ENCRYPTION"),
	}
	if port, err := strconv.Atoi(os.Getenv("SMTP_PORT")); err == nil {
		seeded.Port = port
	}
	if seeded.Encryption == "" {
		// STARTTLS is the safe default; a plaintext sink such as Mailpit has
		// to opt out explicitly with SMTP_ENCRYPTION=none.
		seeded.Encryption = "tls"
	}
	app.store.Settings = seeded
	log.Printf("SMTP settings seeded from environment: %s:%d (encryption %s)", seeded.Host, seeded.Port, seeded.Encryption)
}

func (app *App) save() error {
	if err := os.MkdirAll(filepath.Dir(dataFile), 0755); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(app.store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dataFile, contents, 0600)
}
