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
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const (
	dateFormat         = "2006-01-02"
	emailSubject       = "Your Daily Listing Report: 9121 Blackhawk Rd"
	fallbackFilterDate = "2025-05-21"
)

type Config struct {
	SpreadsheetID    string `json:"spreadsheet_id"`
	Range            string `json:"range"`
	YahooUsername    string `json:"yahoo_username"`
	YahooAppPassword string `json:"yahoo_app_password"`
}

type EmailMessage struct {
	Subject     string
	Date        time.Time
	Content     string
	ID          string
	ZillowSaves int
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

func getSheetData(srv *sheets.Service, spreadsheetID, readRange string) ([][]interface{}, error) {
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve data from sheet: %v", err)
	}

	return resp.Values, nil
}

func appendToSheet(srv *sheets.Service, spreadsheetID, sheetRange string, emails []*EmailMessage) error {
	// Prepare the data to append
	var values [][]interface{}
	for _, email := range emails {
		// Format date as YYYY-MM-DD
		dateStr := email.Date.Format("2006-01-02")

		// Create row: [Date, Saves Count]
		row := []interface{}{dateStr, email.ZillowSaves}
		values = append(values, row)
	}

	if len(values) == 0 {
		fmt.Println("No email data to append to sheet")
		return nil
	}

	// Create the request body
	valueRange := &sheets.ValueRange{
		Values: values,
	}

	// Append the data to the sheet
	_, err := srv.Spreadsheets.Values.Append(spreadsheetID, sheetRange, valueRange).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Do()

	if err != nil {
		return fmt.Errorf("unable to append data to sheet: %v", err)
	}

	fmt.Printf("Successfully appended %d rows to Google Sheet\n", len(values))
	return nil
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

func getYahooEmails(username, appPassword, subject, since string) ([]*EmailMessage, error) {
	return connectToYahooIMAP(username, appPassword, subject, since)
}

func processData(srv *sheets.Service, config *Config, rows [][]interface{}, emails []*EmailMessage) {
	bOK := true
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
		count, err := extractZillowSavesCount(email.Content)
		if err == nil {
			email.ZillowSaves = count
		} else {
			bOK = false
			email.ZillowSaves = -1 // Indicate error with -1
			fmt.Printf("  Zillow Saves: [Error: %v]\n", err)
			break
		}
		fmt.Printf("  Saves Count: %d\n", email.ZillowSaves)

		fmt.Println()
	}

	if bOK {
		appendToSheet(srv, config.SpreadsheetID, config.Range, emails)
	}
}

func doZillow(config *Config) error {
	googleCtx := context.Background()

	// Google Sheets
	fmt.Println("Accessing Google Sheets...")
	httpClient, err := getGoogleClient(googleCtx)
	if err != nil {
		log.Fatalf("Unable to create Google client: %v", err)
	}
	srv, err := sheets.NewService(googleCtx, option.WithHTTPClient(httpClient))
	if err != nil {
		return fmt.Errorf("unable to retrieve Sheets client: %v", err)
	}

	rows, err := getSheetData(srv, config.SpreadsheetID, config.Range)
	if err != nil {
		log.Fatalf("Failed to get sheet data: %v", err)
	}
	fmt.Printf("Retrieved %d rows from Google Sheet\n", len(rows))

	// Determine filterDate from last row in sheet
	var dynamicFilterDate string
	if len(rows) > 0 {
		lastRow := rows[len(rows)-1]
		if len(lastRow) > 0 && lastRow[0] != nil {
			// Get the date from the first column of the last row
			dateStr := strings.TrimSpace(fmt.Sprintf("%v", lastRow[0]))

			// Parse and validate the date
			if parsedDate, err := time.Parse("2006-01-02", dateStr); err == nil {
				// Add one day to start searching from the day after the last entry
				nextDay := parsedDate.AddDate(0, 0, 1)
				dynamicFilterDate = nextDay.Format("2006-01-02")
				fmt.Printf("Using filter date from sheet: %s (day after last entry: %s)\n", dynamicFilterDate, dateStr)
			} else {
				// Try alternative date formats if the standard format fails
				formats := []string{"1/2/2006", "01/02/2006", "2006/01/02", "Jan 2, 2006"}
				parsed := false
				for _, format := range formats {
					if parsedDate, err := time.Parse(format, dateStr); err == nil {
						nextDay := parsedDate.AddDate(0, 0, 1)
						dynamicFilterDate = nextDay.Format("2006-01-02")
						fmt.Printf("Using filter date from sheet: %s (parsed from %s, day after last entry)\n", dynamicFilterDate, dateStr)
						parsed = true
						break
					}
				}
				if !parsed {
					fmt.Printf("Warning: Could not parse date '%s' from last row, using default filter date: %s\n", dateStr, fallbackFilterDate)
					dynamicFilterDate = fallbackFilterDate
				}
			}
		} else {
			fmt.Printf("Warning: Last row has no data in first column, using default filter date: %s\n", fallbackFilterDate)
			dynamicFilterDate = fallbackFilterDate
		}
	} else {
		fmt.Printf("Warning: No rows found in sheet, using default filter date: %s\n", fallbackFilterDate)
		dynamicFilterDate = fallbackFilterDate
	}

	// Yahoo Mail via IMAP
	fmt.Println("Accessing Yahoo Mail via IMAP...")
	emails, err := getYahooEmails(config.YahooUsername, config.YahooAppPassword, emailSubject, dynamicFilterDate)
	if err != nil {
		log.Fatalf("Failed to get Yahoo emails: %v", err)
	}
	fmt.Printf("Found %d emails since %s\n", len(emails), dynamicFilterDate)

	// Sort emails by date (oldest first)
	sort.Slice(emails, func(i, j int) bool {
		return emails[i].Date.Before(emails[j].Date)
	})
	fmt.Println("Sorted emails by date (oldest first)")

	// Process results
	fmt.Println("\nProcessing results...")
	processData(srv, config, rows, emails)
	return nil
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

	if err := doZillow(config); err != nil {
		log.Fatalf("Zillow processing failed: %v", err)
	}

}
