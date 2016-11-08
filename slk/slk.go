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
	active         Entity
	markRead       chan Entity
	quit           chan error
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
		nil,
		make(chan Entity, 1),
		make(chan error, 0),
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

// Quit closes the slack RTM connection.
func (s *Slk) Quit() {
	s.quit <- nil
	close(s.markRead)
	s.r.Disconnect()
	close(s.r.IncomingEvents)
}

// Username returns the name of the user whose api key we are using.
// Will be populated after Init.
func (s *Slk) Username() string {
	return s.username
}

// Uploads lists the first api page of uploads of the given entity.
func (s *Slk) Uploads(e Entity) error {
	var id string
	switch e.Type() {
	case TypeUser:
		id = s.imByUser(e.ID()).ID
	case TypeChannel:
		id = e.ID()
	default:
		err := fmt.Errorf("Can list uploads of a '%s'", e.Type())
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
		from := s.user(files[i].User).QualifiedName()
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
		fmt.Sprintf("files of %s", e.QualifiedName()),
	}

	s.out.List(items, false)
	return nil
}

// Upload a file to the given entity.
func (s *Slk) Upload(e Entity, filepath, title, comment string) chan error {
	ch := make(chan error, 1)

	var id string
	switch e.Type() {
	case TypeUser:
		id = s.imByUser(e.ID()).ID
	case TypeChannel:
		id = e.ID()
	default:
		err := fmt.Errorf("Can not upload to a '%s'", e.Type())
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

	s.out.Notice(
		fmt.Sprintf(
			"Starting upload of %s to %s",
			filepath,
			e.QualifiedName(),
		),
	)

	go func() {
		defer close(ch)
		_, err := s.c.UploadFile(p)
		if err != nil {
			s.out.Warn(err.Error())
		} else {
			s.out.Info(
				fmt.Sprintf(
					"Uploaded %s to %s",
					filepath,
					e.QualifiedName(),
				),
			)
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

	s.out.Info(fmt.Sprintf("Invited %s to %s", user.Name(), channel.Name()))
	return nil
}

// Join makes your user join the given channel or group.
func (s *Slk) Join(e Entity) error {
	if err := s.join(e); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Joined %s", e.Name()))
	return nil
}

// Leave makes your user leave the given channel or group.
func (s *Slk) Leave(e Entity) error {
	if err := s.leave(e); err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Info(fmt.Sprintf("Left %s", e.Name()))
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

// Switch to the given entity and fetch unread history.
func (s *Slk) Switch(e Entity) error {
	if e.Is(s.active) {
		return nil
	}

	s.active = e
	if err := s.Unread(e); err != nil {
		return err
	}

	return nil
}

// IMs returns a list of users you have intiated an IM channel with.
func (s *Slk) IMs() []Entity {
	users := make([]Entity, 0)
	for i := range s.users {
		im := s.imByUser(s.users[i].ID())
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

// Unread writes all unread mesages of the given user, channel or group
// to the Output interface and marks the last message as read.
func (s *Slk) Unread(e Entity) error {
	last := e.lastRead()
	latest := e.latest()
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

	var latestFetched string
	var done bool
	var err error
	first := true
	for {
		latestFetched, done, err = s.history(e, p, first)
		first = false
		if err != nil {
			s.out.Warn(err.Error())
			return err
		}

		if done {
			break
		}

		p.Oldest = latestFetched
	}

	s.markRead <- e
	return nil
}

// History like Unread writes a set of messages to the Output interface
// but takes an amount of messages argument instead of looking up unread
// messages.
func (s *Slk) History(e Entity, amount int) error {
	p := slack.NewHistoryParameters()
	p.Count = amount
	p.Inclusive = true

	latest, done, err := s.history(e, p, true)
	if err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Notice(fmt.Sprintf("%+v %s", done, latest))
	if done {
		e.setLatest(latest)
		s.markRead <- e
	}

	return nil
}

// Pins writes the last 100 (?) pins of a channel or group to the
// Output interface.
func (s *Slk) Pins(e Entity) error {
	var err error
	var items []slack.Item

	if e.Type() != TypeChannel {
		err = fmt.Errorf("Can't list pins of a %s", e.Type())
		s.out.Warn(err.Error())
		return err
	}

	items, _, err = s.c.ListPins(e.ID())
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
			from = s.user(items[i].File.User)
			url = items[i].File.URLPrivate
			timestamp = items[i].File.Timestamp.Time()
		}

		if items[i].Message != nil {
			if from.IsNil() {
				from = s.user(items[i].Message.User)
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
					from.QualifiedName(),
					timestamp.Format(timeFormat),
				),
			},
		)

	}

	listItems = append(
		listItems,
		&ListItem{
			ListItemStatusTitle,
			fmt.Sprintf("Pinned in %s", e.QualifiedName()),
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

// NextUnread returns a random entity (ims first) with unread messages.
func (s *Slk) NextUnread() (Entity, error) {

	for i := range s.users {
		if !s.users[i].Is(s.active) && s.users[i].UnreadCount() != 0 {
			return s.users[i], nil
		}
	}

	for i := range s.channels {
		if !s.channels[i].Is(s.active) && s.channels[i].UnreadCount() != 0 {
			return s.channels[i], nil
		}
	}

	return nil, errors.New("No channel or user with unread messages")
}

// ListUnread writes a list of entities with unread messages to the Output.
func (s *Slk) ListUnread() error {
	s.RLock()
	defer s.RUnlock()

	userList := make(ListItems, 0)
	channelList := make(ListItems, 0)

	for i := range s.users {
		if s.users[i].UnreadCount() != 0 {
			userList = append(
				userList,
				&ListItem{
					ListItemStatusNormal,
					fmt.Sprintf(
						"%-18s [%d]",
						s.users[i].QualifiedName(),
						s.users[i].UnreadCount(),
					),
				},
			)
		}
	}

	for i := range s.channels {
		if s.channels[i].UnreadCount() != 0 {
			channelList = append(
				channelList,
				&ListItem{
					ListItemStatusNormal,
					fmt.Sprintf(
						"%-18s [%d]",
						s.channels[i].QualifiedName(),
						s.channels[i].UnreadCount(),
					),
				},
			)
		}
	}

	sort.Sort(userList)
	sort.Sort(channelList)

	list := make(ListItems, 0, len(userList)+len(channelList)+2)
	list = append(list, &ListItem{ListItemStatusTitle, "Users:"})
	list = append(list, userList...)
	list = append(list, &ListItem{ListItemStatusTitle, "Channels:"})
	list = append(list, channelList...)

	s.out.List(list, false)
	return nil
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

			txt := c.Name()
			if ur := c.UnreadCount(); ur != 0 {
				txt = fmt.Sprintf("%-18s [%d]", txt, ur)
			}

			items = append(items, &ListItem{status, txt})
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

			txt := u.Name()
			if ur := u.UnreadCount(); ur != 0 {
				txt = fmt.Sprintf("%-18s [%d]", txt, ur)
			}

			items = append(items, &ListItem{status, txt})
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
func (s *Slk) Members(e Entity, relevantOnly bool) error {
	channel, ok := e.(*channel)
	if !ok {
		err := fmt.Errorf("Can't list members of a %s", e.Type())
		s.out.Warn(err.Error())
		return err
	}

	items := make(ListItems, 0, len(channel.members))
	for i := range channel.members {
		user := s.user(channel.members[i])
		status := ListItemStatusGood
		if !user.IsActive() {
			if relevantOnly {
				continue
			}

			status = ListItemStatusNormal
		}

		items = append(items, &ListItem{status, user.Name()})
	}

	sort.Sort(items)
	_items := make(ListItems, 1, len(items)+1)
	_items[0] = &ListItem{
		ListItemStatusTitle,
		fmt.Sprintf("Users in %s", channel.QualifiedName()),
	}
	_items = append(_items, items...)

	s.out.List(
		_items,
		false,
	)

	return nil
}

// Run starts handling events.
// Should only be called once.
func (s *Slk) Run() error {
	if s.r == nil {
		return errors.New("Forgot to call Init()?")
	}

	go func() {
		// TODO Quit should also quit this loop.
		for {
			time.Sleep(time.Minute)
			// s.updateUsers(nil)
			// s.updateIMs(nil)
			// Still required for channel.members
			s.updateChannels(nil, nil)
		}
	}()

	markReadEntities := make(map[EntityType]map[string]Entity, 2)

	markTimeout := time.After(time.Second * 5)
	for {
		select {
		case err := <-s.quit:
			return err

		case e := <-s.r.IncomingEvents:
			if err := s.handleEvent(e); err != nil {
				s.Quit()
				return err
			}

		case e := <-s.markRead:
			typ := e.Type()
			if _, ok := markReadEntities[typ]; !ok {
				markReadEntities[typ] = make(map[string]Entity)
			}
			markReadEntities[typ][e.ID()] = e

		case <-markTimeout:
		Outer:
			for typ := range markReadEntities {
				for name, e := range markReadEntities[typ] {
					s.mark(e)
					delete(markReadEntities[typ], name)

					break Outer
				}
			}

			markTimeout = time.After(time.Second * 5)
		}
	}
}

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
		s.out.Debug(event.Type, reflect.TypeOf(event.Data).String())
	}

	return nil
}

// Mark the last read message in an IM, channel or group.
func (s *Slk) mark(e Entity) error {
	var err error

	latest := e.latest()
	switch e.Type() {
	case TypeChannel:
		if e.(*channel).isChannel {
			err = s.c.SetChannelReadMark(e.ID(), latest)
			latest = e.latest()
			break
		}

		err = s.c.SetGroupReadMark(e.ID(), latest)
	case TypeUser:
		err = s.c.MarkIMChannel(s.imByUser(e.ID()).ID, latest)
	default:
		err = fmt.Errorf("Can't mark a %s", e.Type())
	}

	if err != nil {
		s.out.Warn(err.Error())
		return err
	}

	s.out.Notice(fmt.Sprintf("marked %s %s", e.QualifiedName(), time.Now().String()))
	e.resetUnread()
	e.setLastRead(latest)

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

	if notify {
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
		u := s.user(ims[i].User)
		if ims[i].LastRead != "" {
			u.lastReadTs = ims[i].LastRead
			u.unread = ims[i].UnreadCount
		}
		if ims[i].Latest != nil && ims[i].Latest.Timestamp != "" {
			u.latestTs = ims[i].Latest.Timestamp
		}
	}

	s.Lock()
	defer s.Unlock()
	s.ims = _ims
	s.imsByUser = _imsByUser

	return nil
}

func (s *Slk) user(id string) *user {
	s.RLock()
	defer s.RUnlock()
	if u, ok := s.users[id]; ok {
		return u
	}

	return nilUser
}

func (s *Slk) userByName(name string) *user {
	s.RLock()
	defer s.RUnlock()
	if u, ok := s.usersByName[name]; ok {
		return u
	}

	return nilUser
}

func (s *Slk) channel(id string) *channel {
	s.RLock()
	defer s.RUnlock()
	if ch, ok := s.channels[id]; ok {
		return ch
	}

	return nilChan
}

func (s *Slk) channelByName(name string) *channel {
	s.RLock()
	defer s.RUnlock()
	if ch, ok := s.channelsByName[name]; ok {
		return ch
	}

	return nilChan
}

func (s *Slk) im(id string) *slack.IM {
	s.RLock()
	defer s.RUnlock()
	if im, ok := s.ims[id]; ok {
		return im
	}

	return nilIM
}

func (s *Slk) imByUser(id string) *slack.IM {
	s.RLock()
	defer s.RUnlock()
	if im, ok := s.imsByUser[id]; ok {
		return im
	}

	return nilIM
}
