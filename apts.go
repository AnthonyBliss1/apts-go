package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/go-chi/chi/v5"
)

type Apartments struct {
	Name              string
	UnitNumber        string
	Beds              int
	Baths             float64
	SquareFeet        float64
	Rent              float64
	AvailableDateText string
}

func scrape_apartment_listing(raw_url string) ([]Apartments, error) {
	parsedURL, err := url.Parse(raw_url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	host := parsedURL.Host

	req, err := http.NewRequest("GET", raw_url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	if host == "www.apartments.com" {

		// defining headers (same as python version)
		req.Header.Add("authority", "www.apartments.com")
		req.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		//req.Header.Add("accept-encoding", "gzip, deflate, br, zstd")
		req.Header.Add("accept-language", "en-US,en;q=0.9")
		req.Header.Add("cache-control", "no-cache")
		req.Header.Add("dnt", "1")
		req.Header.Add("pragma", "no-cache")
		req.Header.Add("Sec-CH-UA", `"Not A(Brand";v="8", "Chromium";v="132"`)
		req.Header.Add("sec-ch-ua-mobile", "?0")
		req.Header.Add("sec-ch-ua-platform", "macOS")
		req.Header.Add("sec-fetch-dest", "document")
		req.Header.Add("sec-fetch-mode", "navigate")
		req.Header.Add("sec-fetch-site", "none")
		req.Header.Add("sec-fetch-user", "?1")
		req.Header.Add("upgrade-insecure-requests", "1")
		// fake user agent generated for Chrome
		req.Header.Add("user-agent", browser.Chrome())

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("sending HTTP request to apartments.com failed: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("received status %d from apartments.com", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}

		body_string := string(body)

		// defining regex pattern to find the rental section in the body of the response
		pattern := regexp.MustCompile(`(?s)rentals:\s*(\[[\s\S]*?\])\s*,\s*disableMediaCascading`)

		// if we find the rentals
		if match := pattern.FindStringSubmatch(body_string); len(match) > 1 {
			var a []Apartments

			Data := []byte(match[1])

			err := json.Unmarshal(Data, &a)
			if err != nil {
				return nil, fmt.Errorf("parsing json: %w", err)
			}

			// if the listing in one 'room' then we print it regardless (it likely is a home for rent with no Name or Unit)
			if len(a) == 1 {
				return a, nil
			}

			// for now, printing out our rentals that have an availability date
			var records []Apartments
			for _, apt := range a {
				if apt.AvailableDateText != "Available Soon" && apt.UnitNumber != "" {
					records = append(records, apt)
				}
			}
			return records, nil
		}

		//fmt.Println("No apartments found")

	} else if host == "www.zillow.com" {
		fmt.Println("\nDEBUG: Sending request for zillow")
	} else {
		return nil, fmt.Errorf("unsupported host")
	}
	return []Apartments{}, nil
}

func scrapeHandler(w http.ResponseWriter, r *http.Request) {
	raw_url := r.URL.Query().Get("url")
	if raw_url == "" {
		http.Error(w, "`url` query parameter is required", http.StatusBadRequest)
		return
	}

	records, err := scrape_apartment_listing(raw_url)
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

func main() {
	r := chi.NewRouter()

	r.Get("/apts", scrapeHandler)

	fmt.Print("Starting Apts API on 0.0.0.0:8000")

	fmt.Println(`
 ______     ______   ______   ______        ______     ______   __    
/\  __ \   /\  == \ /\__  _\ /\  ___\      /\  __ \   /\  == \ /\ \   
\ \  __ \  \ \  _-/ \/_/\ \/ \ \___  \     \ \  __ \  \ \  _-/ \ \ \  
 \ \_\ \_\  \ \_\      \ \_\  \/\_____\     \ \_\ \_\  \ \_\    \ \_\ 
  \/_/\/_/   \/_/       \/_/   \/_____/      \/_/\/_/   \/_/     \/_/  `)

	log.Fatal(http.ListenAndServe("0.0.0.0:8000", r))
}
