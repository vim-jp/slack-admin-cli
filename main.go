package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/manifoldco/promptui"
	"github.com/mattn/go-tty"
	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
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

func newAPI() *slack.Client {
	token := os.Getenv("SLACK_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "SLACK_TOKEN required")
		os.Exit(1)
	}
	return slack.New(token)
}

func main() {
	_ = godotenv.Load()

	cmd := &cli.Command{
		Name:  "slack-admin-cli",
		Usage: "Slack管理用CLIツール",
		Commands: []*cli.Command{
			{
				Name:    "action",
				Aliases: []string{"a"},
				Usage:   "アクション選択モード",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					api := newAPI()
					channelID, err := resolveChannel(api, "admin-activity")
					if err != nil {
						return fmt.Errorf("channel error: %w", err)
					}
					selected, err := selectAction()
					if err != nil {
						return err
					}
					_, _, err = api.PostMessage(
						channelID,
						slack.MsgOptionText(selected.emoji, false),
					)
					if err != nil {
						return fmt.Errorf("post error: %w", err)
					}
					return nil
				},
			},
			{
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
			},
			{
				Name:    "post",
				Aliases: []string{"p"},
				Usage:   "メッセージを送信",
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
						Name:    "user",
						Aliases: []string{"u"},
						Usage:   "送信先ユーザー (DM)",
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
					user := cmd.String("user")
					threadTS := cmd.String("thread")

					if message == "-" {
						b, err := io.ReadAll(os.Stdin)
						if err != nil {
							return fmt.Errorf("stdin error: %w", err)
						}
						message = strings.TrimRight(string(b), "\n")
					}

					var channelID string
					switch {
					case user != "":
						userID, err := resolveUser(api, user)
						if err != nil {
							return fmt.Errorf("user error: %w", err)
						}
						ch, _, _, err := api.OpenConversation(&slack.OpenConversationParameters{
							Users: []string{userID},
						})
						if err != nil {
							return fmt.Errorf("open conversation error: %w", err)
						}
						channelID = ch.ID
					case channel != "":
						var err error
						channelID, err = resolveChannel(api, channel)
						if err != nil {
							return fmt.Errorf("channel error: %w", err)
						}
						// チャンネルに未参加の場合は参加を試みる (スコープ不足の場合は無視)
						api.JoinConversation(channelID)
					default:
						// -t にURLが指定されている場合はチャンネルIDを抽出
						if threadTS != "" {
							if urlCh, ts, ok := parseMessageURL(threadTS); ok {
								channelID = urlCh
								threadTS = ts
							}
						}
						if channelID == "" {
							return fmt.Errorf("-c (チャンネル) または -u (ユーザー) を指定してください")
						}
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
			},
			{
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
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
