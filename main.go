package main

import (
	"fmt"
	"io"
	"os"

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

func main() {
	_ = godotenv.Load()

	token := os.Getenv("SLACK_TOKEN")
	channel := os.Getenv("SLACK_CHANNEL")

	if token == "" || channel == "" {
		fmt.Fprintln(os.Stderr, "SLACK_TOKEN / SLACK_CHANNEL required")
		os.Exit(1)
	}

	selected, err := selectAction()
	if err != nil {
		os.Exit(1)
	}

	api := slack.New(token)
	_, _, err = api.PostMessage(
		channel,
		slack.MsgOptionText(selected.emoji, false),
	)

	if err != nil {
		fmt.Fprintln(os.Stderr, "post error:", err)
		os.Exit(1)
	}
}
