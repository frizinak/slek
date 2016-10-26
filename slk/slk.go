package slk

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nlopes/slack"
)

type Slk struct {
	out      Output
	username string
	*sync.RWMutex
	token          string
	c              *slack.Client
	r              *slack.RTM
	users          map[string]*user
	usersByName    map[string]*user
	channels       map[string]*channel
	channelsByName map[string]*channel
	ims            map[string]*slack.IM
	imsByUser      map[string]*slack.IM
}

func (s *Slk) Init() error {
	if err := s.updateIMs(); err != nil {
		return err
	}

	if err := s.updateUsers(); err != nil {
		return err
	}

	if err := s.updateChannels(); err != nil {
		return err
	}

	if err := s.updateUsersCounts(); err != nil {
		return err
	}

	return nil
}

func NewSlk(username, token string, output Output) *Slk {
	var rw sync.RWMutex

	return &Slk{
		output,
		username,
		&rw,
		token,
		slack.New(token),
		nil,
		map[string]*user{},
		map[string]*user{},
		map[string]*channel{},
		map[string]*channel{},
		map[string]*slack.IM{},
		map[string]*slack.IM{},
	}
}

func (s *Slk) Invite(channel, user Entity) error {
	if err := s.invite(channel, user); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Invited %s to %s", user.GetName(), channel.GetName()))
	return nil
}

func (s *Slk) Join(e Entity) error {
	if err := s.join(e); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Joined %s", e.GetName()))
	return nil
}

func (s *Slk) Leave(e Entity) error {
	if err := s.leave(e); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Left %s", e.GetName()))
	return nil
}

func (s *Slk) Joined() []Entity {
	joined := make([]Entity, 0)
	for i := range s.channels {
		if s.channels[i].IsActive() {
			joined = append(joined, s.channels[i])
		}
	}

	return joined
}

func (s *Slk) IMs() []Entity {
	users := make([]Entity, 0)
	for i := range s.users {
		im := s.getIMByUser(s.users[i].GetID())
		if im != nilIM {
			users = append(users, s.users[i])
		}
	}

	return users
}

func (s *Slk) Post(e Entity, msg string) error {
	if err := s.post(e, msg); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	return nil
}

func (s *Slk) Mark(e Entity) error {
	var err error

	switch e.GetType() {
	case TypeChannel:
		if e.(*channel).isChannel {
			err = s.c.SetChannelReadMark(e.GetID(), e.getLatest())
			break
		}

		err = s.c.SetGroupReadMark(e.GetID(), e.getLatest())
	case TypeUser:
		err = s.c.MarkIMChannel(s.getIMByUser(e.GetID()).ID, e.getLatest())
	default:
		err = fmt.Errorf("Can't mark a %s", e.GetType())
	}

	if err != nil {
		s.out.Warn(err.Error())
	}

	return err
}

func (s *Slk) Unread(e Entity) error {
	last := e.getLastRead()
	latest := e.getLatest()
	fLast, _ := strconv.ParseFloat(last, 64)
	fLatest, _ := strconv.ParseFloat(latest, 64)
	if last == "" ||
		fLatest == 0.0 ||
		fLast == 0.0 ||
		last == latest ||
		fLast > fLatest {
		return nil
	}

	p := slack.NewHistoryParameters()
	p.Oldest = last

	var err error
	first := true
	for {
		p.Oldest, err = s.history(e, p, first)
		first = false
		if err != nil {
			s.out.Warn(err.Error())
			return err
		}

		if p.Oldest == "" {
			break
		}
	}

	s.Mark(e)

	return nil
}

func (s *Slk) History(e Entity, amount int) error {
	p := slack.NewHistoryParameters()
	p.Count = amount
	p.Inclusive = true

	if _, err := s.history(e, p, true); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	return nil
}

func (s *Slk) Fuzzy(entityType EntityType, query string) []Entity {
	s.RLock()
	defer s.RUnlock()

	lookup := map[string]Entity{}

	switch entityType {
	case TypeChannel:
		lookup = make(map[string]Entity, len(s.channelsByName))
		for i := range s.channelsByName {
			lookup[i] = s.channelsByName[i]
		}
	case TypeUser:
		lookup = make(map[string]Entity, len(s.usersByName))
		for i := range s.usersByName {
			lookup[i] = s.usersByName[i]
		}
	}

	return fuzzySearch(query, lookup)
}

