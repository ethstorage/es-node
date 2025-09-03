package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/pkg/errors"
)

const (
	websiteOfflineAlertContent = "<p><b>Alert: </b>%s </p><p><b>Message: </b>Web site is off line. RPC: %s.</p>"
)

type WebsiteOnlineChecker struct {
	Name    string `json:"name"`
	EmailTo string `json:"email-to"`
	URL     string `json:"url"`
}

func newWebsiteOnlineChecker(params map[string]string, emailTo string) (*WebsiteOnlineChecker, error) {
	name, url := params["name"], params["url"]
	if name == "" || url == "" {
		return nil, errors.New("invalid params to load WebsiteOnlineChecker")
	}

	return &WebsiteOnlineChecker{
		Name:    name,
		EmailTo: emailTo,
		URL:     url,
	}, nil
}

func (c *WebsiteOnlineChecker) Check(lg log.Logger) (bool, string, string) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(c.URL)
	if err != nil {
		lg.Error("Failed to request web site", "alert", c.Name, "url", c.URL, "err", err)
		return true, fmt.Sprintf(errorContent, c.Name, err.Error()), c.EmailTo
	}
	defer resp.Body.Close()

	lg.Info("Check last block", "alert", c.Name, "url", c.URL, "status", resp.StatusCode)
	// Check the HTTP status code
	if resp.StatusCode == http.StatusOK {
		return false, "", c.EmailTo
	} else {
		return true, fmt.Sprintf(websiteOfflineAlertContent, c.Name, c.URL), c.EmailTo
	}
}
