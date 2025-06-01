## Apartment Scraper
This project is an adaptation of my apartments-api written in Python. I'm currently learning Go and think it's a great lesson to translate an existing project. Currently, the application contains one route "/apts", requires one param: 'url', and returns the available units. It also utilizes OxyLabs Residential Proxies for larger scale scraping. I will build on this repo incrementally to be a fully functional API solution. This solution will eventually accept URLs from other providers like Zillow.com

### Usage

1. **Clone this repo**
```bash
git clone https://github.com/AnthonyBliss1/apts-go.git
```

2. **Create .env file and store oxylabs variables**
```bash
cp .env.template .env
```

3. **Run the application (Assuming Go is installed)**
```bash
go run apts.go
```

### If you want to build an executable

```bash 
go build apts.go
```