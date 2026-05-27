package main

import (
	"context"
	"errors"
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

func resolveUsers(api *slack.Client, names ...string) ([]string, error) {
	ids := make([]string, 0, len(names))
	namesMap := map[string]struct{}{}
	for i := range names {
		namesMap[strings.TrimPrefix(names[i], "@")] = struct{}{}
	}
	users, err := api.GetUsers()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if _, ok := namesMap[u.Name]; ok {
			ids = append(ids, u.ID)
		}
	}
	if len(ids) < len(names) {
		return nil, errors.New("unable to find some users")
	}
	if len(ids) > len(names) {
		return nil, errors.New("too many users were found")
	}
	return ids, nil
}

func resolveChannel(api *slack.Client, name string) (string, error) {
	name = strings.TrimPrefix(name, "#")
	// Accept channel IDs directly. C=public/private, G=mpim or legacy private, D=IM.
	if strings.HasPrefix(name, "C") || strings.HasPrefix(name, "G") || strings.HasPrefix(name, "D") {
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
			cmdBackup(),
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
