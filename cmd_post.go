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

func cmdPost() *cli.Command {
	return &cli.Command{
		Name:    "post",
		Aliases: []string{"p"},
		Usage:   "チャンネルにメッセージを送信",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "message",
				Aliases:  []string{"m"},
				Usage:    "送信するメッセージ (- で標準入力から読み込み)",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "channel",
				Aliases: []string{"c"},
				Usage:   "送信先チャンネル",
			},
			&cli.StringFlag{
				Name:    "thread",
				Aliases: []string{"t"},
				Usage:   "スレッドのタイムスタンプ (thread_ts) またはSlack URL",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			message := cmd.String("message")
			channel := cmd.String("channel")
			threadTS := cmd.String("thread")

			if message == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("stdin error: %w", err)
				}
				message = strings.TrimRight(string(b), "\n")
			}

			var channelID string
			if channel != "" {
				var err error
				channelID, err = resolveChannel(api, channel)
				if err != nil {
					return fmt.Errorf("channel error: %w", err)
				}
				// チャンネルに未参加の場合は参加を試みる (スコープ不足の場合は無視)
				api.JoinConversation(channelID)
			} else if threadTS != "" {
				if urlCh, ts, ok := parseMessageURL(threadTS); ok {
					channelID = urlCh
					threadTS = ts
				}
			}
			if channelID == "" {
				return fmt.Errorf("-c (チャンネル) または -t (Slack URL) を指定してください")
			}

			opts := []slack.MsgOption{
				slack.MsgOptionText(message, false),
			}
			if threadTS != "" {
				if urlCh, ts, ok := parseMessageURL(threadTS); ok {
					threadTS = ts
					if channelID == "" {
						channelID = urlCh
					}
				}
				opts = append(opts, slack.MsgOptionTS(threadTS))
			}
			_, _, err := api.PostMessage(channelID, opts...)
			if err != nil {
				return fmt.Errorf("post error: %w", err)
			}
			return nil
		},
	}
}