func (s *Slk) List(eType EntityType, relevantOnly bool) error {
	s.RLock()
	defer s.RUnlock()

	var items ListItems
	var title string

	switch eType {
	case TypeChannel:
		title = "Channels:"
		items = make(ListItems, 0, len(s.channels))
		for _, c := range s.channels {
			status := ListItemStatusGood
			if !c.IsActive() {
				if relevantOnly {
					continue
				}
				status = ListItemStatusNormal
			}

			items = append(items, &ListItem{status, c.GetName()})
		}

	case TypeUser:
		title = "Users:"
		items = make(ListItems, 0, len(s.users))
		for _, u := range s.users {
			status := ListItemStatusGood
			if !u.IsActive() {
				if relevantOnly {
					continue
				}
				status = ListItemStatusNormal
			}

			items = append(items, &ListItem{status, u.GetName()})
		}
	}

	if items != nil {
		sort.Sort(items)
		s.out.List(title, items)
		return nil
	}

	return fmt.Errorf("Can not list items of type '%s'", eType)
}

func (s *Slk) Quit() {
	s.r.Disconnect()
	// TODO we might still receive events on s.r.IncomingEvents
	// which in turn might make s.out.* calls
	close(s.r.IncomingEvents)
}

func (s *Slk) Run() error {
	if s.r != nil {
		return errors.New("Already running?")
	}

	go func() {
		for {
			time.Sleep(time.Second * 20)
			s.updateIMs()
			s.updateUsers()
			s.updateChannels()
		}
	}()

	s.r = s.c.NewRTM()
	go s.r.ManageConnection()

	for e := range s.r.IncomingEvents {
		switch d := e.Data.(type) {
		case *slack.PresenceChangeEvent:
			s.getUser(d.User).Presence = d.Presence
			// TODO notice or something

		case *slack.ChannelJoinedEvent:
			channel := s.getChannel(d.Channel.ID)
			channel.isMember = true
		case *slack.ChannelLeftEvent:
			channel := s.getChannel(d.Channel)
			channel.isMember = false

		case *slack.GroupJoinedEvent:
			channel := s.getChannel(d.Channel.ID)
			channel.isMember = true
		case *slack.GroupLeftEvent:
			channel := s.getChannel(d.Channel)
			channel.isMember = false

		case *slack.UserTypingEvent:
			channel := s.getChannel(d.Channel)
			channelName := channel.GetName()
			if channel == nilChan {
				channelName = "Unknown channel / user"
				user := s.getUser(s.getIM(d.Channel).User)
				if user != nilUser {
					channelName = "IM"
				}
			}

			s.out.Typing(
				channelName,
				s.getUser(d.User).GetName(),
				time.Second*3,
			)
		case *slack.MessageEvent:
			m := slack.Message(*d)
			s.msg(&m, false, true)

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
				continue
			}

			ch := nilChan
			if len(d.File.Channels) != 0 {
				ch = s.getChannel(d.File.Channels[0])
			}

			url := d.File.URLPrivate
			if d.File.PermalinkPublic != "" {
				url = d.File.PermalinkPublic
			}

			s.out.File(
				ch.GetName(),
				s.getUser(d.File.User).GetName(),
				fmt.Sprintf("%s %s", d.File.Title, d.File.Name),
				url,
			)

		case *slack.PrefChangeEvent:
			// TODO lookup which preferences are relevant to slek.
			s.out.Debug(e.Type, d.Name, string(d.Value))
		case *slack.ConnectingEvent:
			s.out.Info("Connecting")
		case *slack.ConnectedEvent:
			s.out.Info("Connected!")
		case *slack.HelloEvent:
			s.out.Info("Slack: hello!")

		case *slack.FilePublicEvent:
			// TODO ignorable? slack.MessageEvent seems to suffice
			s.out.Debug(d.Type, fmt.Sprintf("%+v", d.File))
			// Ignore
		case *slack.IMMarkedEvent:
			// Ignore
		case *slack.LatencyReport:
			// Ignore
		case *slack.ReconnectUrlEvent:
			// Ignore
		default:
			// TODO
			s.out.Debug(e.Type, reflect.TypeOf(e.Data).String())
		}
	}

	return nil
}

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
	channel := s.getChannel(channelID)
	entity = channel

	if channel == nilChan || !channel.isMember {
		entity = s.getUser(s.getIM(channelID).User)
		if entity == nilUser {
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
		entity.GetType() == TypeUser,
	)
}

