package slk

import "github.com/nlopes/slack"

const (
	userPresenceActive = "active"
	nilID              = "-"
	nilName            = "UNKNOWN"

	// TypeChannel identifies the channel-type
	TypeChannel EntityType = "channel"
	// TypeUser identifies the user-type
	TypeUser EntityType = "user"
)

var (
	nilUser = &user{
		User: &slack.User{
			ID:       nilID,
			Name:     nilName,
			RealName: nilName,
		},
	}

	nilChan = &channel{
		id:      nilID,
		name:    nilName,
		creator: nilName,
		members: []string{},
	}

	nilIM = &slack.IM{User: nilID}
)

func init() {
	nilIM.ID = "-" // :(
}

// EntityType represents a slack entity-type (i.e.: channel, user, ...)
type EntityType string

// Entity abstracts users, groups and channels
type Entity interface {
	ID() string
	Name() string
	QualifiedName() string
	Type() EntityType
	UnreadCount() int
	IsActive() bool
	IsNil() bool

	lastRead() string
	latest() string

	setLastRead(string)
	setLatest(string)
	incrementUnread()
	resetUnread()
}

type channel struct {
	id         string
	name       string
	creator    string
	members    []string
	isChannel  bool
	isMember   bool
	unread     int
	lastReadTs string
	latestTs   string
}

func (c *channel) ID() string            { return c.id }
func (c *channel) Name() string          { return c.name }
func (c *channel) QualifiedName() string { return "#" + c.name }
func (c *channel) Type() EntityType      { return TypeChannel }
func (c *channel) UnreadCount() int      { return c.unread }
func (c *channel) IsActive() bool        { return c.isMember }
func (c *channel) IsNil() bool           { return c.id == nilID }
func (c *channel) lastRead() string      { return c.lastReadTs }
func (c *channel) latest() string        { return c.latestTs }
func (c *channel) setLastRead(l string)  { c.lastReadTs = l }
func (c *channel) setLatest(l string)    { c.latestTs = l }
func (c *channel) incrementUnread()      { c.unread++ }
func (c *channel) resetUnread()          { c.unread = 0 }

type user struct {
	*slack.User
	unread     int
	lastReadTs string
	latestTs   string
}

func (u *user) ID() string            { return u.User.ID }
func (u *user) Name() string          { return u.User.Name }
func (u *user) QualifiedName() string { return "@" + u.User.Name }
func (u *user) Type() EntityType      { return TypeUser }
func (u *user) UnreadCount() int      { return u.unread }
func (u *user) IsActive() bool        { return u.Presence == userPresenceActive }
func (u *user) IsNil() bool           { return u.User.ID == nilID }
func (u *user) lastRead() string      { return u.lastReadTs }
func (u *user) latest() string        { return u.latestTs }
func (u *user) setLastRead(l string)  { u.lastReadTs = l }
func (u *user) setLatest(l string)    { u.latestTs = l }
func (u *user) incrementUnread()      { u.unread++ }
func (u *user) resetUnread()          { u.unread = 0 }

func slackChannelToChannel(c *slack.Channel, original *channel) *channel {
	ch := &channel{
		id:        c.ID,
		name:      c.Name,
		creator:   c.Creator,
		members:   c.Members,
		isChannel: true,
		isMember:  c.IsMember,

		lastReadTs: c.LastRead,
		unread:     c.UnreadCount,
	}

	if c.Latest != nil {
		ch.latestTs = c.Latest.Timestamp
	}

	if original != nil && !original.IsNil() && ch.lastReadTs == "" {
		ch.lastReadTs = original.lastReadTs
		ch.latestTs = original.latestTs
		ch.unread = original.unread
	}

	return ch
}

func slackGroupToChannel(g *slack.Group, original *channel) *channel {
	ch := &channel{
		id:        g.ID,
		name:      g.Name,
		creator:   g.Creator,
		members:   g.Members,
		isChannel: false,
		isMember:  true,

		lastReadTs: g.LastRead,
		unread:     g.UnreadCount,
	}

	if g.Latest != nil {
		ch.latestTs = g.Latest.Timestamp
	}

	if original != nil && !original.IsNil() && ch.lastReadTs == "" {
		ch.lastReadTs = original.lastReadTs
		ch.latestTs = original.latestTs
		ch.unread = original.unread
	}

	return ch
}

func slackUserToUser(u *slack.User, original *user) *user {
	usr := &user{User: u}

	if original != nil && !original.IsNil() && usr.lastReadTs == "" {
		usr.lastReadTs = original.lastReadTs
		usr.latestTs = original.latestTs
		usr.unread = original.unread
	}

	return usr
}
