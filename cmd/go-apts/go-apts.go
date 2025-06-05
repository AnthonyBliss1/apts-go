package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	handlers "github.com/anthonybliss1/go-apts/api/handlers"
	utils "github.com/anthonybliss1/go-apts/api/utils"
	setup "github.com/anthonybliss1/go-apts/internal/setup"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

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
		err := setup.Setup_go_apts()
		if err != nil {
			log.Fatal(err)
		}

		proxies_enabled, _ = os.LookupEnv("proxies_enabled")
		telegram_enabled, _ = os.LookupEnv("telegram_enabled")
	}

	switch {
	case strings.EqualFold(proxies_enabled, "y") && strings.EqualFold(telegram_enabled, "y"):
		proxy_client, err := utils.Create_proxies()
		if err != nil {
			log.Fatal(err)
		}
		r.Get("/apts", handlers.Scrape_handler(proxy_client))
		r.Post("/chat", handlers.Chat_handler(proxy_client))
		fmt.Println("\n<GO APTS> /apts and /chat with proxies running on port 8000")
	case strings.EqualFold(proxies_enabled, "n") && strings.EqualFold(telegram_enabled, "y"):
		r.Get("/apts", handlers.Scrape_handler(client))
		r.Post("/chat", handlers.Chat_handler(client))
		fmt.Println("\n<GO APTS> /apts and /chat running on port 8000")
	case strings.EqualFold(proxies_enabled, "y") && strings.EqualFold(telegram_enabled, "n"):
		proxy_client, err := utils.Create_proxies()
		if err != nil {
			log.Fatal(err)
		}
		r.Get("/apts", handlers.Scrape_handler(proxy_client))
		fmt.Println("\n<GO APTS> /apts with proxies running on port 8000")
	default:
		r.Get("/apts", handlers.Scrape_handler(client))
		fmt.Println("\n<GO APTS> /apts running on port 8000")
	}

	log.Fatal(http.ListenAndServe("0.0.0.0:8000", r))
}
