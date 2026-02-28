package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/urfave/cli/v3"
)

func cmdBot() *cli.Command {
	return &cli.Command{
		Name:  "bot",
		Usage: "BotモードでDM転送とメンション反応を行う",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "forward-channel",
				Usage: "DMを転送するチャンネル名",
				Value: "#administrator",
			},
			&cli.StringFlag{
				Name:  "reaction",
				Usage: "メンションに付けるリアクション名",
				Value: "eyes",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			api := newAPI()
			forwardChannel := cmd.String("forward-channel")
			reaction := strings.Trim(cmd.String("reaction"), ":")

			auth, err := api.AuthTest()
			if err != nil {
				return fmt.Errorf("auth error: %w", err)
			}
			botUserID := auth.UserID

			forwardChannelID, err := resolveChannel(api, forwardChannel)
			if err != nil {
				return fmt.Errorf("channel error: %w", err)
			}
			api.JoinConversation(forwardChannelID)

			sm := socketmode.New(api)
			go func() {
				if err := sm.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "socketmode error: %v\n", err)
				}
			}()

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sig)

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-sig:
					return nil
				case evt := <-sm.Events:
					switch evt.Type {
					case socketmode.EventTypeConnectionError:
						fmt.Fprintf(os.Stderr, "connection error: %+v\n", evt.Data)
					case socketmode.EventTypeEventsAPI:
						eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
						if !ok {
							continue
						}
						sm.Ack(*evt.Request)
						if eventsAPIEvent.Type != slackevents.CallbackEvent {
							continue
						}
						switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
						case *slackevents.MessageEvent:
							if ev.User == "" || ev.User == botUserID || ev.BotID != "" || ev.SubType != "" {
								continue
							}
							if strings.HasPrefix(ev.Channel, "D") {
								text := fmt.Sprintf("DM from <@%s>:\n%s", ev.User, ev.Text)
								if _, _, err := api.PostMessage(forwardChannelID, slack.MsgOptionText(text, false)); err != nil {
									fmt.Fprintf(os.Stderr, "forward error: %v\n", err)
								}
							}
							if reaction != "" && strings.Contains(ev.Text, "<@"+botUserID+">") {
								if err := api.AddReaction(reaction, slack.ItemRef{
									Channel:   ev.Channel,
									Timestamp: ev.TimeStamp,
								}); err != nil && !errorsIsAlreadyReacted(err) {
									fmt.Fprintf(os.Stderr, "reaction error: %v\n", err)
								}
							}
						}
					}
				}
			}
		},
	}
}

func errorsIsAlreadyReacted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already_reacted")
}
