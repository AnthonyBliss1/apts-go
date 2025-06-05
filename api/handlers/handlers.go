package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	utils "github.com/anthonybliss1/go-apts/api/utils"
)

func Scrape_handler(client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw_url := r.URL.Query().Get("url")
		if raw_url == "" {
			http.Error(w, "`url` query parameter is required", http.StatusBadRequest)
			return
		}

		records, _, err := utils.Scrape_apartment_listing(raw_url, client)
		if err != nil {
			if strings.HasPrefix(err.Error(), "unsupported host") || strings.HasPrefix(err.Error(), "invalid URL") {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(records); err != nil {
			log.Printf("failed to write JSON: %v\n", err)
		}
	}
}

func Chat_handler(client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw_url := r.URL.Query().Get("url")
		if raw_url == "" {
			http.Error(w, "`url` query parameter is required", http.StatusBadRequest)
			return
		}

		if err := utils.Send_notification(raw_url, client); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}
}
