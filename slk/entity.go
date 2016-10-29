package slk

import "github.com/nlopes/slack"

const (
	userPresenceActive = "active"
)

const (
	nilID                  = "-"
	nilName                = "UNKNOWN"
	TypeChannel EntityType = "channel"
	TypeUser    EntityType = "user"
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
	nilIM.ID = "-"
}

type EntityType string

type Entity interface {
	GetID() string
	GetName() string
	GetQualifiedName() string
	GetType() EntityType
	GetUnreadCount() int
	IsActive() bool
	IsNil() bool
	getLastRead() string
	getLatest() string
}

type channel struct {
	id        string
	name      string
	creator   string
	members   []string
	isChannel bool
	isMember  bool
	unread    int
	lastRead  string
	latest    string
}

func (c *channel) GetID() string {
	return c.id
}

func (c *channel) GetName() string {
	return c.name
}

func (c *channel) GetQualifiedName() string {
	return "#" + c.name
}

func (c *channel) GetType() EntityType {
	return TypeChannel
}

func (c *channel) GetUnreadCount() int {
	return c.unread
}

func (c *channel) IsActive() bool {
	return c.isMember
}

func (c *channel) IsNil() bool {
	return c.GetID() == nilID
}

func (c *channel) getLastRead() string {
	return c.lastRead
}

func (c *channel) getLatest() string {
	return c.latest
}

type user struct {
	*slack.User
	unread   int
	lastRead string
	latest   string
}

func (u *user) GetID() string {
	return u.ID
}

func (u *user) GetName() string {
	return u.Name
}

func (u *user) GetQualifiedName() string {
	return "@" + u.Name
}

func (u *user) GetType() EntityType {
	return TypeUser
}

func (u *user) GetUnreadCount() int {
	return u.unread
}

func (u *user) IsActive() bool {
	return u.Presence == userPresenceActive
}

func (u *user) IsNil() bool {
	return u.GetID() == nilID
}

func (u *user) getLastRead() string {
	return u.lastRead
}

func (u *user) getLatest() string {
	return u.latest
}

func slackChannelToChannel(c *slack.Channel) *channel {
	return &channel{
		id:        c.ID,
		name:      c.Name,
		creator:   c.Creator,
		members:   c.Members,
		isChannel: true,
		isMember:  c.IsMember,
	}
}

func slackGroupToChannel(g *slack.Group) *channel {
	return &channel{
		id:        g.ID,
		name:      g.Name,
		creator:   g.Creator,
		members:   g.Members,
		isChannel: false,
		isMember:  true,
	}
}

func slackUserToUser(u *slack.User) *user {
	usr := &user{User: u}

	return usr
}
