package output

import (
	"log"
	"os"
	"time"

	"github.com/frizinak/slek/slk"
)

var (
	std    = log.New(os.Stdout, "", log.LstdFlags)
	stderr = log.New(os.Stderr, "", log.LstdFlags)
)

type Stdout struct {
	format
}

func NewStdout(username string) *Stdout {
	return &Stdout{format{ownUsername: username}}
}

func (s *Stdout) Notify(channel, from, text string, force bool) {
	// noop
}

func (s *Stdout) Info(msg string) {
	std.Println(s.format.Info(msg))
}

func (s *Stdout) Notice(msg string) {
	std.Println(s.format.Notice(msg))
}

func (s *Stdout) Warn(msg string) {
	std.Println(s.format.Warn(msg))
}

func (s *Stdout) Msg(channel, from, msg string, ts time.Time, section bool) {
	std.Println(s.format.Msg(channel, from, msg, ts, section))
}

func (s *Stdout) File(channel, from, title, url string) {
	std.Println(s.format.File(channel, from, title, url))
}

func (s *Stdout) Typing(channel, user string, timeout time.Duration) {
	std.Println(s.format.Typing(channel, user))
}

func (s *Stdout) Debug(msg ...string) {
	stderr.Println(s.format.Debug(msg...))
}

func (s *Stdout) List(title string, items []*slk.ListItem) {
	std.Println(s.format.List(title, items))
}
