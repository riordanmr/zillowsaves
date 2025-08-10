#!/bin/bash

# ZillowSaves Setup and Run Script

echo "ZillowSaves - Zillow Email to Google Sheets Processor"
echo "=================================================="

# Check if config.json exists
if [ ! -f "config.json" ]; then
    echo "Creating config.json from example..."
    if [ -f "config.json.example" ]; then
        cp config.json.example config.json
        echo "✓ Created config.json"
        echo "⚠️  Please edit config.json with your Google Sheet ID"
    else
        echo "❌ config.json.example not found"
        exit 1
    fi
fi

# Check if credentials.json exists
if [ ! -f "google-credentials.json" ]; then
    echo "❌ google-credentials.json not found"
    echo "Please follow the setup instructions in README.md to:"
    echo "1. Create a Google Cloud project"
    echo "2. Enable Google Sheets and Gmail APIs"
    echo "3. Create OAuth 2.0 credentials"
    echo "4. Download credentials.json to this directory"
    exit 1
fi

# Build the program
echo "Building ZillowSaves..."
go build -o zillowsaves .
if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi
echo "✓ Build successful"

# Run the program
echo "Running ZillowSaves..."
./zillowsaves config.json
