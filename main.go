package main

import (
	"flag"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
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

var slackURLPattern = regexp.MustCompile(`/archives/([^/]+)/p(\d{10})(\d{6})`)

// parseMessageURL parses a Slack message URL and returns channelID and thread_ts.
func parseMessageURL(url string) (channelID, threadTS string, ok bool) {
	m := slackURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2] + "." + m[3], true
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
		flagAction   bool
		flagList     bool
		flagMessage  string
		flagChannel  string
		flagUser     string
		flagThreadTS string
	)
	flag.BoolVar(&flagAction, "a", false, "アクション選択モード")
	flag.BoolVar(&flagList, "l", false, "メンバ一覧をCSVでエクスポート")
	flag.StringVar(&flagMessage, "m", "", "送信するメッセージ")
	flag.StringVar(&flagChannel, "c", "", "送信先チャンネル")
	flag.StringVar(&flagUser, "u", "", "送信先ユーザー (DM)")
	flag.StringVar(&flagThreadTS, "t", "", "スレッドのタイムスタンプ (thread_ts)")
	flag.Parse()

	token := os.Getenv("SLACK_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "SLACK_TOKEN required")
		os.Exit(1)
	}

	api := slack.New(token)

	if flagMessage == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "stdin error:", err)
			os.Exit(1)
		}
		flagMessage = strings.TrimRight(string(b), "\n")
	}

	switch {
	case flagList:
		users, err := api.GetUsers()
		if err != nil {
			fmt.Fprintln(os.Stderr, "users error:", err)
			os.Exit(1)
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
			// チャンネルに未参加の場合は参加を試みる (スコープ不足の場合は無視)
			api.JoinConversation(channelID)
		default:
			// -t にURLが指定されている場合はチャンネルIDを抽出
			if flagThreadTS != "" {
				if urlCh, ts, ok := parseMessageURL(flagThreadTS); ok {
					channelID = urlCh
					flagThreadTS = ts
				}
			}
			if channelID == "" {
				fmt.Fprintln(os.Stderr, "-c (チャンネル) または -u (ユーザー) を指定してください")
				os.Exit(1)
			}
		}
		opts := []slack.MsgOption{
			slack.MsgOptionText(flagMessage, false),
		}
		if flagThreadTS != "" {
			if urlCh, ts, ok := parseMessageURL(flagThreadTS); ok {
				flagThreadTS = ts
				if channelID == "" {
					channelID = urlCh
				}
			}
			opts = append(opts, slack.MsgOptionTS(flagThreadTS))
		}
		_, _, err := api.PostMessage(channelID, opts...)
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
		fmt.Fprintf(os.Stderr, "使い方: %[1]s -a | %[1]s -l | %[1]s -m メッセージ -c チャンネル | %[1]s -m メッセージ -u ユーザー\n", os.Args[0])
		os.Exit(1)
	}
}
