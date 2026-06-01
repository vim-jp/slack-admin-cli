package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
)

func cmdEmoji() *cli.Command {
	return &cli.Command{
		Name:      "emoji",
		Aliases:   []string{"em"},
		Usage:     "カスタム絵文字の一覧を表示",
		ArgsUsage: "[フィルタ]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "url",
				Aliases: []string{"u"},
				Usage:   "名前とURL (エイリアスの場合は参照先) を表示",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			emojis, err := api.GetEmoji()
			if err != nil {
				return fmt.Errorf("emoji error: %w", err)
			}

			filter := cmd.Args().First()
			showURL := cmd.Bool("url")

			names := make([]string, 0, len(emojis))
			for name := range emojis {
				if filter != "" && !strings.Contains(name, filter) {
					continue
				}
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				if showURL {
					fmt.Printf("%s\t%s\n", name, emojis[name])
				} else {
					fmt.Println(name)
				}
			}
			return nil
		},
	}
}
