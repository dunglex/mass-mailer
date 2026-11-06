package mailer

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-co-op/gocron/v2"
)

func Run() error {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		return err
	}
	app := &App{scheduler: scheduler, jobs: make(map[string]gocron.Job)}
	app.tmpl = template.Must(template.New("base").Funcs(template.FuncMap{"receiverName": func(receiver Receiver) string {
		if receiver["name"] != "" {
			return receiver["name"]
		}
		return receiver["email"]
	}}).ParseFS(templateFiles, "templates/*.gohtml"))
	if err := app.load(); err != nil {
		return fmt.Errorf("load data: %w", err)
	}
	for _, batch := range app.store.Batches {
		if err := app.schedule(batch); err != nil {
			log.Printf("schedule %q: %v", batch.Name, err)
		}
	}
	scheduler.Start()

	warnIfUnauthenticated()
	http.HandleFunc("/", requireAuth(app.dashboard))
	http.HandleFunc("/settings", requireAuth(app.settings))
	http.HandleFunc("/batches/new", requireAuth(app.newBatch))
	http.HandleFunc("/batches", requireAuth(app.createBatch))
	http.HandleFunc("/batches/", requireAuth(app.batchAction))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Mass Mailer listening at http://localhost:%s", port)
	return http.ListenAndServe(":"+port, nil)
}
