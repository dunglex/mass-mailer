package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const dataFile = "data/mass-mailer.json"

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
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, &app.store)
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
