package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

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

// defining regex pattern to find the rental section in the body of the response (same pattern from python project proved reliable)
var pattern = regexp.MustCompile(`rentals:\s*(\[.*?\])\s*,\s*disableMediaCascading`)
var listing_pattern = regexp.MustCompile(`listingName:\s*'([^']+)'`)

func setup_proxies() (*http.Client, error) {
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

func setup_telegram_chat() (string, string, error) {
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
func scrape_apartment_listing(raw_url string, client *http.Client) ([]Apartments, string, error) {
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
		if match := pattern.FindStringSubmatch(body_string); len(match) > 1 {
			var a []Apartments
			var listing_name string

			if listing_match := listing_pattern.FindStringSubmatch(body_string); len(listing_match) > 1 {
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

func send_notification(raw_url string, client *http.Client) error {

	api_url, chat_id, err := setup_telegram_chat()
	if err != nil {
		return err
	}

	records, listing_name, err := scrape_apartment_listing(raw_url, client)
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

func scrape_handler(client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw_url := r.URL.Query().Get("url")
		if raw_url == "" {
			http.Error(w, "`url` query parameter is required", http.StatusBadRequest)
			return
		}

		records, _, err := scrape_apartment_listing(raw_url, client)
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

func chat_handler(client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw_url := r.URL.Query().Get("url")
		if raw_url == "" {
			http.Error(w, "`url` query parameter is required", http.StatusBadRequest)
			return
		}

		if err := send_notification(raw_url, client); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}
}

func create_bash(op_sys string, url string) (script string, err error) {
	if op_sys == "linux" || op_sys == "darwin" {
		script = `#!/bin/bash

if ! command -v curl >/dev/null 2>&1; then
	echo "curl not found. Attempting to install..."
	if command -v apt-get >/dev/null 2>&1; then
		sudo apt-get update && sudo apt-get install -y curl
	elif command -v yum >/dev/null 2>&1; then
		sudo yum install -y curl
	else
		echo "No supported package manager found (apt-get or yum). Please install curl manually."
		exit 1
	fi

	if ! command -v curl >/dev/null 2>&1; then
		echo "Installation of curl failed. Aborting."
		exit 1
	fi
fi

curl -X POST "http://127.0.0.1:8000/chat?url=` + url + `"
`

	} else {
		return "", fmt.Errorf("cannot create bash script for %q", op_sys)
	}
	return script, nil
}

func setup_telegram_bot() (bot_token string, chat_id string, err error) {
	var chat_started string

	fmt.Println("\nBeginning Telegram Bot Setup...")
	fmt.Println("> Open Telegram")
	fmt.Println("> Search for the 'BotFather' (username: @BotFather)")
	fmt.Println("> Start a chat with BotFather and use the command '/newbot' to create a new bot and follow the instructions")
	fmt.Print("> Enter the bot token here: ")
	fmt.Scan(&bot_token)
	fmt.Println("> Click the t.me/<yourbotname> link that BotFather provided, press 'Start' to begin a chat, and send it a message")

	for {
		fmt.Print("> Have you sent your bot a message? (y / n) ")
		fmt.Scan(&chat_started)
		if chat_started == "y" {
			break
		}

		fmt.Print("> Start the chat with your created bot and send it a message. Have you done this? (y / n) ")
		fmt.Scan(&chat_started)
		if chat_started == "y" {
			break
		}
	}

	fmt.Println("> Fetching Chat ID...")

	get_updates_url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates", bot_token)

	resp, err := http.Get(get_updates_url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("get_updates_url returned %d", resp.StatusCode)
	}

	var payload map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", fmt.Errorf("decoding getUpdates JSON: %w", err)
	}

	//fmt.Printf("Raw getUpdates payload: %+v\n", payload)

	resultArr, _ := payload["result"].([]interface{})
	if len(resultArr) == 0 {
		return "", "", fmt.Errorf("no messages found in chat. Please send your bot a message")
	}

	last := resultArr[len(resultArr)-1].(map[string]interface{})

	msg_obj, _ := last["message"].(map[string]interface{})
	chat_obj, _ := msg_obj["chat"].(map[string]interface{})
	float_id, _ := chat_obj["id"].(float64)

	chat_id = strconv.FormatFloat(float_id, 'f', 0, 64)

	fmt.Printf("\n> Retrieved Chat ID: %s\n", chat_id)
	fmt.Println("> Storing Bot Token and Chat ID in environment variables...")

	fmt.Println("\n> Telegram Bot Sucessfully Enabled!")

	return bot_token, chat_id, nil
}

func setup_systemd() error {
	const systemd_template = `[Unit]
	Description=go-apts service
	After=network.target

	[Service]
	WorkingDirectory=%s
	ExecStart=%s
	Restart=on-failure

	[Install]
	WantedBy=multi-user.target
	`

	current_user := os.Getenv("USER")
	if current_user == "" {
		current_user = "root"
	}

	binary_path, _ := filepath.Abs(os.Args[0])
	binary_dir := filepath.Dir(binary_path)

	unitText := fmt.Sprintf(systemd_template, binary_dir, binary_path)

	unit_path := "/etc/systemd/system/go-apts.service"
	if err := os.WriteFile(unit_path, []byte(unitText), 0o644); err != nil {
		return fmt.Errorf("could not write systemd unit: %w", err)
	}

	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload failed: %v (%s)", err, string(out))
	}

	if out, err := exec.Command("systemctl", "enable", "go-apts.service").CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "is enabled") {
			return fmt.Errorf("enable failed: %v (%s)", err, string(out))
		}
	}

	if out, err := exec.Command("systemctl", "restart", "go-apts.service").CombinedOutput(); err != nil {
		return fmt.Errorf("restart failed: %v (%s)", err, string(out))
	}
	return nil
}

func setup_launchd() error {
	label := "com.go-apts.agent"

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not find user home directory: %q", err)
	}

	agents_dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agents_dir, 0o755); err != nil {
		return fmt.Errorf("could not create LaunchAgents dir: %w", err)
	}

	binary_path, err := filepath.Abs(os.Args[0])
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	binary_dir := filepath.Dir(binary_path)

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>

  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>

  <key>WorkingDirectory</key>
  <string>%s</string>

  <key>RunAtLoad</key>
  <true/>

  <key>KeepAlive</key>
  <true/>

  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>`,
		label,
		binary_path,
		binary_dir,
		filepath.Join(home, "Library", "Logs", "go-apts.log"),
		filepath.Join(home, "Library", "Logs", "go-apts.log"),
	)

	plist_path := filepath.Join(agents_dir, label+".plist")
	if err := os.WriteFile(plist_path, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("could not write plist to %s: %w", plist_path, err)
	}

	unload_cmd := exec.Command("launchctl", "unload", plist_path)
	unload_cmd.Stderr = &bytes.Buffer{}
	unload_cmd.Run()

	load_cmd := exec.Command("launchctl", "load", plist_path)
	out, err := load_cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to load LaunchAgent: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func setup_scheduled_task() error {
	var op_sys, url, cron_dir, script_name, script_path string
	var timing int

	//unique_time := fmt.Sprint(time.Now())

	fmt.Println("\nBeginning Scheduled Task Setup...")

	op_sys = runtime.GOOS
	fmt.Println("> Operating System Detected: ", op_sys)

	fmt.Print("> Please enter the URL of the listing you wish to monitor: ")
	fmt.Scan(&url)

	fmt.Println("\n> Building Bash Script...")
	script, err := create_bash(op_sys, url)
	if err != nil {
		return fmt.Errorf("making bash: %q", err)
	}

	fmt.Println("> Creating Scheduled Task...")
	fmt.Print("> Schedule this task Hourly (1), Daily (2), Weekly (3), or Monthly (4) ? ")
	fmt.Scan(&timing)

	if op_sys == "linux" {

		switch timing {
		case 1:
			cron_dir = "/etc/cron.hourly"
		case 2:
			cron_dir = "/etc/cron.daily"
		case 3:
			cron_dir = "/etc/cron.weekly"
		case 4:
			cron_dir = "/etc/cron.monthly"
		default:
			return fmt.Errorf("cannot set timeframe for that")
		}

		// TODO make sure script_name is a unique name so user can add multiple tasks
		script_name = "go-apts-schedule"
		script_path = filepath.Join(cron_dir, script_name)

		if err := os.WriteFile(script_path, []byte(script), 0755); err != nil {
			return fmt.Errorf("failed to write script to %s: %w", script_path, err)
		}
	} else if op_sys == "darwin" {
		return fmt.Errorf("cannot create scheduled task for macos (coming soon)")
	} else {
		return fmt.Errorf("unsupported os: %q", op_sys)
	}

	fmt.Println("\n> Scheduled Task Sucessfully Built!")

	return nil
}

func setup_go_apts() error {
	var proxies_enabled, telegram_enabled, telly_setup, bot_token, chat_id, always_on_enabled, sch_task_enabled, op_sys string
	var err error

	m, _ := godotenv.Read(".env")
	op_sys = runtime.GOOS

	fmt.Print("\n\nDo you want to enable proxies with OxyLabs? Proxies help avoid IP blocking (y / n) ")
	fmt.Scan(&proxies_enabled)
	if strings.EqualFold(proxies_enabled, "y") {
		oxy_name, _ := os.LookupEnv("OXYLABS_USERNAME")
		oxy_pass, _ := os.LookupEnv("OXYLABS_PASSWORD")
		oxy_host, _ := os.LookupEnv("OXYLABS_PROXY_HOST")
		oxy_port, _ := os.LookupEnv("OXYLABS_PROXY_PORT")
		if oxy_name != "" || oxy_pass != "" || oxy_host != "" || oxy_port != "" {
		} else {
			proxies_enabled = "n"
			fmt.Println("\nUnable to locate all OxyLabs credentials")
			fmt.Println("Starting without proxies enabled...")
		}
	} else {
		proxies_enabled = "n"
	}

	fmt.Print("\nDo you want to enable notifications with Telegram? (y / n) ")
	fmt.Scan(&telegram_enabled)

	if strings.EqualFold(telegram_enabled, "y") {
		telly_test := os.Getenv("TELEGRAM_BOT_TOKEN")

		if telly_test == "" {
			fmt.Println("\nYou must setup a Telgram bot and add your credentials")
			fmt.Print("Do you wish to set that up now? (y / n) ")
			fmt.Scan(&telly_setup)

			if strings.EqualFold(telly_setup, "y") {
				bot_token, chat_id, err = setup_telegram_bot()
				m["TELEGRAM_BOT_TOKEN"] = bot_token
				m["TELEGRAM_CHAT_ID"] = chat_id
				godotenv.Write(m, ".env")
				if err != nil {
					return fmt.Errorf("error during Telegram bot setup: %q", err)
				}
				telegram_enabled = "y"
			} else {
				telegram_enabled = "n"
			}
		}
	}

	m["proxies_enabled"] = proxies_enabled
	m["telegram_enabled"] = telegram_enabled

	godotenv.Write(m, ".env")

	os.Setenv("TELEGRAM_BOT_TOKEN", bot_token)
	os.Setenv("TELEGRAM_CHAT_ID", chat_id)
	os.Setenv("proxies_enabled", proxies_enabled)
	os.Setenv("telegram_enabled", telegram_enabled)

	godotenv.Load(".env")

	fmt.Print("\nDo you want to setup Go Apts as an always on service? (y / n) ")
	fmt.Scan(&always_on_enabled)

	if strings.EqualFold(always_on_enabled, "y") {
		switch op_sys {
		case "linux":
			err := setup_systemd()
			if err != nil {
				log.Fatal(err)
			}
		case "darwin":
			err := setup_launchd()
			if err != nil {
				log.Fatal(err)
			}
		default:
			return fmt.Errorf("unsupported os detected: %q", op_sys)
		}
	}

	if strings.EqualFold(always_on_enabled, "y") && strings.EqualFold(telegram_enabled, "y") {
		fmt.Print("\nDo you want to monitor a listing with Telegram? (y / n) ")
		fmt.Scan(&sch_task_enabled)

		if strings.EqualFold(sch_task_enabled, "y") {
			err := setup_scheduled_task()
			if err != nil {
				return err
			} else {
				log.Fatal()
			}
		} else if strings.EqualFold(sch_task_enabled, "n") {
			log.Fatal()
		}
	}
	return nil
}

func main() {
	var proxies_enabled, telegram_enabled string

	godotenv.Load(".env")

	client := &http.Client{}

	r := chi.NewRouter()

	fmt.Println(`
 ______     ______        ______     ______   ______   ______    
/\  ___\   /\  __ \      /\  __ \   /\  == \ /\__  _\ /\  ___\   
\ \ \__ \  \ \ \/\ \     \ \  __ \  \ \  _-/ \/_/\ \/ \ \___  \  
 \ \_____\  \ \_____\     \ \_\ \_\  \ \_\      \ \_\  \/\_____\ 
  \/_____/   \/_____/      \/_/\/_/   \/_/       \/_/   \/_____/ `)

	setup_mode := flag.Bool("setup", false, "Run interactive configuration and exit")
	flag.Parse()

	oxy_name, _ := os.LookupEnv("OXYLABS_USERNAME")
	oxy_pass, _ := os.LookupEnv("OXYLABS_PASSWORD")
	oxy_host, _ := os.LookupEnv("OXYLABS_PROXY_HOST")
	oxy_port, _ := os.LookupEnv("OXYLABS_PROXY_PORT")
	telegram_bot_token := os.Getenv("TELEGRAM_BOT_TOKEN")
	telegram_chat_id := os.Getenv("TELEGRAM_CHAT_ID")
	proxies_enabled, _ = os.LookupEnv("proxies_enabled")
	telegram_enabled, _ = os.LookupEnv("telegram_enabled")

	switch {
	case oxy_name == "" || oxy_pass == "" || oxy_host == "" || oxy_port == "":
		proxies_enabled = "n"
	case telegram_bot_token == "" || telegram_chat_id == "":
		telegram_enabled = "n"
	}

	if *setup_mode {
		err := setup_go_apts()
		if err != nil {
			log.Fatal(err)
		}

		proxies_enabled, _ = os.LookupEnv("proxies_enabled")
		telegram_enabled, _ = os.LookupEnv("telegram_enabled")
	}

	switch {
	case strings.EqualFold(proxies_enabled, "y") && strings.EqualFold(telegram_enabled, "y"):
		proxy_client, err := setup_proxies()
		if err != nil {
			log.Fatal(err)
		}
		r.Get("/apts", scrape_handler(proxy_client))
		r.Post("/chat", chat_handler(proxy_client))
		fmt.Println("\n<GO APTS> /apts and /chat with proxies running on port 8000")
	case strings.EqualFold(proxies_enabled, "n") && strings.EqualFold(telegram_enabled, "y"):
		r.Get("/apts", scrape_handler(client))
		r.Post("/chat", chat_handler(client))
		fmt.Println("\n<GO APTS> /apts and /chat running on port 8000")
	case strings.EqualFold(proxies_enabled, "y") && strings.EqualFold(telegram_enabled, "n"):
		proxy_client, err := setup_proxies()
		if err != nil {
			log.Fatal(err)
		}
		r.Get("/apts", scrape_handler(proxy_client))
		fmt.Println("\n<GO APTS> /apts with proxies running on port 8000")
	default:
		r.Get("/apts", scrape_handler(client))
		fmt.Println("\n<GO APTS> /apts running on port 8000")
	}

	log.Fatal(http.ListenAndServe("0.0.0.0:8000", r))
}
