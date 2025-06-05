package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

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

// defining regex pattern to find the rental section in the body of the response (same pattern from python project proved reliable)
var Pattern = regexp.MustCompile(`rentals:\s*(\[.*?\])\s*,\s*disableMediaCascading`)
var Listing_pattern = regexp.MustCompile(`listingName:\s*'([^']+)'`)

func Create_proxies() (*http.Client, error) {
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

	// build the proxy_url using the env variables
	proxy_string := fmt.Sprintf("http://%s:%s@%s:%s", oxy_name, oxy_pass, oxy_proxy_host, oxy_proxy_port)
	proxy_url, err := url.Parse(proxy_string)
	if err != nil {
		return nil, fmt.Errorf("parsing proxy url: %q", err)
	}

	dialer := &net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// create transport using the proxy_url
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxy_url),
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: 3 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
		//DisableKeepAlives:   true,
	}

	// wrap the proxy transport in the client
	client := &http.Client{Transport: transport}

	return client, nil
}

func Create_telegram_vars() (string, string, error) {
	// setup telegram variables and chat notis
	telegram_bot_token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if telegram_bot_token == "" {
		return "", "", fmt.Errorf("telegram bot token not set")
	}

	telegram_chat_id := os.Getenv("TELEGRAM_CHAT_ID")
	if telegram_chat_id == "" {
		return "", "", fmt.Errorf("telegram chat id not set")
	}

	// build api url
	telegram_api_url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegram_bot_token)

	return telegram_api_url, telegram_chat_id, nil
}

// TODO: need to add choice to use proxy or not. Fixed proxy latency but maybe still add the option if user doesn't have oxylabs account
func Scrape_apartment_listing(raw_url string, client *http.Client) ([]Apartments, string, error) {
	parsedURL, err := url.Parse(raw_url)
	if err != nil {
		return nil, "", fmt.Errorf("invalid URL: %w", err)
	}

	host := parsedURL.Host

	// establishing the GET request to pull rental data from url
	req, err := http.NewRequest("GET", raw_url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building request: %w", err)
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

		// drop dead sockets (if idle)
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("sending HTTP request to apartments.com failed: %w", err)
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("received status %d from apartments.com", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("reading response body: %w", err)
		}

		body_string := string(body)

		// if we find the rentals
		if match := Pattern.FindStringSubmatch(body_string); len(match) > 1 {
			var a []Apartments
			var listing_name string

			if listing_match := Listing_pattern.FindStringSubmatch(body_string); len(listing_match) > 1 {
				listing_name = listing_match[1]
			}

			Data := []byte(match[1])

			err := json.Unmarshal(Data, &a)
			if err != nil {
				return nil, "", fmt.Errorf("parsing json: %w", err)
			}

			// if the listing in one 'room' then we print it regardless (it likely is a home for rent with no Name or Unit)
			if len(a) == 1 {
				return a, "", nil
			}

			// for now, printing out our rentals that have an availability date
			var records []Apartments
			for _, apt := range a {
				if apt.AvailableDateText != "Available Soon" && apt.UnitNumber != "" {
					records = append(records, apt)
				}
			}
			return records, listing_name, nil
		}

	} else if host == "www.zillow.com" {
		fmt.Println("\nDEBUG: Sending request for zillow")
	} else {
		return nil, "", fmt.Errorf("unsupported host")
	}
	return []Apartments{}, "", nil
}

func Send_notification(raw_url string, client *http.Client) error {

	api_url, chat_id, err := Create_telegram_vars()
	if err != nil {
		return err
	}

	records, listing_name, err := Scrape_apartment_listing(raw_url, client)
	if err != nil {
		return err
	}

	var data []string
	for _, apt := range records {
		line := fmt.Sprintf("🏠 Unit: %s\n🛏️ %d Bed | 🛁 %.1f Bath\n💰 $%.2f | 📏 %.0f sqft\n🗓️ %s", apt.Name, apt.Beds, apt.Baths, apt.Rent, apt.SquareFeet, apt.AvailableDateText)
		data = append(data, line)
	}

	var msg_body string
	var full_message string
	if len(data) > 0 {
		msg_body = strings.Join(data, "\n━━━━━━━━━━━━━━━━━\n")
		full_message = fmt.Sprintf("\n🚨 %s Alert 🚨\n\n%s\n", listing_name, msg_body)
	} else {
		full_message = fmt.Sprintf("No available units right now at %s", listing_name)
	}

	// define our payload which requires the chat_id and a message
	payload := map[string]string{
		"chat_id": chat_id,
		"text":    full_message,
	}

	// marshal the payload to []byte type
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	req, err := http.NewRequest("POST", api_url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building telegram request: %q", err)
	}

	req.Header.Set("Content-Type", "application/json")

	t_client := &http.Client{}

	resp, err := t_client.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram request: %q", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received status code %d from telegram", resp.StatusCode)
	}

	return nil
}
