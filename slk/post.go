package slk

import (
	"errors"
	"fmt"

	"github.com/nlopes/slack"
)

func (s *Slk) post(e Entity, msg string) error {
	switch e.GetType() {
	case TypeUser:
		return s.postIM(e.GetID(), msg)
	case TypeChannel:
		return s.postChannel(e.GetID(), msg)
	}

	return fmt.Errorf("Can not post message to type %s", e.GetType())
}

func (s *Slk) postChannel(ch, msg string) error {
	s.RLock()
	channel, ok := s.channels[ch]
	s.RUnlock()

	if !ok {
		return errors.New("No such channel")
	}

	p := slack.NewPostMessageParameters()
	p.Username = s.username
	p.AsUser = true

	_, _, err := s.r.PostMessage(channel.GetID(), msg, p)

	return err
}

func (s *Slk) postIM(name string, msg string) error {
	s.RLock()
	user, ok := s.users[name]
	s.RUnlock()

	if !ok {
		return errors.New("No such user")
	}

	p := slack.NewPostMessageParameters()
	p.Username = s.username
	p.AsUser = true

	_, _, err := s.r.PostMessage(user.ID, msg, p)

	return err
}
