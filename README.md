# ZillowSaves

Updates a Google Sheet with the number of Zillow Saves from emails received on a Yahoo Mail account using IMAP.

## Setup

### 1. Google API Credentials

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google Sheets API
4. Create credentials (OAuth 2.0 Client ID)
5. Download the credentials file and save it as `credentials.json` in this directory

### 2. Yahoo Mail App Password

**Important**: You need a Yahoo App Password, not your regular password!

1. Go to [Yahoo Account Security](https://login.yahoo.com/account/security)
2. Turn on 2-step verification if not already enabled
3. Generate an app password for "Mail"
4. Use this app password in your configuration

### 3. Configuration

1. Copy `config.json.example` to `config.json`
2. Update the configuration:
   - `spreadsheet_id`: Your Google Sheet ID
   - `range`: Cell range (default: `Sheet1!A:Z`)
   - `yahoo_username`: Your Yahoo email address
   - `yahoo_app_password`: The app password from step 2

### 4. Running the Program

```bash
# Using the helper script
./run.sh

# Or manually
go run . config.json
```

On first run, you'll be prompted to authorize the application in your browser for Google Sheets access.
You'll need to extract the Google auth code from the redirect URL and paste it into zillowsaves.

## How it Works

The program:

1. **Accesses Google Sheets**: Retrieves all rows from the specified sheet using Google Sheets API
2. **Connects to Yahoo Mail**: Uses IMAP to directly access your Yahoo Mail account
3. **Searches for Emails**: Finds emails with subject "Your Daily Listing Report: 9121 Blackhawk Rd" since July 15, 2025
4. **Extracts Data**: Parses email content to find Zillow save counts using multiple regex patterns
5. **Displays Results**: Shows both sheet data and email data with extracted save counts

## Email Parsing

The program searches for the save count by matching against several patterns in email content. 

## Security

- Keep your `credentials.json`, `token.json`, and `config.json` files secure
- Never commit them to version control
- Use Yahoo App Passwords, never your regular password
- The program requests minimal scopes (Google Sheets read-only)

## Building

To create a standalone executable:

```bash
go build -o zillowsaves .
./zillowsaves config.json
```

## Troubleshooting

- **Authentication Errors**: Ensure you're using a Yahoo App Password, not your regular password
- **No Emails Found**: Check your email subject and date filters
- **Google Sheets Errors**: Verify your spreadsheet ID and that the sheet is accessible
