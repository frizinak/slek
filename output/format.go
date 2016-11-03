package output

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/slek/slk"
	"github.com/mitchellh/go-wordwrap"
)

const (
	colorRed   = "\033[1;31m"
	colorGreen = "\033[0;32m"
	colorBlue  = "\033[1;34m"
	colorGray  = "\033[0;37m"

	colorBgRed    = "\033[1;41m"
	colorBgGreen  = "\033[1;30;42m"
	colorBgBlue   = "\033[1;30;44m"
	colorBgYellow = "\033[1;30;43m"
	colorBgGray   = "\033[1;30;47m"

	colorBold   = "\033[1m"
	colorItalic = "\033[32m"

	colorReset = "\033[0m"
)

var (
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reCode       = regexp.MustCompile("(?s)\n?```(.*?)```\n?")

	markups = []*markup{
		// _italic_
		&markup{
			re:         regexp.MustCompile("(^|\\s)_([^_]+)_(\\s|$)"),
			repl:       "$1\001[1]$2\001[2]$3",
			prefixRepl: "\001[1]",
			suffixRepl: "\001[2]",
			color:      colorItalic,
			colorEnd:   colorReset,
			prefix:     "_",
			suffix:     "_",
		},
		// *bold*
		&markup{
			re:         regexp.MustCompile("(^|\\s)\\*([^*]+)\\*(\\s|$)"),
			repl:       "$1\001[3]$2\001[4]$3",
			prefixRepl: "\001[3]",
			suffixRepl: "\001[4]",
			color:      colorBold,
			colorEnd:   colorReset,
			prefix:     "*",
			suffix:     "*",
		},
		// > quote
		&markup{
			re: regexp.MustCompile("(?s)(^|\n)(\\s*)>>>(.*)$"),
			replFunc: func(m *markup, str string) string {
				matches := m.re.FindStringSubmatch(str)
				lines := strings.Split(matches[3], "\n")
				for i := range lines {
					lines[i] = "\001[5]\001[6]" + strings.TrimSpace(lines[i])
				}

				return matches[1] + matches[2] + strings.Join(lines, "\n")
			},
			prefixRepl: "\001[5]",
			suffixRepl: "\001[6]",
			color:      " " + colorBgGreen + " ",
			colorEnd:   colorReset + " ",
			prefix:     ">>>",
			suffix:     "",
		},
		// >>> multiline quote
		&markup{
			re:         regexp.MustCompile("(?m)^(\001\\[5\\]\001\\[6\\])?(\\s*)>\\s?(.*)$"),
			repl:       "$1\001[7]\001[8]$3",
			prefixRepl: "\001[7]",
			suffixRepl: "\001[8]",
			color:      " " + colorBgGreen + " ",
			colorEnd:   colorReset + " ",
			prefix:     ">",
			suffix:     "",
		},
	}
)

type markup struct {
	re         *regexp.Regexp
	repl       string
	replFunc   func(*markup, string) string
	prefixRepl string
	suffixRepl string
	color      string
	colorEnd   string
	prefix     string
	suffix     string
}

type msgPrefix struct {
	channel string
	from    string
	ts      time.Time
}

type format struct {
	ownUsername string
	lastPrefix  *msgPrefix
}

func (t *format) setUsername(username string) {
	t.ownUsername = username
}

func (t *format) wrap(str string, len uint) string {
	return wordwrap.WrapString(str, len)
}

func (t *format) Info(msg string) string {
	t.lastPrefix = nil
	return fmt.Sprintf("%s %s %s", colorBgGreen, msg, colorReset)
}
func (t *format) Notice(msg string) string {
	t.lastPrefix = nil
	return fmt.Sprintf("%s %s %s", colorBgYellow, msg, colorReset)
}
func (t *format) Warn(msg string) string {
	t.lastPrefix = nil
	return fmt.Sprintf("%s %s %s", colorBgRed, msg, colorReset)
}

