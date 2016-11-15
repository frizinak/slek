package slk

import "github.com/nlopes/slack"

func (s *Slk) updateUsers(users []slack.User) error {
	if users == nil {
		var err error
		users, err = s.c.GetUsers()
		if err != nil {
			return err
		}
	}

	_users := make(map[string]*user, len(users))
	usersByName := make(map[string]*user, len(users))

	for i := range users {
		u := slackUserToUser(&users[i], s.user(users[i].ID))
		_users[users[i].ID] = u
		usersByName[users[i].Name] = u
	}

	s.users = _users
	s.usersByName = usersByName

	return nil
}

func (s *Slk) updateChannels(
	channels []slack.Channel,
	groups []slack.Group,
) error {
	var err error

	if channels == nil {
		channels, err = s.c.GetChannels(true)
		if err != nil {
			return err
		}
	}

	if groups == nil {
		groups, err = s.c.GetGroups(true)
		if err != nil {
			return err
		}
	}

	_channels := make(map[string]*channel, len(channels))
	channelsByName := make(map[string]*channel, len(channels))
	for i := range channels {
		_channels[channels[i].ID] = slackChannelToChannel(
			&channels[i],
			s.channel(channels[i].ID),
		)
	}

	for i := range groups {
		_channels[groups[i].ID] = slackGroupToChannel(
			&groups[i],
			s.channel(groups[i].ID),
		)
	}

	for i := range _channels {
		channelsByName[_channels[i].Name()] = _channels[i]
	}

	s.channels = _channels
	s.channelsByName = channelsByName

	return nil
}

func (s *Slk) updateIMs(ims []slack.IM) error {
	if ims == nil {
		var err error
		ims, err = s.c.GetIMChannels()
		if err != nil {
			return err
		}
	}

	_ims := make(map[string]*slack.IM, len(ims))
	_imsByUser := make(map[string]*slack.IM, len(ims))
	for i := range ims {
		_ims[ims[i].ID] = &ims[i]
		_imsByUser[ims[i].User] = &ims[i]
		u := s.user(ims[i].User)
		if ims[i].LastRead != "" {
			u.lastReadTs = ims[i].LastRead
			u.unread = ims[i].UnreadCount
		}
		if ims[i].Latest != nil && ims[i].Latest.Timestamp != "" {
			u.latestTs = ims[i].Latest.Timestamp
		}
	}

	s.ims = _ims
	s.imsByUser = _imsByUser

	return nil
}

func (s *Slk) user(id string) *user {
	if u, ok := s.users[id]; ok {
		return u
	}

	return nilUser
}

func (s *Slk) userByName(name string) *user {
	if u, ok := s.usersByName[name]; ok {
		return u
	}

	return nilUser
}

func (s *Slk) channel(id string) *channel {
	if ch, ok := s.channels[id]; ok {
		return ch
	}

	return nilChan
}

func (s *Slk) channelByName(name string) *channel {
	if ch, ok := s.channelsByName[name]; ok {
		return ch
	}

	return nilChan
}

func (s *Slk) im(id string) *slack.IM {
	if im, ok := s.ims[id]; ok {
		return im
	}

	return nilIM
}

func (s *Slk) imByUser(id string) *slack.IM {
	if im, ok := s.imsByUser[id]; ok {
		return im
	}

	return nilIM
}
