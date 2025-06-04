```
 ______     ______        ______     ______   ______   ______    
/\  ___\   /\  __ \      /\  __ \   /\  == \ /\__  _\ /\  ___\   
\ \ \__ \  \ \ \/\ \     \ \  __ \  \ \  _-/ \/_/\ \/ \ \___  \  
 \ \_____\  \ \_____\     \ \_\ \_\  \ \_\      \ \_\  \/\_____\ 
  \/_____/   \/_____/      \/_/\/_/   \/_/       \/_/   \/_____/

```

GO APTS is an adaptation of my apartments-api written in Python. I'm currently learning Go and think it's a great lesson to translate an existing project. 

The application contains two routes: `GET /apts` and `POST /chat`, requires one query param: `url`, and returns the available units. (Only `/apts` is enabled by default)

`/apts` and `/chat` can be used with OxyLabs Residential Proxies to avoid IP blocking. To use proxies, you must have your OxyLabs credentials set in a `.env` file located in the root of the project directory (must be next to the executable if compiled) and have proxies enabled.

To enable the `/chat` endpoint, Telegram `.env` variables must be set. If you do not have a Telegram bot created, run `./go-apts` with the `--setup` flag and follow the prompts.

Run with the `--setup` flag to:
  - Enable / disable proxies
  - Enable / disable `/chat` endpoint
  - Create a Telegram Bot
  - Create a `systemd` service for always on service (Linux only, `launchd` coming soon!)
  - Create a scheduled task with cron (hourly, daily, weekly, monthly) to request `/chat` with a provided Apartments.com URL

Creating a scheduled task through GO APTS is useful to monitor an apartment listing. If proxies are NOT enabled, you run the risk of getting your IP blocked by Apartments.com.

Scheduled tasks can only be created if `/chat` is enabled and a `systemd` service is created. This currently works with Linux only but I will implement `launchd` support for MacOS soon.

This application only accepts Apartments.com listings but will eventually accept URLs from other providers like Zillow.com.

### Usage

1. **Clone this repo**
```bash
git clone https://github.com/AnthonyBliss1/go-apts.git
```

or 

**Use go install**
```bash
go install github.com/anthonybliss1/go-apts
```

2. **Create .env file in the project directory and store credentials**
```bash
cp .env.template .env
```

3. **Run the application**
```bash
go run go-apts.go
```

or 

```bash
go run go-apts.go --setup
```

### Building an Executable File

1. **Run the build command**
```bash 
go build go-apts.go
```

2. **Run the built project**
```bash
./go-apts
```

or 

```bash
./go-apts --setup
```