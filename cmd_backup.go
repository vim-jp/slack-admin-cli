package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

var (
	userRefPattern    = regexp.MustCompile(`<@(U[A-Z0-9]+)(?:\|[^>]*)?>`)
	channelRefPattern = regexp.MustCompile(`<#(C[A-Z0-9]+)(?:\|([^>]*))?>`)
)

func cmdBackup() *cli.Command {
	return &cli.Command{
		Name:    "backup",
		Aliases: []string{"b"},
		Usage:   "チャンネルをスレッド・添付込みでバックアップ",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "channel",
				Aliases:  []string{"c"},
				Usage:    "バックアップ対象チャンネル",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "output",
				Aliases:  []string{"o"},
				Usage:    "出力先ディレクトリ",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			channel := cmd.String("channel")
			outDir := cmd.String("output")

			channelID, err := resolveChannel(api, channel)
			if err != nil {
				return fmt.Errorf("channel error: %w", err)
			}

			fmt.Fprintln(os.Stderr, "ユーザー一覧を取得中...")
			userMap, err := buildUserMap(api)
			if err != nil {
				return fmt.Errorf("user map error: %w", err)
			}

			fmt.Fprintln(os.Stderr, "チャンネル一覧を取得中...")
			channelMap, err := buildChannelMap(api)
			if err != nil {
				return fmt.Errorf("channel map error: %w", err)
			}

			filesDir := filepath.Join(outDir, "files")
			if err := os.MkdirAll(filesDir, 0o755); err != nil {
				return fmt.Errorf("mkdir error: %w", err)
			}

			channelName := channelMap[channelID]
			if channelName == "" {
				channelName = channelID
			}
			outPath := filepath.Join(outDir, channelName+".md")
			out, err := os.Create(outPath)
			if err != nil {
				return fmt.Errorf("create error: %w", err)
			}
			defer out.Close()

			fmt.Fprintf(out, "# #%s\n\n", channelName)
			fmt.Fprintf(out, "_Channel ID: `%s`_  \n", channelID)
			fmt.Fprintf(out, "_Backup at: %s_\n\n", time.Now().Format(time.RFC3339))
			fmt.Fprintln(out, "---")
			fmt.Fprintln(out)

			fmt.Fprintln(os.Stderr, "メッセージを取得中...")
			messages, err := fetchAllHistory(api, channelID)
			if err != nil {
				return fmt.Errorf("history error: %w", err)
			}

			botToken := os.Getenv("SLACK_BOT_TOKEN")
			total := 0
			for _, msg := range messages {
				// スレッド子メッセージは親で取得するのでスキップ
				if msg.ThreadTimestamp != "" && msg.ThreadTimestamp != msg.Timestamp {
					continue
				}
				writeMessage(out, msg, userMap, channelMap, filesDir, botToken, false)
				total++

				if msg.ReplyCount > 0 {
					replies, err := fetchReplies(api, channelID, msg.Timestamp)
					if err != nil {
						fmt.Fprintf(os.Stderr, "thread error (ts=%s): %v\n", msg.Timestamp, err)
						continue
					}
					for i, r := range replies {
						if i == 0 && r.Timestamp == msg.Timestamp {
							continue
						}
						writeMessage(out, r, userMap, channelMap, filesDir, botToken, true)
						total++
					}
				}
				fmt.Fprintln(out, "---")
				fmt.Fprintln(out)
			}

			fmt.Fprintf(os.Stderr, "完了: %d メッセージを %s に書き出しました\n", total, outPath)
			return nil
		},
	}
}

func buildUserMap(api *slack.Client) (map[string]string, error) {
	users, err := api.GetUsers()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(users))
	for _, u := range users {
		name := u.Profile.DisplayName
		if name == "" {
			name = u.Name
		}
		m[u.ID] = name
	}
	return m, nil
}

