package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

func cmdDM() *cli.Command {
	return &cli.Command{
		Name:    "dm",
		Aliases: []string{"d"},
		Usage:   "ユーザーにDMを送信",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "message",
				Aliases:  []string{"m"},
				Usage:    "送信するメッセージ (- で標準入力から読み込み)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "user",
				Aliases:  []string{"u"},
				Usage:    "送信先ユーザー",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			message := cmd.String("message")
			user := cmd.String("user")

			if message == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("stdin error: %w", err)
				}
				message = strings.TrimRight(string(b), "\n")
			}

			userID, err := resolveUser(api, user)
			if err != nil {
				return fmt.Errorf("user error: %w", err)
			}
			ch, _, _, err := api.OpenConversation(&slack.OpenConversationParameters{
				Users: []string{userID},
			})
			if err != nil {
				return fmt.Errorf("open conversation error: %w", err)
			}

			_, _, err = api.PostMessage(
				ch.ID,
				slack.MsgOptionText(message, false),
			)
			if err != nil {
				return fmt.Errorf("post error: %w", err)
			}
			return nil
		},
	}
}
