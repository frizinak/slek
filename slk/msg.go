package slk

import (
	"fmt"

	"github.com/nlopes/slack"
)

func (s *Slk) reaction(r interface{}) {
	var userID string
	var channelID string
	var sign string
	var item string
	var timestamp string

	switch reaction := r.(type) {
	case *slack.ReactionAddedEvent:
		sign = "[+]"
		userID = reaction.User
		channelID = reaction.Item.Channel
		item = reaction.Reaction
		timestamp = reaction.Item.Timestamp
	case *slack.ReactionRemovedEvent:
		sign = "[-]"
		userID = reaction.User
		channelID = reaction.Item.Channel
		item = reaction.Reaction
		timestamp = reaction.Item.Timestamp
	default:
		s.out.Warn("Not a reaction event")
		return
	}

	var entity Entity
	channel := s.channel(channelID)
	entity = channel

	if channel.IsNil() || !channel.isMember {
		entity = s.user(s.im(channelID).User)
		if entity.IsNil() {
			return
		}
	}

	s.msg(
		&slack.Message{
			Msg: slack.Msg{
				Channel:   channelID,
				User:      userID,
				Text:      fmt.Sprintf("%s %s", sign, item),
				Timestamp: timestamp,
			},
		},
		false,
		entity.Type() == TypeUser,
		false,
	)
}

func (s *Slk) msg(m *slack.Message, newSection, notify, isNew bool) {
	if m.Hidden {
		// TODO we sure 'bout that?
		return
	}

	if m.User == "" && m.SubMessage != nil {
		m.Msg = *m.SubMessage
	}

	var entity Entity
	ch := s.channel(m.Channel)
	entity = ch

	im := false
	if ch.IsNil() {
		user := s.user(s.im(m.Channel).User)
		im = !user.IsNil()
		entity = user
	}

	if s.active == nil {
		s.Switch(entity)
	}

	active := entity.Is(s.active)
	if isNew {
		entity.incrementUnread()
		entity.setLatest(m.Timestamp)
		if active {
			s.markRead <- entity
		}
	}

	username := s.user(m.User).Name()
	if m.User == "" && m.Username != "" {
		username = m.Username
	}

	text, mentions := s.parseTextIncoming(
		append([]string{m.Text},
			s.parseAttachments(m.Attachments)...)...,
	)

	if notify && username != s.username {
		if im {
			if username != s.username {
				s.out.Notify(entity.QualifiedName(), username, text, false)
			}
		} else {
			for i := range mentions {
				if mentions[i] == s.username {
					s.out.Notify(
						entity.QualifiedName(),
						username,
						text,
						false,
					)
				}
			}
		}
	}

	if !active {
		return
	}

	s.out.Msg(
		entity.QualifiedName(),
		username,
		text,
		ts(m.Timestamp),
		newSection,
	)
}
