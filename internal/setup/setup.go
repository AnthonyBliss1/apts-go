package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func Create_bash(op_sys string, url string) (script string, err error) {
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

func Setup_telegram_bot() (bot_token string, chat_id string, err error) {
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

func Setup_systemd() error {
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

func Setup_launchd() error {
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

func Setup_scheduled_task() error {
	var op_sys, url, cron_dir, script_name, script_path string
	var timing int

	//unique_time := fmt.Sprint(time.Now())

	fmt.Println("\nBeginning Scheduled Task Setup...")

	op_sys = runtime.GOOS
	fmt.Println("> Operating System Detected: ", op_sys)

	fmt.Print("> Please enter the URL of the listing you wish to monitor: ")
	fmt.Scan(&url)

	fmt.Println("\n> Building Bash Script...")
	script, err := Create_bash(op_sys, url)
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

func Setup_go_apts() error {
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
				bot_token, chat_id, err = Setup_telegram_bot()
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
			err := Setup_systemd()
			if err != nil {
				log.Fatal(err)
			}
		case "darwin":
			err := Setup_launchd()
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
			err := Setup_scheduled_task()
			if err != nil {
				return err
			} else {
				log.Fatal()
			}
		} else if strings.EqualFold(sch_task_enabled, "n") {
			log.Fatal()
		}
	} else if strings.EqualFold(always_on_enabled, "y") {
		log.Fatal()
	}
	return nil
}
