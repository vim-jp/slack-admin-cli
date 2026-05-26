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
			&cli.StringSliceFlag{
				Name:     "user",
				Aliases:  []string{"u"},
				Usage:    "送信先ユーザー",
				Required: true,
			},
			&cli.BoolFlag{
				Name:     "without-resolve",
				Usage:    "ユーザーをIDで指定して、名前による解決を行わない",
				Required: false,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			message := cmd.String("message")
			users := cmd.StringSlice("user")
			withoutResolve := cmd.Bool("without-resolve")

			if message == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("stdin error: %w", err)
				}
				message = strings.TrimRight(string(b), "\n")
			}

			var userIDs []string
			if withoutResolve {
				userIDs = users
			} else {
				ids, err := resolveUsers(api, users...)
				if err != nil {
					return fmt.Errorf("user error: %w", err)
				}
				userIDs = ids
			}

			ch, _, _, err := api.OpenConversation(&slack.OpenConversationParameters{
				Users: userIDs,
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
