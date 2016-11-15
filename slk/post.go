package slk

import (
	"errors"
	"fmt"

	"github.com/nlopes/slack"
)

func (s *Slk) post(e Entity, msg string) error {
	switch e.Type() {
	case TypeUser:
		return s.postIM(e.ID(), msg)
	case TypeChannel:
		return s.postChannel(e.ID(), msg)
	}

	return fmt.Errorf("Can not post message to type %s", e.Type())
}

func (s *Slk) postChannel(ch, msg string) error {
	channel, ok := s.channels[ch]

	if !ok {
		return errors.New("No such channel")
	}

	p := slack.NewPostMessageParameters()
	p.Username = s.username
	p.AsUser = true
	p.LinkNames = 1

	_, _, err := s.r.PostMessage(channel.ID(), msg, p)

	return err
}

func (s *Slk) postIM(name string, msg string) error {
	user, ok := s.users[name]

	if !ok {
		return errors.New("No such user")
	}

	p := slack.NewPostMessageParameters()
	p.Username = s.username
	p.AsUser = true
	p.LinkNames = 1

	_, _, err := s.r.PostMessage(user.ID(), msg, p)

	return err
}
