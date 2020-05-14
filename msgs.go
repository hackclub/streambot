package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

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

		channelName = "<#" + channel.Conversation.ID + ">"
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
