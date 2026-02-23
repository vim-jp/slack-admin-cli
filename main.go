package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/manifoldco/promptui"
	"github.com/mattn/go-tty"
	"github.com/slack-go/slack"
)

type action struct {
	name        string
	emoji       string
	description string
}

var actions = []action{
	{"勧告CoC", ":admin-activity-advice-by-coc:", "CoCに基づいて対象者に連絡を取り勧告を行った"},
	{"勧告AUP", ":admin-activity-advice-by-aup:", "AUPに基づいて対象者に連絡を取り勧告を行った"},
	{"警告CoC", ":admin-activity-warn-by-coc:", "CoCに基づいて対象者に連絡を取り警告を行った"},
	{"警告AUP", ":admin-activity-warn-by-aup:", "AUPに基づいて対象者に連絡を取り警告を行った"},
	{"削除CoC", ":admin-activity-delete-by-coc:", "CoCに基づいてメッセージを削除した"},
	{"削除AUP", ":admin-activity-delete-by-aup:", "AUPに基づいてメッセージを削除した"},
	{"削除BAN", ":admin-activity-banned:", "退会処理を実施した"},
}

type escInterceptReader struct {
	r   io.ReadCloser
	buf []byte
}

func newEscInterceptReader(r io.ReadCloser) *escInterceptReader {
	return &escInterceptReader{r: r}
}

func (e *escInterceptReader) Read(p []byte) (int, error) {
	if len(e.buf) > 0 {
		n := copy(p, e.buf)
		e.buf = e.buf[n:]
		return n, nil
	}

	n, err := e.r.Read(p)
	if n > 0 && p[0] == 0x1b {
		if n == 1 {
			p[0] = 0x03
		}
	}
	return n, err
}

func (e *escInterceptReader) Close() error {
	return e.r.Close()
}

func selectAction() (action, error) {
	t, err := tty.Open()
	if err != nil {
		return action{}, err
	}
	defer t.Close()

	items := make([]string, len(actions))
	for i, a := range actions {
		items[i] = fmt.Sprintf("%s: %s", a.name, a.description)
	}

	stdin := newEscInterceptReader(t.Input())
	p := promptui.Select{
		Label:    "投稿するアクションを選択",
		HideHelp: true,
		Items:    items,
		Stdin:    stdin,
		Stdout:   t.Output(),
	}

	index, _, err := p.Run()
	if err != nil {
		return action{}, err
	}

	return actions[index], nil
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
	var cursor string
	for {
		params := &slack.GetConversationsParameters{
			Cursor:          cursor,
			Limit:           1000,
			ExcludeArchived: true,
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

func main() {
	_ = godotenv.Load()

	var (
		flagAction  bool
		flagMessage string
		flagChannel string
		flagUser    string
	)
	flag.BoolVar(&flagAction, "a", false, "アクション選択モード")
	flag.StringVar(&flagMessage, "m", "", "送信するメッセージ")
	flag.StringVar(&flagChannel, "c", "", "送信先チャンネル")
	flag.StringVar(&flagUser, "u", "", "送信先ユーザー (DM)")
	flag.Parse()

	token := os.Getenv("SLACK_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "SLACK_TOKEN required")
		os.Exit(1)
	}

	api := slack.New(token)

	switch {
	case flagMessage != "":
		var channelID string
		switch {
		case flagUser != "":
			userID, err := resolveUser(api, flagUser)
			if err != nil {
				fmt.Fprintln(os.Stderr, "user error:", err)
				os.Exit(1)
			}
			ch, _, _, err := api.OpenConversation(&slack.OpenConversationParameters{
				Users: []string{userID},
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, "open conversation error:", err)
				os.Exit(1)
			}
			channelID = ch.ID
		case flagChannel != "":
			var err error
			channelID, err = resolveChannel(api, flagChannel)
			if err != nil {
				fmt.Fprintln(os.Stderr, "channel error:", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintln(os.Stderr, "-c (チャンネル) または -u (ユーザー) を指定してください")
			os.Exit(1)
		}
		_, _, err := api.PostMessage(
			channelID,
			slack.MsgOptionText(flagMessage, false),
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, "post error:", err)
			os.Exit(1)
		}

	case flagAction:
		channelID, err := resolveChannel(api, "admin-activity")
		if err != nil {
			fmt.Fprintln(os.Stderr, "channel error:", err)
			os.Exit(1)
		}
		selected, err := selectAction()
		if err != nil {
			os.Exit(1)
		}
		_, _, err = api.PostMessage(
			channelID,
			slack.MsgOptionText(selected.emoji, false),
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, "post error:", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "使い方: %[1]s -a | %[1]s -m メッセージ -c チャンネル | %[1]s -m メッセージ -u ユーザー\n", os.Args[0])
		os.Exit(1)
	}
}
