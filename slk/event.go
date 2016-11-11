package slk

import (
	"fmt"
	"reflect"
	"time"

	"github.com/nlopes/slack"
)

func (s *Slk) handleEvent(event slack.RTMEvent) error {
	switch d := event.Data.(type) {

	case *slack.GroupCreatedEvent:
		s.updateChannels(nil, nil)
	case *slack.GroupArchiveEvent:
		s.updateChannels(nil, nil)
	case *slack.GroupUnarchiveEvent:
		s.updateChannels(nil, nil)

	case *slack.ChannelCreatedEvent:
		s.updateChannels(nil, nil)
	case *slack.ChannelArchiveEvent:
		s.updateChannels(nil, nil)
	case *slack.ChannelUnarchiveEvent:
		s.updateChannels(nil, nil)
	case *slack.ChannelDeletedEvent:
		s.updateChannels(nil, nil)

	case *slack.TeamJoinEvent:
		s.updateUsers(nil)
		s.updateIMs(nil)

	case *slack.IMCreatedEvent:
		s.updateIMs(nil)

	case *slack.PresenceChangeEvent:
		s.user(d.User).Presence = d.Presence
		// TODO notice or something

	case *slack.ChannelJoinedEvent:
		channel := s.channel(d.Channel.ID)
		channel.isMember = true
	case *slack.ChannelLeftEvent:
		channel := s.channel(d.Channel)
		channel.isMember = false

	case *slack.GroupJoinedEvent:
		channel := s.channel(d.Channel.ID)
		channel.isMember = true
	case *slack.GroupLeftEvent:
		channel := s.channel(d.Channel)
		channel.isMember = false

	case *slack.UserTypingEvent:
		channel := s.channel(d.Channel)
		channelName := channel.Name()
		if channel.IsNil() {
			channelName = "Unknown channel / user"
			user := s.user(s.im(d.Channel).User)
			if !user.IsNil() {
				channelName = "IM"
			}
		}

		s.out.Typing(
			channelName,
			s.user(d.User).Name(),
			time.Second*4,
		)
	case *slack.MessageEvent:
		switch d.SubType {
		case "channel_join":
			fallthrough
		case "group_join":
			ch := s.channel(d.Channel)
			ch.members = append(ch.members, d.User)

		case "channel_leave":
			fallthrough
		case "group_leave":
			ch := s.channel(d.Channel)
			for i := range ch.members {
				if ch.members[i] == d.User {
					ch.members = append(ch.members[:i], ch.members[i+1:]...)
					break
				}
			}
		}

		m := slack.Message(*d)
		s.msg(&m, false, true, true)

	case *slack.ReactionAddedEvent:
		s.reaction(d)

	case *slack.ReactionRemovedEvent:
		s.reaction(d)

	case *slack.FileSharedEvent:
		// TODO ignorable? slack.MessageEvent seems to suffice
		s.out.Debug(d.Type, fmt.Sprintf("%+v", d.File))

		if d.File.URLPrivate == "" &&
			d.File.URLPrivateDownload == "" &&
			d.File.PermalinkPublic == "" &&
			d.File.Permalink == "" {
			return nil
		}

		ch := nilChan
		if len(d.File.Channels) != 0 {
			ch = s.channel(d.File.Channels[0])
		}

		url := d.File.URLPrivate
		if d.File.PermalinkPublic != "" {
			url = d.File.PermalinkPublic
		}

		s.out.File(
			ch.Name(),
			s.user(d.File.User).Name(),
			fmt.Sprintf("%s %s", d.File.Title, d.File.Name),
			url,
		)

	case *slack.PrefChangeEvent:
		switch d.Name {
		case "emoji_use":
			return nil
		}
		// TODO lookup which preferences are relevant to slek.
		s.out.Debug(event.Type, d.Name, string(d.Value))

	case *slack.DisconnectedEvent:
		if !d.Intentional {
			s.out.Warn("Disconnected! Reconnecting...")
		}
	case *slack.ConnectingEvent:
		s.out.Notice("Connecting...")
	case *slack.ConnectedEvent:
		s.out.Notice("Connected!")
	case *slack.HelloEvent:
		s.out.Notice("Slack: hello!")

	case *slack.FilePublicEvent:
		// TODO ignorable? slack.MessageEvent seems to suffice
		s.out.Debug(d.Type, fmt.Sprintf("%+v", d.File))
		// Ignore

	// Ignores
	case *slack.ChannelMarkedEvent:
	case *slack.IMMarkedEvent:
	case *slack.LatencyReport:
	case *slack.ReconnectUrlEvent:
	default:
		// TODO
		s.out.Debug(event.Type, reflect.TypeOf(event.Data).String())
	}

	return nil
}
