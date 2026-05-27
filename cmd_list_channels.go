package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

func cmdListChannels() *cli.Command {
	return &cli.Command{
		Name:    "list-channels",
		Aliases: []string{"lc"},
		Usage:   "チャンネル一覧をCSVでエクスポート (public + private)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "include-archived",
				Usage: "アーカイブ済みチャンネルも含める",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			includeArchived := cmd.Bool("include-archived")

			w := csv.NewWriter(os.Stdout)
			w.Write([]string{"id", "name", "is_private", "is_archived", "is_member", "num_members", "topic", "purpose"})

			var cursor string
			for {
				chs, next, err := api.GetConversations(&slack.GetConversationsParameters{
					Cursor:          cursor,
					Limit:           1000,
					ExcludeArchived: !includeArchived,
					Types:           []string{"public_channel", "private_channel"},
				})
				if err != nil {
					return fmt.Errorf("conversations error: %w", err)
				}
				for _, c := range chs {
					w.Write([]string{
						c.ID,
						c.Name,
						strconv.FormatBool(c.IsPrivate),
						strconv.FormatBool(c.IsArchived),
						strconv.FormatBool(c.IsMember),
						strconv.Itoa(c.NumMembers),
						c.Topic.Value,
						c.Purpose.Value,
					})
				}
				if next == "" {
					break
				}
				cursor = next
			}
			w.Flush()
			return nil
		},
	}
}
