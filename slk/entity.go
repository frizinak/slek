package slk

import "github.com/nlopes/slack"

const (
	userPresenceActive = "active"
)

var (
	nilUser = &user{
		User: &slack.User{
			ID:       "-",
			Name:     "UNKNOWN USER",
			RealName: "UNKNOWN USER",
		},
	}

	nilChan = &channel{}
	nilIM   = &slack.IM{User: "-"}

	TypeChannel EntityType = "channel"
	TypeUser    EntityType = "user"
)

func init() {
	nilChan.id = "-"
	nilChan.name = "UNKNOWN CHANNEL"
	nilChan.creator = "-"
	nilChan.members = []string{}

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
