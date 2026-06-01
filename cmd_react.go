package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

func cmdReact() *cli.Command {
	return &cli.Command{
		Name:    "react",
		Aliases: []string{"r"},
		Usage:   "メッセージにリアクションを付ける",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "target",
				Aliases:  []string{"t"},
				Usage:    "対象のメッセージURL",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "emoji",
				Aliases:  []string{"e"},
				Usage:    "リアクションの絵文字名 (例: +1 または :+1:)",
				Required: true,
			},
			&cli.BoolFlag{
				Name:    "remove",
				Aliases: []string{"d"},
				Usage:   "リアクションを削除する",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			target := cmd.String("target")
			emoji := strings.Trim(cmd.String("emoji"), ":")
			remove := cmd.Bool("remove")

			channelID, ts, ok := parseMessageURL(target)
			if !ok {
				return fmt.Errorf("無効なSlack URL: %s", target)
			}

			item := slack.NewRefToMessage(channelID, ts)
			if remove {
				if err := api.RemoveReaction(emoji, item); err != nil {
					return fmt.Errorf("リアクション削除エラー: %w", err)
				}
				return nil
			}
			if err := api.AddReaction(emoji, item); err != nil {
				return fmt.Errorf("リアクション追加エラー: %w", err)
			}
			return nil
		},
	}
}
