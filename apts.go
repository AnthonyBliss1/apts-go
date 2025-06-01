package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
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

// TODO: need to add choice to use proxy or not. Proxy adds significant latency, might be good to give a choice to the user
func scrape_apartment_listing(raw_url string) ([]Apartments, error) {
	// store env variables to build proxy url
	oxy_name := os.Getenv("OXYLABS_USERNAME")
	if oxy_name == "" {
		return nil, fmt.Errorf("oxy username not set")
	}

	oxy_pass := os.Getenv("OXYLABS_PASSWORD")
	if oxy_pass == "" {
		return nil, fmt.Errorf("oxy pass not set")
	}

	oxy_proxy_host := os.Getenv("OXYLABS_PROXY_HOST")
	if oxy_proxy_host == "" {
		return nil, fmt.Errorf("oxy host not set")
	}

	oxy_proxy_port := os.Getenv("OXYLABS_PROXY_PORT")
	if oxy_proxy_port == "" {
		return nil, fmt.Errorf("oxy port not set")
	}

	parsedURL, err := url.Parse(raw_url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// build the proxy_url using the env variables
	proxy_string := fmt.Sprintf("https://%s:%s@%s:%s", oxy_name, oxy_pass, oxy_proxy_host, oxy_proxy_port)
	proxy_url, err := url.Parse(proxy_string)
	if err != nil {
		return nil, fmt.Errorf("parsing proxy url: %q", err)
	}

	// create transport using the proxy_url
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxy_url),
	}

	// wrap the proxy transport in the client
	client := &http.Client{Transport: transport}

	host := parsedURL.Host

	// establishing the GET request to pull rental data from url
	req, err := http.NewRequest("GET", raw_url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	if host == "www.apartments.com" {

		// defining headers (same as python version)
		req.Header.Add("authority", "www.apartments.com")
		req.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
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

		// defining regex pattern to find the rental section in the body of the response (same pattern from python project proved reliable)
		pattern := regexp.MustCompile(`rentals:\s*(\[.*?\])\s*,\s*disableMediaCascading`)

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

	} else if host == "www.zillow.com" {
		fmt.Println("\nDEBUG: Sending request for zillow")
	} else {
		return nil, fmt.Errorf("unsupported host")
	}
	return []Apartments{}, nil
}

func scrape_handler(w http.ResponseWriter, r *http.Request) {
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
	godotenv.Load()

	r := chi.NewRouter()

	r.Get("/apts", scrape_handler)

	fmt.Print("Starting Apts API on 0.0.0.0:8000")

	fmt.Println(`
 ______     ______   ______   ______        ______     ______   __    
/\  __ \   /\  == \ /\__  _\ /\  ___\      /\  __ \   /\  == \ /\ \   
\ \  __ \  \ \  _-/ \/_/\ \/ \ \___  \     \ \  __ \  \ \  _-/ \ \ \  
 \ \_\ \_\  \ \_\      \ \_\  \/\_____\     \ \_\ \_\  \ \_\    \ \_\ 
  \/_/\/_/   \/_/       \/_/   \/_____/      \/_/\/_/   \/_/     \/_/  `)

	log.Fatal(http.ListenAndServe("0.0.0.0:8000", r))
}
