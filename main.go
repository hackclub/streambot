package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/zachlatta/streambot/util"
	"github.com/zachlatta/streambot/ws"
)

var (
	redisURL                       string
	authToken                      string
	streamChannel                  string
	ignoreChannelsCreatedByUserIds []string
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	redisURL = os.Getenv("REDIS_URL")
	authToken = os.Getenv("AUTH_TOKEN")
	streamChannel = os.Getenv("STREAM_CHANNEL")

	// Comma separated list of Slack user IDs. Streambot will not join channels created by them.
	ignoreChannelsCreatedByUserIds = strings.Split(os.Getenv("IGNORE_CHANNELS_CREATED_BY_USER_IDS"), ",")

	// websocket stuff
	wsPort := os.Getenv("PORT")
	if wsPort == "" {
		wsPort = "1337"
	}

	server := ws.NewServer(wsPort)

	go server.Serve()

	// streambot (slack stuff)
	config, err := NewConfig(redisURL)
	if err != nil {
		log.Fatal(err)
	}

	api := slack.New(authToken)

	rtm := api.NewRTM()
	go rtm.ManageConnection()

	go func() {
		channels, _ := rtm.GetChannels(true)
		for _, channel := range channels {
			if util.Contains(ignoreChannelsCreatedByUserIds, channel.Creator) {
				continue
			}

			fmt.Println("joining", channel.Name)

			rtm.JoinChannel(channel.Name)

			time.Sleep((5 * time.Second))

			fmt.Println(channel.ID)
		}
	}()

	for msg := range rtm.IncomingEvents {
		fmt.Println("Event Received:", msg)

		go func() {
			toSend := ws.NewActivity(msg.Type, "")

			switch ev := msg.Data.(type) {
			case *slack.MessageEvent:
				if ev.User == "USLACKBOT" || ev.User == "" || ev.Channel == streamChannel {
					return
				}

				// Ignore messages if not in a public channel
				if !strings.HasPrefix(ev.Channel, "C") {
					fmt.Println(ev.Channel, "ignoring because not public")
					return
				}

				channel, err := rtm.GetChannelInfo(ev.Channel)
				if err != nil {
					log.Println("Error getting channel info:", err)
					return
				}

				toSend.ChannelName = "#" + channel.Name
			case *slack.UserTypingEvent:
				if ev.User == "USLACKBOT" || ev.User == "" || ev.Channel == streamChannel {
					return
				}

				// Ignore messages if not in a public channel
				if !strings.HasPrefix(ev.Channel, "C") {
					fmt.Println(ev.Channel, "ignoring because not public")
					return
				}

				channel, err := rtm.GetChannelInfo(ev.Channel)
				if err != nil {
					log.Println("Error getting channel info:", err)
					return
				}

				toSend.ChannelName = "#" + channel.Name
			}

			// log message type to ws
			server.Broadcast(toSend)
		}()

		switch ev := msg.Data.(type) {
		case *slack.MessageEvent:
			if ev.User == "USLACKBOT" || ev.User == "" || ev.Channel == streamChannel {
				continue
			}

			// Ignore messages if not in a public channel
			if !strings.HasPrefix(ev.Channel, "C") {
				fmt.Println(ev.Channel, "ignoring because not public")
				continue
			}

			if strings.HasSuffix(ev.Text, "status me") {
				status := config.UserActive(ev.User)
				msg := ""

				if status {
					msg = "i am streaming your messages"
				} else {
					msg = "i am ignoring your messages"
				}

				rtm.SendMessage(rtm.NewOutgoingMessage(msg, ev.Channel))
				continue
			}

			if strings.HasSuffix(ev.Text, "disable me") {
				config.DisableUser(ev.User)

				rtm.SendMessage(rtm.NewOutgoingMessage("i will now ignore your messages", ev.Channel))
				continue
			}

			if strings.HasSuffix(ev.Text, "enable me") {
				config.EnableUser(ev.User)

				rtm.SendMessage(rtm.NewOutgoingMessage("i will now stream your messages", ev.Channel))
				continue
			}

			if strings.HasSuffix(ev.Text, "status channel") {
				status := config.ChannelActive(ev.Channel)
				msg := ""

				if status {
					msg = "i am streaming this channel's messages"
				} else {
					msg = "i am ignoring this channel's messages"
				}

				rtm.SendMessage(rtm.NewOutgoingMessage(msg, ev.Channel))
				continue
			}

			if strings.HasSuffix(ev.Text, "disable channel") {
				config.DisableChannel(ev.Channel)

				rtm.SendMessage(rtm.NewOutgoingMessage("i will now ignore this channel's messages", ev.Channel))
				continue
			}

			if strings.HasSuffix(ev.Text, "enable channel") {
				config.EnableChannel(ev.Channel)

				rtm.SendMessage(rtm.NewOutgoingMessage("i will now stream this channel's messages", ev.Channel))
				continue
			}

			if !config.ChannelActive(ev.Channel) {
				fmt.Println("ignoring message because", ev.Channel, "is set to ignore")
				continue
			}

			if !config.UserActive(ev.User) {
				fmt.Println("ignoring message because", ev.User, "is set to ignore")
				continue
			}

			fmt.Println(ev.Text)

			go streamMsg(api, rtm, ev)
		case *slack.UserTypingEvent:
			if !config.ChannelActive(ev.Channel) {
				fmt.Println("ignoring typing because", ev.Channel, "is set to ignore")
				continue
			}

			if !config.UserActive(ev.User) {
				fmt.Println("ignoring typing because", ev.User, "is set to ignore")
				continue
			}

			go streamTyping(api, rtm, ev)
		case *slack.MemberJoinedChannelEvent:
			if ev.Channel == streamChannel {
				attachment := slack.Attachment{
					Color:    "#0040FF",
					ImageURL: "https://i.imgur.com/4m3Rra5.gif",
				}

				_, err := api.PostEphemeral(streamChannel, ev.User, slack.MsgOptionAttachments(attachment), slack.MsgOptionAsUser(true))
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					continue
				}
			}

		case *slack.ChannelJoinedEvent:
			fmt.Println(ev.Channel)
			fmt.Println(ev.Type)
			rtm.SendMessage(rtm.NewOutgoingMessage(
				`:wave: hi! i'm a bot built by <@zrl> that streams channel activity to <#`+streamChannel+`> so people can easily discover new channels.

don't want your channel (or your account) to be part of this? that's ok! just type `+"`"+`<@streambot> disable me`+"`"+` to have me ignore all of your messages or `+"`"+`<@streambot> disable channel`+"`"+` to have me ignore this whole channel.

if you want to re-enable streaming, you can type `+"`"+`<@streambot> enable me`+"`"+` or `+"`"+`<@streambot> enable channel`+"`"+` and if you want to check whether i'm streaming, you can type `+"`"+`<@streambot> status me`+"`"+` or `+"`"+`<@streambot> status channel`+"`"+`.

i'll never stream private messages, group chats, or private channels. message <@zrl> if you have any questions. happy hacking!`, ev.Channel.ID))
		case *slack.RTMError:
			fmt.Fprintln(os.Stderr, "Error:", ev.Error())
		}
	}
}
