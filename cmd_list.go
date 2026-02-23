package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func cmdList() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Aliases: []string{"l"},
		Usage:   "メンバ一覧をCSVでエクスポート",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			users, err := api.GetUsers()
			if err != nil {
				return fmt.Errorf("users error: %w", err)
			}
			w := csv.NewWriter(os.Stdout)
			w.Write([]string{"id", "name", "real_name", "display_name", "email", "deleted", "is_bot"})
			for _, u := range users {
				if u.ID == "USLACKBOT" {
					continue
				}
				deleted := "false"
				if u.Deleted {
					deleted = "true"
				}
				isBot := "false"
				if u.IsBot {
					isBot = "true"
				}
				w.Write([]string{
					u.ID,
					u.Name,
					u.RealName,
					u.Profile.DisplayName,
					u.Profile.Email,
					deleted,
					isBot,
				})
			}
			w.Flush()
			return nil
		},
	}
}
