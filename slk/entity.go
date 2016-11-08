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
	Is(Entity) bool

	lastRead() string
	latest() string

	setLastRead(string)
	setLatest(string)
	incrementUnread()
	resetUnread()
}

type entity struct {
	unread     int
	lastReadTs string
	latestTs   string
}

func (e *entity) UnreadCount() int     { return e.unread }
func (e *entity) lastRead() string     { return e.lastReadTs }
func (e *entity) latest() string       { return e.latestTs }
func (e *entity) setLastRead(l string) { e.lastReadTs = l }
func (e *entity) setLatest(l string)   { e.latestTs = l }
func (e *entity) incrementUnread()     { e.unread++ }
func (e *entity) resetUnread()         { e.unread = 0 }

type channel struct {
	entity
	id        string
	name      string
	creator   string
	members   []string
	isChannel bool
	isMember  bool
}

func (c *channel) ID() string            { return c.id }
func (c *channel) Name() string          { return c.name }
func (c *channel) QualifiedName() string { return "#" + c.name }
func (c *channel) Type() EntityType      { return TypeChannel }
func (c *channel) IsActive() bool        { return c.isMember }
func (c *channel) IsNil() bool           { return c.id == nilID }
func (c *channel) Is(entity Entity) bool {
	return entity != nil &&
		c.id == entity.ID() && entity.Type() == c.Type()
}

type user struct {
	*slack.User
	entity
}

func (u *user) ID() string            { return u.User.ID }
func (u *user) Name() string          { return u.User.Name }
func (u *user) QualifiedName() string { return "@" + u.User.Name }
func (u *user) Type() EntityType      { return TypeUser }
func (u *user) IsActive() bool        { return u.Presence == userPresenceActive }
func (u *user) IsNil() bool           { return u.User.ID == nilID }
func (u *user) Is(entity Entity) bool {
	return entity != nil &&
		u.User.ID == entity.ID() && entity.Type() == u.Type()
}

func slackChannelToChannel(c *slack.Channel, original *channel) *channel {
	ch := &channel{
		id:        c.ID,
		name:      c.Name,
		creator:   c.Creator,
		members:   c.Members,
		isChannel: true,
		isMember:  c.IsMember,

		entity: entity{
			lastReadTs: c.LastRead,
			unread:     c.UnreadCount,
		},
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

		entity: entity{
			lastReadTs: g.LastRead,
			unread:     g.UnreadCount,
		},
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
