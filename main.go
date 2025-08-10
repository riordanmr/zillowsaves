package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	dateFormat   = "2006-01-02"
	emailSubject = "Your Daily Listing Report: 9121 Blackhawk Rd"
	filterDate   = "2025-05-21"
)

type Config struct {
	SpreadsheetID    string `json:"spreadsheet_id"`
	Range            string `json:"range"`
	YahooUsername    string `json:"yahoo_username"`
	YahooAppPassword string `json:"yahoo_app_password"`
}

type EmailMessage struct {
	Subject string
	Date    time.Time
	Content string
	ID      string
}

func loadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var config Config
	return &config, json.Unmarshal(data, &config)
}

func getGoogleClient(ctx context.Context) (*http.Client, error) {
	googleCredsFilename := "google-credentials.json"
	b, err := ioutil.ReadFile(googleCredsFilename)
	if err != nil {
		return nil, fmt.Errorf("unable to read %s: %v", googleCredsFilename, err)
	}

	config, err := google.ConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %v", err)
	}

	tokFile := "google-token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(ctx, tok), nil
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to this URL and enter the authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	return tok, json.NewDecoder(f).Decode(tok)
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getSheetData(ctx context.Context, httpClient *http.Client, spreadsheetID, readRange string) ([][]interface{}, error) {
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve Sheets client: %v", err)
	}

	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve data from sheet: %v", err)
	}

	return resp.Values, nil
}

func getYahooEmails(username, appPassword, subject, since string) ([]*EmailMessage, error) {
	return connectToYahooIMAP(username, appPassword, subject, since)
}
func processData(rows [][]interface{}, emails []*EmailMessage) {
	fmt.Println("\n=== Google Sheets Data ===")
	for i, row := range rows {
		fmt.Printf("Row %d: %v\n", i+1, row)
		if i >= 4 {
			fmt.Printf("... and %d more rows\n", len(rows)-5)
			break
		}
	}

	fmt.Println("\n=== Yahoo Mail Data ===")
	for i, email := range emails {
		fmt.Printf("Email %d:\n", i+1)
		fmt.Printf("  Subject: %s\n", email.Subject)
		fmt.Printf("  Date: %s\n", email.Date.Format("2006-01-02 15:04:05"))
		fmt.Printf("  ID: %s\n", email.ID)

		extractZillowSaves(email)
		fmt.Println()
	}
}

func extractZillowSaves(email *EmailMessage) {
	if email.Content == "" {
		fmt.Printf("  Zillow Saves: [No content]\n")
		return
	}

	count, err := extractZillowSavesCount(email.Content)
	if err != nil {
		fmt.Printf("  Zillow Saves: [Error: %v]\n", err)
		return
	}

	fmt.Printf("  Zillow Saves: %d\n", count)

	// Show snippet for debugging
	snippet := email.Content
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}
	fmt.Printf("  Content: %s\n", strings.ReplaceAll(snippet, "\n", " "))
}

func extractZillowSavesCount(content string) (int, error) {
	patterns := []string{
		`(\d+)\s+saves?`,
		// `saved\s+(\d+)\s+times?`,
		// `(\d+)\s+people?\s+saved`,
		// `total\s+saves?:\s*(\d+)`,
		// `save\s+count:\s*(\d+)`,
		// `(\d+)\s+favorites?`,
		// `favorited\s+(\d+)\s+times?`,
	}

	lowerContent := strings.ToLower(content)

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(lowerContent)
		if len(matches) > 1 {
			if count, err := strconv.Atoi(matches[1]); err == nil {
				return count, nil
			}
		}
	}

	return 0, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: zillowsaves <config.json>")
		fmt.Println("Example config.json:")
		fmt.Println(`{
  "spreadsheet_id": "your-google-sheet-id",
  "range": "Sheet1!A:Z", 
  "yahoo_username": "your-email@yahoo.com",
  "yahoo_app_password": "your-yahoo-app-password"
}`)
		fmt.Println("\nIMPORTANT: You need a Yahoo App Password!")
		fmt.Println("Get one at: https://login.yahoo.com/account/security")
		os.Exit(1)
	}

	config, err := loadConfig(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()

	// Google Sheets
	fmt.Println("Accessing Google Sheets...")
	httpClient, err := getGoogleClient(ctx)
	if err != nil {
		log.Fatalf("Unable to create Google client: %v", err)
	}

	rows, err := getSheetData(ctx, httpClient, config.SpreadsheetID, config.Range)
	if err != nil {
		log.Fatalf("Failed to get sheet data: %v", err)
	}
	fmt.Printf("Retrieved %d rows from Google Sheet\n", len(rows))

	// Yahoo Mail via IMAP
	fmt.Println("Accessing Yahoo Mail via IMAP...")
	emails, err := getYahooEmails(config.YahooUsername, config.YahooAppPassword, emailSubject, filterDate)
	if err != nil {
		log.Fatalf("Failed to get Yahoo emails: %v", err)
	}
	fmt.Printf("Found %d emails since %s\n", len(emails), filterDate)

	// Process results
	fmt.Println("\nProcessing results...")
	processData(rows, emails)
}
