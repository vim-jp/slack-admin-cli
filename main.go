package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

var slackURLPattern = regexp.MustCompile(`/archives/([^/]+)/p(\d{10})(\d{6})`)

// parseMessageURL parses a Slack message URL and returns channelID and thread_ts.
func parseMessageURL(url string) (channelID, threadTS string, ok bool) {
	m := slackURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2] + "." + m[3], true
}

func resolveUser(api *slack.Client, name string) (string, error) {
	name = strings.TrimPrefix(name, "@")
	users, err := api.GetUsers()
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if u.Name == name {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("user %q not found", name)
}

func resolveChannel(api *slack.Client, name string) (string, error) {
	name = strings.TrimPrefix(name, "#")
	// Accept channel IDs directly.
	if strings.HasPrefix(name, "C") || strings.HasPrefix(name, "G") {
		return name, nil
	}
	var cursor string
	for {
		params := &slack.GetConversationsParameters{
			Cursor:          cursor,
			Limit:           1000,
			ExcludeArchived: true,
			Types:           []string{"public_channel", "private_channel"},
		}
		channels, nextCursor, err := api.GetConversations(params)
		if err != nil {
			return "", err
		}
		for _, ch := range channels {
			if ch.Name == name {
				return ch.ID, nil
			}
		}
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}
	return "", fmt.Errorf("channel %q not found", name)
}

func newAPI() *slack.Client {
	botToken := os.Getenv("SLACK_BOT_TOKEN")
	if botToken == "" {
		fmt.Fprintln(os.Stderr, "SLACK_BOT_TOKEN required")
		os.Exit(1)
	}
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if appToken == "" {
		fmt.Fprintln(os.Stderr, "SLACK_APP_TOKEN required")
		os.Exit(1)
	}
	return slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)
}

func main() {
	_ = godotenv.Load()

	cmd := &cli.Command{
		Name:  "slack-admin-cli",
		Usage: "Slack管理用CLIツール",
		Commands: []*cli.Command{
			cmdAction(),
			cmdList(),
			cmdPost(),
			cmdDM(),
			cmdEdit(),
			cmdBot(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