func buildChannelMap(api *slack.Client) (map[string]string, error) {
	m := make(map[string]string)
	var cursor string
	for {
		chs, next, err := api.GetConversations(&slack.GetConversationsParameters{
			Cursor: cursor,
			Limit:  1000,
			Types:  []string{"public_channel", "private_channel"},
		})
		if err != nil {
			return nil, err
		}
		for _, c := range chs {
			m[c.ID] = c.Name
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return m, nil
}

func fetchAllHistory(api *slack.Client, channelID string) ([]slack.Message, error) {
	var all []slack.Message
	var cursor string
	for {
		resp, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Cursor:    cursor,
			Limit:     200,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Messages...)
		if !resp.HasMore {
			break
		}
		if resp.ResponseMetaData.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor
	}
	// API は新しい順に返すので、古い順に並べ替え
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

func fetchReplies(api *slack.Client, channelID, ts string) ([]slack.Message, error) {
	var all []slack.Message
	var cursor string
	for {
		ms, hasMore, next, err := api.GetConversationReplies(&slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: ts,
			Cursor:    cursor,
			Limit:     200,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, ms...)
		if !hasMore || next == "" {
			break
		}
		cursor = next
	}
	return all, nil
}

func writeMessage(out io.Writer, msg slack.Message, userMap, channelMap map[string]string, filesDir, botToken string, isReply bool) {
	prefix := ""
	if isReply {
		prefix = "> "
	}

	userName := userMap[msg.User]
	if userName == "" {
		switch {
		case msg.Username != "":
			userName = msg.Username
		case msg.BotID != "":
			userName = "bot:" + msg.BotID
		default:
			userName = msg.User
		}
	}

	writeLine := func(s string) {
		if s == "" {
			fmt.Fprintln(out, strings.TrimRight(prefix, " "))
			return
		}
		fmt.Fprintf(out, "%s%s\n", prefix, s)
	}

	writeLine(fmt.Sprintf("**@%s** · %s", userName, formatTimestamp(msg.Timestamp)))
	writeLine("")

	text := resolveText(msg.Text, userMap, channelMap)
	for _, line := range strings.Split(text, "\n") {
		writeLine(line)
	}

	for _, f := range msg.Files {
		saved := saveFile(f, filesDir, botToken)
		writeLine("")
		if saved == "" {
			writeLine(fmt.Sprintf("📎 添付(取得失敗): %s", f.Name))
			continue
		}
		path := "files/" + saved
		if strings.HasPrefix(f.Mimetype, "image/") {
			writeLine(fmt.Sprintf("![%s](%s)", f.Name, path))
		} else {
			writeLine(fmt.Sprintf("📎 [%s](%s) (%s, %d bytes)", f.Name, path, f.Mimetype, f.Size))
		}
	}

	if len(msg.Reactions) > 0 {
		var parts []string
		for _, r := range msg.Reactions {
			parts = append(parts, fmt.Sprintf(":%s: ×%d", r.Name, r.Count))
		}
		writeLine("")
		writeLine(fmt.Sprintf("**リアクション:** %s", strings.Join(parts, " ")))
	}
	writeLine("")
}

func resolveText(text string, userMap, channelMap map[string]string) string {
	text = userRefPattern.ReplaceAllStringFunc(text, func(m string) string {
		sub := userRefPattern.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		if name, ok := userMap[sub[1]]; ok {
			return "@" + name
		}
		return m
	})
	text = channelRefPattern.ReplaceAllStringFunc(text, func(m string) string {
		sub := channelRefPattern.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		if sub[2] != "" {
			return "#" + sub[2]
		}
		if name, ok := channelMap[sub[1]]; ok {
			return "#" + name
		}
		return m
	})
	return text
}

func formatTimestamp(ts string) string {
	parts := strings.SplitN(ts, ".", 2)
	if len(parts) == 0 {
		return ts
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts
	}
	return time.Unix(sec, 0).Format("2006-01-02 15:04:05")
}

func saveFile(f slack.File, dir, token string) string {
	url := f.URLPrivateDownload
	if url == "" {
		url = f.URLPrivate
	}
	if url == "" {
		return ""
	}

	safe := sanitizeFilename(f.Name)
	if safe == "" {
		safe = "file"
	}
	outPath := filepath.Join(dir, f.ID+"_"+safe)

	if _, err := os.Stat(outPath); err == nil {
		return filepath.Base(outPath)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	w, err := os.Create(outPath)
	if err != nil {
		return ""
	}
	defer w.Close()
	if _, err := io.Copy(w, resp.Body); err != nil {
		os.Remove(outPath)
		return ""
	}
	return filepath.Base(outPath)
}

func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		return r
	}, name)
}
