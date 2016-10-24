package output

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/frizinak/slek/slk"
)

var (
	colorRed   = "\033[1;31m"
	colorGreen = "\033[0;32m"
	colorBlue  = "\033[1;34m"
	colorGray  = "\033[0;37m"

	colorBgRed    = "\033[1;41m"
	colorBgGreen  = "\033[1;30;42m"
	colorBgBlue   = "\033[1;30;44m"
	colorBgYellow = "\033[1;30;43m"
	colorBgGray   = "\033[1;30;47m"

	colorReset = "\033[0m"
)

type msgPrefix struct {
	channel string
	from    string
	ts      time.Time
}

type format struct {
	ownUsername string
	lastPrefix  *msgPrefix
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

func (t *format) List(title string, items []*slk.ListItem) string {
	l := make([]string, len(items))
	for i, item := range items {
		l[i] = fmt.Sprintf("%s %s", status[item.Status], item.Value)
	}

	return fmt.Sprintf(
		"%s %s %s\n%s",
		colorBgGreen,
		title,
		colorReset,
		strings.Join(l, "\n"),
	)
}
