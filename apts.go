package main

import (
	"encoding/json"
	"fmt"
	"io"
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

func scrape_apartment_listing(url string, host string) string {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Sprintln("Error creating request:", err)
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
			return fmt.Sprintln("Error sending request:", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintln("Request failed with status code:", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Sprintln("No body of response found:", err)
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
				return fmt.Sprintln("Error parsing data:", err)
			}

			// if the listing in one 'room' then we print it regardless (it likely is a home for rent with no Name or Unit)
			if len(a) == 1 {
				apt := a[0]
				return fmt.Sprintf("Name: %s Unit: %s Beds: %d Baths: %.1f Sqft: %.0f Rent: $%.0f Availability Date: %s", apt.Name, apt.UnitNumber, apt.Beds, apt.Baths, apt.SquareFeet, apt.Rent, apt.AvailableDateText)
			}

			// for now, printing out our rentals that have an availability date
			var records []string
			for i := range a {
				if a[i].AvailableDateText != "Available Soon" && a[i].UnitNumber != "" {
					records = append(records, fmt.Sprintf("Name: %s  Unit: %s Beds: %d Baths: %.1f Sqft: %.0f Rent: $%.0f Availability Date: %s", a[i].Name, a[i].UnitNumber, a[i].Beds, a[i].Baths, a[i].SquareFeet, a[i].Rent, a[i].AvailableDateText))
				}
			}

			if len(records) == 0 {
				return fmt.Sprintln("No apartments are currently available")
			} else {
				return strings.Join(records, "\n")
			}
		}
		return fmt.Sprintln("No match found")

	} else if host == "www.zillow.com" {
		return fmt.Sprintln("DEBUG: Sending request for zillow")
	} else {
		return fmt.Sprintln("Received an invalid URL")
	}
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
			fmt.Println(scrape_apartment_listing(raw_url, host))
		} else {
			fmt.Println("Exiting application...")
			break
		}

	}
}
