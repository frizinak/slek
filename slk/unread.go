package slk

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/url"

	"github.com/nlopes/slack"
)

// Get users.counts (hidden api method)
// https://github.com/slackhq/slack-api-docs/blob/d4c5fca36d163fc2d62d28d49d443fe84dde17a9/methods/users.counts.md
// https://github.com/slackhq/slack-api-docs/commit/332f7a0837ff023e98dc1688822b8d43b8118469

// Mimic github.com/nlopes/slack

type specialIM struct {
	ID                 string `json:"id"`
	LastRead           string `json:"last_read,omitempty"`
	Latest             string `json:"latest,omitempty"`
	UnreadCount        int    `json:"unread_count,omitempty"`
	UnreadCountDisplay int    `json:"unread_count_display,omitempty"`
	UserID             string `json:"user_id"`
}

type specialChannel struct {
	ID                 string `json:"id"`
	LastRead           string `json:"last_read,omitempty"`
	Latest             string `json:"latest,omitempty"`
	UnreadCount        int    `json:"unread_count,omitempty"`
	UnreadCountDisplay int    `json:"unread_count_display,omitempty"`
}

type usersCountsResponseFull struct {
	Channels []*specialChannel `json:"channels"`
	Groups   []*specialChannel `json:"groups"`
	IMs      []*specialIM      `json:"ims"`
	slack.SlackResponse
}

func (s *Slk) updateUsersCounts() error {
	// Same values as the webapp call. (as of 2016/10/21)
	values := url.Values{
		"token":             {s.token},
		"only_relevant_ims": {"true"},
		"simple_unreads":    {"true"},
	}

	response := &usersCountsResponseFull{}

	httpResp, err := slack.HTTPClient.PostForm(slack.SLACK_API+"users.counts", values)
	if httpResp != nil && httpResp.Body != nil {
		defer httpResp.Body.Close()
	}

	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, response); err != nil {
		return err
	}

	if !response.Ok {
		return errors.New(response.Error)
	}

	s.RLock()
	defer s.RUnlock()

	for i := range response.Channels {
		c := s.getChannel(response.Channels[i].ID)
		if c != nilChan {
			c.lastRead = response.Channels[i].LastRead
			c.latest = response.Channels[i].Latest
			c.unread = response.Channels[i].UnreadCount
		}
	}

	for i := range response.Groups {
		c := s.getChannel(response.Groups[i].ID)
		if c != nilChan {
			c.lastRead = response.Groups[i].LastRead
			c.latest = response.Groups[i].Latest
			c.unread = response.Groups[i].UnreadCount
		}
	}

	for i := range response.IMs {
		u := s.getUser(response.IMs[i].UserID)
		if u != nilUser {
			u.lastRead = response.IMs[i].LastRead
			u.latest = response.IMs[i].Latest
			u.unread = response.IMs[i].UnreadCount
		}
	}

	return nil
}
