package mattermost

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/otis/internal/notify"
)

type Options struct {
	URL        string
	TokenEnv   string
	Token      string
	HTTPClient *http.Client
}

type Notifier struct {
	options Options
}

type webhookPayload struct {
	Channel string `dd:",+omitempty"`
	Text    string `dd:",+required"`
}

func New(options Options) *Notifier {
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Notifier{options: options}
}

func (n *Notifier) Post(ctx context.Context, notification notify.Notification) error {
	if len(notification.Findings) == 0 || strings.TrimSpace(n.options.URL) == "" {
		return nil
	}
	endpoint, err := n.endpoint()
	if err != nil {
		return err
	}
	raw, err := dd.UnbindJSON(webhookPayload{
		Channel: notification.Channel,
		Text:    notify.Render(notification),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.options.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mattermost webhook failed: %s", resp.Status)
	}
	return nil
}

func (n *Notifier) endpoint() (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(n.options.URL), "/")
	token := n.options.Token
	if token == "" && n.options.TokenEnv != "" {
		token = os.Getenv(n.options.TokenEnv)
		if token == "" {
			return "", fmt.Errorf("%s is required for mattermost webhook", n.options.TokenEnv)
		}
	}
	if token != "" {
		endpoint += "/" + strings.TrimLeft(token, "/")
	}
	return endpoint, nil
}