func (t *format) Msg(
	channel,
	from,
	msg string,
	ts time.Time,
	section bool,
) string {
	colorUser := colorBgBlue
	if from == t.ownUsername {
		colorUser = colorBgGray
	}

	// TODO This is filthy, use a proper markdown parser or just
	// ... better code.
	// Anyway we replace the regexes with \001[\d]
	// remove them if inside a code block or replace them with their
	// respective colors.
	for _, m := range markups {
		if m.replFunc != nil {
			msg = m.re.ReplaceAllStringFunc(
				msg,
				func(m *markup) func(str string) string {
					return func(str string) string {
						return m.replFunc(m, str)
					}
				}(m),
			)

			continue
		}

		msg = m.re.ReplaceAllString(msg, m.repl)
	}

	cleanMarkup := func(str string) string {
		for _, m := range markups {
			str = strings.Replace(str, m.prefixRepl, m.prefix, -1)
			str = strings.Replace(str, m.suffixRepl, m.suffix, -1)
		}

		return str
	}

	msg = reCode.ReplaceAllStringFunc(
		msg,
		func(str string) string {
			m := reCode.FindStringSubmatch(str)
			str = cleanMarkup(strings.Trim(m[1], "\n"))
			lines := strings.Split(str, "\n")
			l := 0
			for i := range lines {
				if len(lines[i]) > l {
					l = len(lines[i])
				}
			}
			for i := range lines {
				lines[i] = fmt.Sprintf(" %-"+strconv.Itoa(l)+"s ", lines[i])
			}

			return fmt.Sprintf(
				"\n%s%s%s\n",
				colorBgGray,
				strings.Join(lines, "\n"),
				colorReset,
			)
		},
	)

	msg = reInlineCode.ReplaceAllStringFunc(
		msg,
		func(str string) string {
			str = cleanMarkup(str)
			return colorBgGray + str[1:len(str)-1] + colorReset
		},
	)

	for _, m := range markups {
		msg = strings.Replace(msg, m.prefixRepl, m.color, -1)
		msg = strings.Replace(msg, m.suffixRepl, m.colorEnd, -1)
	}

	msg = strings.Trim(msg, "\n")

	if section ||
		t.lastPrefix == nil ||
		t.lastPrefix.channel != channel ||
		t.lastPrefix.from != from ||
		math.Abs(ts.Sub(t.lastPrefix.ts).Seconds()) > float64(time.Minute*5) {

		prefix := ""
		if section || t.lastPrefix == nil || t.lastPrefix.channel != channel {
			prefix = fmt.Sprintf(
				"\n%s %-34s%s",
				colorBgGreen,
				channel,
				colorReset,
			)
		}

		prefix += fmt.Sprintf(
			"\n%s %-18s %s %s",
			colorUser,
			fmt.Sprintf("%s:", from),
			colorReset,
			ts.Format("02/01 15:04:05"),
		)

		t.lastPrefix = &msgPrefix{channel, from, ts}
		return fmt.Sprintf(
			"%s\n%s",
			prefix,
			msg,
		)
	}

	return msg
}

func (t *format) File(channel, from, title, url string) string {
	// TODO
	//return t.Msg(channel, from, msg)
	t.lastPrefix = nil
	return fmt.Sprintf(
		"%-18s %s%-12s%s%s%s%s",
		fmt.Sprintf("[%s]", channel),
		colorRed,
		fmt.Sprintf("%s's file: ", from),
		colorGreen,
		title,
		url,
		colorReset,
	)
}

func (t *format) Typing(channel, user string) string {
	return fmt.Sprintf(
		"%s %s is typing...%s",
		fmt.Sprintf("[%s]", channel),
		user,
		colorReset,
	)
}

func (t *format) Debug(msg ...string) string {
	f, _ := os.OpenFile("slk-debug.log", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0755)
	defer f.Close()
	m := fmt.Sprintf(
		"DEBUG: %s%s",
		strings.Join(msg, " "),
		colorReset,
	)

	f.WriteString(m + "\n")
	return m
}

var status = map[int]string{
	slk.ListItemStatusGood:   fmt.Sprintf("%s●%s", colorGreen, colorReset),
	slk.ListItemStatusNormal: fmt.Sprintf("%s●%s", colorGray, colorReset),
	slk.ListItemStatusBad:    fmt.Sprintf("%s●%s", colorRed, colorReset),
}

func (t *format) List(
	list slk.ListItems,
	reverse bool,
) string {
	l := make([]string, 0, len(list))
	i := 0
	e := len(list)
	diff := 1
	if reverse {
		i, e = e-1, i-1
		diff = -1
	}

	for ; i != e; i += diff {
		s := list[i].Status
		if s == slk.ListItemStatusTitle {
			l = append(
				l,
				fmt.Sprintf(
					"%s %s %s",
					colorBgGreen,
					list[i].Value,
					colorReset,
				),
			)
			continue
		}

		l = append(
			l,
			fmt.Sprintf("%s %s", status[s], list[i].Value),
		)
	}

	return strings.Join(l, "\n")
}
