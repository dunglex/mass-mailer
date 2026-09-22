package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/go-co-op/gocron/v2"
)

func main() {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		log.Fatal(err)
	}
	app := &App{scheduler: scheduler, jobs: make(map[string]gocron.Job)}
	app.tmpl = template.Must(template.New("base").Funcs(template.FuncMap{"receiverName": func(receiver Receiver) string {
		if receiver["name"] != "" {
			return receiver["name"]
		}
		return receiver["email"]
	}}).ParseFS(templateFiles, "templates/*.gohtml"))
	if err := app.load(); err != nil {
		log.Fatalf("load data: %v", err)
	}
	for _, batch := range app.store.Batches {
		if err := app.schedule(batch); err != nil {
			log.Printf("schedule %q: %v", batch.Name, err)
		}
	}
	scheduler.Start()

	http.HandleFunc("/", app.dashboard)
	http.HandleFunc("/settings", app.settings)
	http.HandleFunc("/batches/new", app.newBatch)
	http.HandleFunc("/batches", app.createBatch)
	http.HandleFunc("/batches/", app.batchAction)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Mass Mailer listening at http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
