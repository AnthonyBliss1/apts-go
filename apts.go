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

func scrape_apartment_listing(url string, host string) ([]Apartments, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
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
			log.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatal(err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
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
				log.Fatal(err)
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
		fmt.Println("DEBUG: Sending request for zillow")
	} else {
		fmt.Println("Received an invalid URL")
	}
	return []Apartments{}, nil
}

func main() {
	for {
		var raw_url string

		fmt.Println("\nEnter the URL to scrape ('quit' to escape): ")
		fmt.Scan(&raw_url)

		parsedURL, err := url.Parse(raw_url)
		if err != nil {
			fmt.Println("Error parsing raw_url:", err)
		}

		var host string = parsedURL.Host
		//fmt.Println("Host:", host)

		if !strings.EqualFold(raw_url, "quit") {
			records, err := scrape_apartment_listing(raw_url, host)

			if err != nil {
				fmt.Println("Error Scraping Apartments:", err)
			}

			out, err := json.MarshalIndent(records, "", "  ")
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(string(out))

		} else {
			fmt.Println("Exiting application...")
			return
		}

	}
}
