package slk

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nlopes/slack"
)

var (
	reEntity     = regexp.MustCompile(`<(#|@)([^>]+)>`)
	reEntityRepl = regexp.MustCompile(`<(#|@)([^>|]+)(?:|[^>]+)?>`)
)

func ts(ts string) (t time.Time) {
	t = time.Unix(0, 0)
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

func (s *Slk) parseTextIncoming(texts ...string) (parsed string, mentions []string) {
	clean := make([]string, 0, len(texts))
	for i := range texts {

		txt := reEntity.ReplaceAllStringFunc(
			html.UnescapeString(texts[i]),
			func(str string) string {
				m := reEntityRepl.FindStringSubmatch(str)
				if len(m) != 3 {
					return str
				}

				var entity Entity
				switch m[1] {
				case "@":
					entity = s.user(m[2])
				case "#":
					entity = s.channel(m[2])
				}

				if !entity.IsNil() {
					mentions = append(mentions, entity.Name())
					return entity.QualifiedName()
				}

				return str
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