func (s *Slk) msg(m *slack.Message, newSection bool, notify bool) {
	if m.Hidden {
		// TODO we sure 'bout that?
		return
	}

	if m.User == "" && m.SubMessage != nil {
		m.Msg = *m.SubMessage
	}

	var entity Entity
	ch := s.getChannel(m.Channel)
	entity = ch

	im := false
	if ch == nilChan {
		user := s.getUser(s.getIM(m.Channel).User)
		im = user != nilUser
		entity = user
	}

	username := s.getUser(m.User).Name
	if m.User == "" && m.Username != "" {
		username = m.Username
	}

	text, mentions := s.parseText(
		append([]string{m.Text},
			s.parseAttachments(m.Attachments)...)...,
	)

	if notify {
		if im {
			if username != s.username {
				s.out.Notify(entity.GetQualifiedName(), username, text, false)
			}
		} else {
			for i := range mentions {
				if mentions[i] == s.username {
					s.out.Notify(
						entity.GetQualifiedName(),
						username,
						text,
						false,
					)
				}
			}
		}
	}

	s.out.Msg(
		entity.GetQualifiedName(),
		username,
		text,
		ts(m.Timestamp),
		newSection,
	)
}

func (s *Slk) updateUsers() error {
	users, err := s.c.GetUsers()
	if err != nil {
		return err
	}

	_users := make(map[string]*user, len(users))
	usersByName := make(map[string]*user, len(users))

	s.RLock()
	for i := range users {
		u := slackUserToUser(&users[i])
		_users[users[i].ID] = u
		usersByName[users[i].Name] = u
	}

	s.RUnlock()
	s.Lock()
	defer s.Unlock()
	s.users = _users
	s.usersByName = usersByName

	return nil
}

func (s *Slk) updateChannels() error {
	channels, err := s.c.GetChannels(true)
	if err != nil {
		return err
	}

	groups, err := s.c.GetGroups(true)
	if err != nil {
		return err
	}

	_channels := make(map[string]*channel, len(channels))
	channelsByName := make(map[string]*channel, len(channels))
	for i := range channels {
		_channels[channels[i].ID] = slackChannelToChannel(&channels[i])
	}

	for i := range groups {
		_channels[groups[i].ID] = slackGroupToChannel(&groups[i])
	}

	for i := range _channels {
		channelsByName[_channels[i].GetName()] = _channels[i]
	}

	s.Lock()
	defer s.Unlock()
	s.channels = _channels
	s.channelsByName = channelsByName

	return nil
}

func (s *Slk) updateIMs() error {
	channels, err := s.c.GetIMChannels()
	if err != nil {
		return err
	}

	s.Lock()
	defer s.Unlock()

	s.ims = make(map[string]*slack.IM, len(channels))
	s.imsByUser = make(map[string]*slack.IM, len(channels))
	for i := range channels {
		s.ims[channels[i].ID] = &channels[i]
		s.imsByUser[channels[i].User] = &channels[i]
	}

	return nil
}

func (s *Slk) getUser(id string) *user {
	s.RLock()
	defer s.RUnlock()
	if u, ok := s.users[id]; ok {
		return u
	}

	return nilUser
}

func (s *Slk) getChannel(id string) *channel {
	s.RLock()
	defer s.RUnlock()
	if ch, ok := s.channels[id]; ok {
		return ch
	}

	return nilChan
}

func (s *Slk) getIM(id string) *slack.IM {
	s.RLock()
	defer s.RUnlock()
	if im, ok := s.ims[id]; ok {
		return im
	}

	return nilIM
}

func (s *Slk) getIMByUser(id string) *slack.IM {
	s.RLock()
	defer s.RUnlock()
	if im, ok := s.imsByUser[id]; ok {
		return im
	}

	return nilIM
}
