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

const timeFormat = "02/01 15:04:05"

// Slk abstracts a bunch of nlopes/slack calls and writes all output to
// the given Output interface.
//
// Handling of errors returned by Slk exposed methods is optional.
// Except for Init and Run.
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

// NewSlk returns a new Slk 'engine'.
func NewSlk(token string, output Output) *Slk {
	var rw sync.RWMutex

	return &Slk{
		output,
		"",
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

// Init establishes the rtm connection and returns once we have all
// channel / group / im / user information. Should be called before Run().
func (s *Slk) Init() error {
	if s.r != nil {
		return errors.New("Already initiated?")
	}

	s.r = s.c.NewRTM()
	go s.r.ManageConnection()
	for {
		select {
		case <-time.After(time.Millisecond * 50):
			d := s.r.GetInfo()
			if d == nil {
				continue
			}

			s.username = d.User.Name
			s.updateUsers(d.Users)
			s.updateIMs(d.IMs)
			s.updateChannels(d.Channels, d.Groups)
			return nil
		case <-time.After(time.Second * 5):
			return errors.New("Could not establish rtm connection")
		}
	}
}

// GetUsername returns the name of the user whose api key we are using.
// Will be populated after Init.
func (s *Slk) GetUsername() string {
	return s.username
}

// Uploads lists the first api page of uploads of the given entity.
func (s *Slk) Uploads(e Entity) error {
	var id string
	switch e.GetType() {
	case TypeUser:
		id = s.getIMByUser(e.GetID()).ID
	case TypeChannel:
		id = e.GetID()
	default:
		err := fmt.Errorf("Can list uploads of a '%s'", e.GetType())
		s.out.Warn(err.Error())
		return err
	}

	p := slack.NewGetFilesParameters()
	p.Channel = id
	files, _, err := s.c.GetFiles(p)
	if err != nil {
		s.out.Warn(err.Error())
		return err
	}

	items := make(ListItems, len(files)*2+1)
	for i := range files {
		from := s.getUser(files[i].User).GetQualifiedName()
		items[i*2+1] = &ListItem{
			ListItemStatusNormal,
			fmt.Sprintf(
				"%s: %s",
				from,
				files[i].Timestamp.Time().Format(timeFormat),
			),
		}
		items[i*2+2] = &ListItem{ListItemStatusNone, files[i].URLPrivate}
	}

	items[0] = &ListItem{
		ListItemStatusTitle,
		fmt.Sprintf("files of %s", e.GetQualifiedName()),
	}

	s.out.List(items, false)
	return nil
}

// Upload a file to the given entity.
func (s *Slk) Upload(e Entity, filepath, title, comment string) chan error {
	ch := make(chan error, 1)

	var id string
	switch e.GetType() {
	case TypeUser:
		id = s.getIMByUser(e.GetID()).ID
	case TypeChannel:
		id = e.GetID()
	default:
		err := fmt.Errorf("Can not upload to a '%s'", e.GetType())
		s.out.Warn(err.Error())
		ch <- err
		close(ch)
		return ch
	}

	p := slack.FileUploadParameters{
		File:     filepath,
		Channels: []string{id},
	}

	if title != "" {
		p.Title = title
	}

	if comment != "" {
		p.InitialComment = comment
	}

	s.out.Info(
		fmt.Sprintf(
			"Starting upload of %s to %s",
			filepath,
			e.GetQualifiedName(),
		),
	)

	go func() {
		defer close(ch)
		_, err := s.c.UploadFile(p)
		if err != nil {
			s.out.Warn(err.Error())
		}
		ch <- err
	}()

	return ch
}

// Invite a user to a channel or group.
func (s *Slk) Invite(channel, user Entity) error {
	if err := s.invite(channel, user); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Invited %s to %s", user.GetName(), channel.GetName()))
	return nil
}

// Join makes your user join the given channel or group.
func (s *Slk) Join(e Entity) error {
	if err := s.join(e); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Joined %s", e.GetName()))
	return nil
}

// Leave makes your user leave the given channel or group.
func (s *Slk) Leave(e Entity) error {
	if err := s.leave(e); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Left %s", e.GetName()))
	return nil
}

// Joined returns a list of channels and groups you are a member of.
func (s *Slk) Joined() []Entity {
	joined := make([]Entity, 0)
	for i := range s.channels {
		if s.channels[i].IsActive() {
			joined = append(joined, s.channels[i])
		}
	}

	return joined
}

// IMs returns a list of users you have intiated an IM channel with.
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

// Post a message to the given user, channel or group.
func (s *Slk) Post(e Entity, msg string) error {
	if err := s.post(e, msg); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	return nil
}

// Mark the last read message in an IM, channel or group.
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

// Unread writes all unread mesages of the given user, channel or group
// to the Output interface and marks the last message as read.
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

// History like Unread writes a set of messages to the Output interface
// but takes an amount of messages argument instead of looking up unread
// messages.
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

// Pins writes the last 100 (?) pins of a channel or group to the
// Output interface.
func (s *Slk) Pins(e Entity) error {
	var err error
	var items []slack.Item

	if e.GetType() != TypeChannel {
		err = fmt.Errorf("Can't list pins of a %s", e.GetType())
		s.out.Warn(err.Error())
		return err
	}

	items, _, err = s.c.ListPins(e.GetID())
	if err != nil {
		s.out.Warn(err.Error())
		return err
	}

	listItems := make(ListItems, 0, len(items)+1)
	for i := range items {
		var url string
		var msg string
		var from = nilUser
		var timestamp time.Time

		if items[i].File != nil {
			from = s.getUser(items[i].File.User)
			url = items[i].File.URLPrivate
			timestamp = items[i].File.Timestamp.Time()
		}

		if items[i].Message != nil {
			if from.IsNil() {
				from = s.getUser(items[i].Message.User)
			}
			msg = items[i].Message.Text
			timestamp = ts(items[i].Message.Timestamp)
		}

		// Don't return early so we know parsing is flawed
		// if only the username and stamp are shown without msg or url.

		if msg == "" {
			listItems = append(listItems, &ListItem{ListItemStatusNone, url})
		} else {
			listItems = append(listItems, &ListItem{ListItemStatusNone, msg})
		}

		listItems = append(
			listItems,
			&ListItem{
				ListItemStatusNormal,
				fmt.Sprintf(
					"%s: %s",
					from.GetQualifiedName(),
					timestamp.Format(timeFormat),
				),
			},
		)

	}

	listItems = append(
		listItems,
		&ListItem{
			ListItemStatusTitle,
			fmt.Sprintf("Pinned in %s", e.GetQualifiedName()),
		},
	)

	s.out.List(
		listItems,
		true,
	)

	return nil
}

// Fuzzy returns a list of entities of type entityType whose names fuzzy match
// the given query.
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

// List writes a list of entities of type entityType to the Output interface.
func (s *Slk) List(entityType EntityType, relevantOnly bool) error {
	s.RLock()
	defer s.RUnlock()

	var items ListItems
	var title string

	switch entityType {
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
		_items := make(ListItems, 1, len(items)+1)
		_items[0] = &ListItem{ListItemStatusTitle, title}
		_items = append(_items, items...)
		s.out.List(_items, false)
		return nil
	}

	err := fmt.Errorf("Can not list items of type %s", entityType)
	s.out.Warn(err.Error())

	return err
}

// Members writes a list of members of the given channel or group to the
// Output interface.
func (s *Slk) Members(e Entity, relevanOnly bool) error {
	channel, ok := e.(*channel)
	if !ok {
		err := fmt.Errorf("Can't list members of a %s", e.GetType())
		s.out.Warn(err.Error())
		return err
	}

	items := make(ListItems, 0, len(channel.members))
	for i := range channel.members {
		user := s.getUser(channel.members[i])
		status := ListItemStatusGood
		if !user.IsActive() {
			if relevanOnly {
				continue
			}

			status = ListItemStatusNormal
		}

		items = append(items, &ListItem{status, user.GetName()})
	}

	sort.Sort(items)
	_items := make(ListItems, 1, len(items)+1)
	_items[0] = &ListItem{
		ListItemStatusTitle,
		fmt.Sprintf("Users in %s", channel.GetQualifiedName()),
	}
	_items = append(_items, items...)

	s.out.List(
		_items,
		false,
	)

	return nil
}

// Quit closes the slack RTM connection.
func (s *Slk) Quit() {
	s.r.Disconnect()
	// TODO we might still receive events on s.r.IncomingEvents
	// which in turn might make s.out.* calls
	close(s.r.IncomingEvents)
}

// Run opens the slack RTM connection and starts handling events.
// Should only be called once.
func (s *Slk) Run() error {
	if s.r == nil {
		return errors.New("Forgot to call Init()?")
	}

	go func() {
		// TODO Quit should also quit this loop.
		for {
			time.Sleep(time.Second * 60)
			// s.updateUsers(nil)
			// s.updateIMs(nil)
			// Still required for channel.members
			s.updateChannels(nil, nil)
		}
	}()

	for e := range s.r.IncomingEvents {
		switch d := e.Data.(type) {

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
			if channel.IsNil() {
				channelName = "Unknown channel / user"
				user := s.getUser(s.getIM(d.Channel).User)
				if !user.IsNil() {
					channelName = "IM"
				}
			}

			s.out.Typing(
				channelName,
				s.getUser(d.User).GetName(),
				time.Second*4,
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
			switch d.Name {
			case "emoji_use":
				continue
			}
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

	if channel.IsNil() || !channel.isMember {
		entity = s.getUser(s.getIM(channelID).User)
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
	if ch.IsNil() {
		user := s.getUser(s.getIM(m.Channel).User)
		im = !user.IsNil()
		entity = user
	}

	username := s.getUser(m.User).Name
	if m.User == "" && m.Username != "" {
		username = m.Username
	}

	text, mentions := s.parseTextIncoming(
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
		u := s.getUser(ims[i].User)
		u.lastRead = ims[i].LastRead
		u.unread = ims[i].UnreadCount
		if ims[i].Latest != nil {
			u.latest = ims[i].Latest.Timestamp
		}
	}

	s.Lock()
	defer s.Unlock()
	s.ims = _ims
	s.imsByUser = _imsByUser

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

func (s *Slk) getUserByName(name string) *user {
	s.RLock()
	defer s.RUnlock()
	if u, ok := s.usersByName[name]; ok {
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

func (s *Slk) getChannelByName(name string) *channel {
	s.RLock()
	defer s.RUnlock()
	if ch, ok := s.channelsByName[name]; ok {
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
