package slk

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/nlopes/slack"
)

func (s *Slk) history(
	e Entity,
	p slack.HistoryParameters,
	newSection bool,
) (latest string, done bool, err error) {
	var hist *slack.History

	switch e.Type() {
	case TypeChannel:
		if e.(*channel).isChannel {
			hist, err = s.channelHistory(e.ID(), p)
			break
		}

		hist, err = s.groupHistory(e.ID(), p)
	case TypeUser:
		hist, err = s.imHistory(e.ID(), p)
	default:
		err = fmt.Errorf("Can not post message to type %s", e.Type())
	}

	if err != nil {
		return
	}

	l := 0.0
	first := true
	for i := len(hist.Messages) - 1; i >= 0; i-- {
		if hist.Messages[i].Channel == "" {
			hist.Messages[i].Channel = e.ID()
			if e.Type() == TypeUser {
				hist.Messages[i].Channel = s.imByUser(e.ID()).ID
			}
		}

		s.msg(&hist.Messages[i], newSection && first, false, false)
		first = false

		ts, _ := strconv.ParseFloat(hist.Messages[i].Timestamp, 64)
		if ts > l {
			l = ts
			latest = hist.Messages[i].Timestamp
		}
	}

	// Slack be weird, history of an IM has no hist.Latest value.
	if hist.Latest == "" && hist.HasMore {
		return
	}

	histLatest, _ := strconv.ParseFloat(hist.Latest, 64)
	if latest == hist.Latest || l >= histLatest {
		done = true
	}

	return
}

func (s *Slk) imHistory(
	user string,
	p slack.HistoryParameters,
) (*slack.History, error) {
	im := s.imByUser(user)

	if im == nilIM {
		return nil, errors.New("No such user...")
	}

	return s.c.GetIMHistory(im.ID, p)
}

func (s *Slk) channelHistory(
	ch string,
	p slack.HistoryParameters,
) (*slack.History, error) {
	channel := s.channel(ch)

	if channel.IsNil() {
		return nil, errors.New("No such channel...")
	}

	return s.c.GetChannelHistory(channel.ID(), p)
}

func (s *Slk) groupHistory(
	ch string,
	p slack.HistoryParameters,
) (*slack.History, error) {
	channel := s.channel(ch)

	if channel.IsNil() {
		return nil, errors.New("No such channel...")
	}

	return s.c.GetGroupHistory(channel.ID(), p)
}
