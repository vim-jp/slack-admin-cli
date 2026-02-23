package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

func cmdEdit() *cli.Command {
	return &cli.Command{
		Name:    "edit",
		Aliases: []string{"e"},
		Usage:   "投稿を編集",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "target",
				Aliases:  []string{"t"},
				Usage:    "編集対象のメッセージURL",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "message",
				Aliases: []string{"m"},
				Usage:   "新しいメッセージ (- で標準入力から読み込み)",
			},
			&cli.BoolFlag{
				Name:    "editor",
				Aliases: []string{"e"},
				Usage:   "エディタで編集 ($EDITOR, デフォルト vim)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			target := cmd.String("target")
			message := cmd.String("message")
			useEditor := cmd.Bool("editor")

			channelID, ts, ok := parseMessageURL(target)
			if !ok {
				return fmt.Errorf("無効なSlack URL: %s", target)
			}

			// 元のメッセージを取得
			resp, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
				ChannelID: channelID,
				Latest:    ts,
				Oldest:    ts,
				Inclusive: true,
				Limit:     1,
			})
			if err != nil {
				return fmt.Errorf("メッセージ取得エラー: %w", err)
			}
			if len(resp.Messages) == 0 {
				return fmt.Errorf("メッセージが見つかりません")
			}
			originalText := resp.Messages[0].Text

			switch {
			case useEditor:
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vim"
				}
				tmpfile, err := os.CreateTemp("", "slack-edit-*.txt")
				if err != nil {
					return fmt.Errorf("一時ファイル作成エラー: %w", err)
				}
				defer os.Remove(tmpfile.Name())
				if _, err := tmpfile.WriteString(originalText); err != nil {
					tmpfile.Close()
					return fmt.Errorf("一時ファイル書き込みエラー: %w", err)
				}
				tmpfile.Close()

				c := exec.Command(editor, tmpfile.Name())
				c.Stdin = os.Stdin
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				if err := c.Run(); err != nil {
					return fmt.Errorf("エディタエラー: %w", err)
				}

				edited, err := os.ReadFile(tmpfile.Name())
				if err != nil {
					return fmt.Errorf("一時ファイル読み込みエラー: %w", err)
				}
				message = strings.TrimRight(string(edited), "\n")
			case message == "-":
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("stdin error: %w", err)
				}
				message = strings.TrimRight(string(b), "\n")
			case message == "":
				return fmt.Errorf("-m (メッセージ) または -editor を指定してください")
			}

			if message == originalText {
				fmt.Fprintln(os.Stderr, "変更がありません")
				return nil
			}

			_, _, _, err = api.UpdateMessage(
				channelID,
				ts,
				slack.MsgOptionText(message, false),
			)
			if err != nil {
				return fmt.Errorf("更新エラー: %w", err)
			}
			return nil
		},
	}
}
