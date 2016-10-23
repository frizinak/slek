package slk

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nlopes/slack"
)

var (
	reChannel     = regexp.MustCompile(`<#([^>]+)>`)
	reChannelRepl = regexp.MustCompile(`<#([^>|]+)>`)
	reMention     = regexp.MustCompile(`<@([^>]+)>`)
	reMentionRepl = regexp.MustCompile(`<@([^>|]+)`)
)

func ts(ts string) (t time.Time) {
	_s := strings.Split(ts, ".")
	sec, err := strconv.Atoi(_s[0])
	if err != nil {
		return
	}

	nsec, err := strconv.ParseFloat("0."+_s[1], 64)
	if err != nil {
		return
	}

	t = time.Unix(int64(sec), int64(nsec*1e9))

	return
}

func (s *Slk) parseText(texts ...string) (parsed string, mentions []string) {
	clean := make([]string, 0, len(texts))
	for i := range texts {

		txt := reMention.ReplaceAllStringFunc(
			html.UnescapeString(texts[i]),
			func(str string) string {
				var id string
				m := reMentionRepl.FindStringSubmatch(str)

				if len(m) > 1 {
					id = m[1]
				} else {
					id = strings.Trim(str, "@<>")
				}

				u := s.getUser(id)
				if u != nilUser {
					mentions = append(mentions, u.Name)
					return fmt.Sprintf("@%s", u.Name)
				}

				return id
			},
		)

		txt = reChannel.ReplaceAllStringFunc(
			txt,
			func(str string) string {
				var id string
				m := reChannelRepl.FindStringSubmatch(str)

				if len(m) > 1 {
					id = m[1]
				} else {
					id = strings.Trim(str, "#<>")
				}

				c := s.getChannel(id)
				if c != nilChan {
					return fmt.Sprintf("#%s", c.name)
				}

				return id
			},
		)

		if txt == "" {
			continue
		}

		clean = append(clean, txt)
	}

	parsed = strings.Join(clean, "\n")
	return
}

func (s *Slk) parseAttachments(attachments []slack.Attachment) []string {
	texts := make([]string, 0, len(attachments))
	for i := range attachments {
		img := attachments[i].ImageURL
		if img == "" {
			img = attachments[i].ThumbURL
		}

		if img != "" &&
			attachments[i].Title == "" &&
			attachments[i].Pretext == "" &&
			attachments[i].Text == "" {
			// Probably just an expanded image, ignore
			continue
		}

		txt := "-"
		if attachments[i].Title != "" {
			txt = attachments[i].Title + ": "
		}

		if attachments[i].Pretext != "" {
			txt += attachments[i].Pretext + "\n"
		}

		if img != "" {
			txt += img + "\n"
		}

		if attachments[i].Text != "" {
			txt += attachments[i].Text
		}

		texts = append(texts, txt)
	}

	return texts
}
