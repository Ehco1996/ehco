package main

import (
	"github.com/Ehco1996/ehco"
	"log"

	"encoding/json"
	"net/http"
)

func renderConfig(w http.ResponseWriter, r *http.Request) {

	cfg := ehco.NewConfig("config.json")
	cfg.LoadConfig()

	res, err := json.Marshal(cfg.Configs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

func main() {
	http.HandleFunc("/config", renderConfig)
	log.Println("start http server on http://127.0.0.1:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
