package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/slack-go/slack"
)

var (
	redisURL      = os.Getenv("REDIS_URL")
	authToken     = os.Getenv("AUTH_TOKEN")
	streamChannel = os.Getenv("STREAM_CHANNEL")

	// Comma separated list of Slack user IDs. Streambot will not join channels craeted by them.
	ignoreChannelsCreatedByUserIds = strings.Split(os.Getenv("IGNORE_CHANNELS_CREATED_BY_USER_IDS"), ",")
)

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

type Config struct {
	db *redis.Client
}

func NewConfig(redisURL string) (Config, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return Config{}, err
	}

	client := redis.NewClient(opts)

	return Config{
		db: client,
	}, nil
}

func (c *Config) UserActive(id string) bool {
	if err := c.db.Get(id).Err(); err != nil {
		if err == redis.Nil {
			return true
		}
	}

	return false
}

func (c *Config) EnableUser(id string) {
	c.db.Del(id)

}

func (c *Config) DisableUser(id string) {
	c.db.Set(id, true, 0)
}

func (c *Config) ChannelActive(id string) bool {
	if err := c.db.Get(id).Err(); err != nil {
		if err == redis.Nil {
			return true
		}
	}

	return false
}

func (c *Config) EnableChannel(id string) {
	c.db.Del(id)
}

func (c *Config) DisableChannel(id string) {
	c.db.Set(id, true, 0)
}

func streamMsgAttachment(api *slack.Client, attachment slack.Attachment) error {
	if attachment.Color == "" {
		attachment.Color = "#0040FF"
	}

	_, _, err := api.PostMessage(streamChannel, slack.MsgOptionAttachments(attachment), slack.MsgOptionAsUser(true))
	if err != nil {
		return err
	}

	return nil
}

func streamMsg(api *slack.Client, rtm *slack.RTM, ev *slack.MessageEvent) {
	user, err := rtm.GetUserInfo(ev.User)
	if err != nil {
		log.Println("Error getting user:", err)
		return
	}

	channelName := ""

	if strings.HasPrefix(ev.Channel, "D") {
		channelName = "In DM with streambot"
	} else {
		channel, err := rtm.GetChannelInfo(ev.Channel)
		if err != nil {
			log.Println("Error getting channel info:", err)
			return
		}

		channelName = "#" + channel.Name
	}

	attachment := slack.Attachment{
		AuthorID:      user.ID,
		AuthorName:    user.Profile.DisplayName,
		AuthorSubname: channelName,
		AuthorIcon:    user.Profile.ImageOriginal,

		Text: ev.Text,
	}

	streamMsgAttachment(api, attachment)
}

func streamTyping(api *slack.Client, rtm *slack.RTM, ev *slack.UserTypingEvent) error {
	user, err := rtm.GetUserInfo(ev.User)
	if err != nil {
		return err
	}

	channelName := ""

	if strings.HasPrefix(ev.Channel, "D") {
		channelName = "In DM with streambot"
	} else {
		fmt.Println(ev.Channel)
		channel, err := rtm.GetChannelInfo(ev.Channel)
		if err != nil {
			log.Println("Error getting channel info:", err)
			return err
		}

		channelName = "#" + channel.Name
	}

	channel, timestamp, err := api.PostMessage(streamChannel, slack.MsgOptionText("_"+user.Name+" is typing in "+channelName+"…_", false), slack.MsgOptionAsUser(true))
	if err != nil {
		return err
	}

	time.Sleep(1 * time.Second)

	if _, _, err := api.DeleteMessage(channel, timestamp); err != nil {
		return err
	}

	return nil
}

func main() {
	// websocket stuff
	hub := newHub()
	go hub.run()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "1337"
	}

	go func() {
		fmt.Println("websocket server listening on :" + port)
		err := http.ListenAndServe(":"+port, nil)
		if err != nil {
			log.Fatal("error with websocket server: ", err)
		}
	}()

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
			if contains(ignoreChannelsCreatedByUserIds, channel.Creator) {
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
			// log message type to ws
			toSendToWs, err := json.Marshal(struct {
				Type      string    `json:"type"`
				Timestamp time.Time `json:"timestamp"`
			}{
				msg.Type,
				time.Now(),
			})
			if err != nil {
				fmt.Println("error encoding json for ws:", err)
			} else {
				fmt.Println("broadcasting msg type to ws...")
				hub.broadcast <- toSendToWs
			}
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
