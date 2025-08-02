package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// connectToYahooIMAPV1 connects to Yahoo Mail via IMAP v1 library
func connectToYahooIMAP(username, password, subject, since string) ([]*EmailMessage, error) {
	// Parse the filter date
	filterTime, err := time.Parse("2006-01-02", since)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %v", err)
	}

	// Connect to Yahoo IMAP server
	c, err := client.DialTLS("imap.mail.yahoo.com:993", &tls.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Yahoo IMAP: %v", err)
	}
	defer c.Logout()

	// Login
	if err := c.Login(username, password); err != nil {
		return nil, fmt.Errorf("failed to login: %v", err)
	}

	// Select INBOX
	_, err = c.Select("INBOX", false)
	if err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %v", err)
	}

	// Search for emails since the date
	criteria := imap.NewSearchCriteria()
	criteria.Since = filterTime

	uids, err := c.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("search failed: %v", err)
	}

	if len(uids) == 0 {
		return []*EmailMessage{}, nil
	}

	fmt.Printf("Found %d emails since %s\n", len(uids), since)

	// Fetch messages
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)

	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope, imap.FetchRFC822}, messages)
	}()

	var emailMessages []*EmailMessage
	for msg := range messages {
		if msg.Envelope == nil {
			continue
		}

		// Check if subject contains our target text
		if !strings.Contains(strings.ToLower(msg.Envelope.Subject), strings.ToLower(subject)) {
			continue
		}

		email := &EmailMessage{
			Subject: msg.Envelope.Subject,
			Date:    msg.Envelope.Date,
			ID:      fmt.Sprintf("%d", msg.SeqNum),
		}

		// Read body content
		for _, r := range msg.Body {
			if b, err := ioutil.ReadAll(r); err == nil {
				email.Content = string(b)
				break
			}
		}

		emailMessages = append(emailMessages, email)
	}

	if err := <-done; err != nil {
		return emailMessages, fmt.Errorf("fetch failed: %v", err)
	}

	return emailMessages, nil
}
