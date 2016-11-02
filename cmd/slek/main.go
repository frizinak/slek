package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/slek/cmd/config"
	"github.com/frizinak/slek/output"
	"github.com/frizinak/slek/slk"
	"github.com/jroimartin/gocui"
)

var (
	stderr = log.New(os.Stderr, "", 0)
	help   = slk.ListItems{
		{slk.ListItemStatusTitle, "HELP (#room = @user #group or #channel)"},

		{slk.ListItemStatusTitle, "General"},
		{slk.ListItemStatusNone, "quit             : quit slek"},
		{slk.ListItemStatusNone, "exit             : quit slek"},

		{slk.ListItemStatusTitle, "Messages"},
		{slk.ListItemStatusNone, "#room <msg>: send <msg>"},

		{slk.ListItemStatusTitle, "Rooms"},
		{slk.ListItemStatusNone, "#room /h <n>          : get history (<n> items)"},
		{slk.ListItemStatusNone, "#room /u | /users:    : list online users in #room"},
		{slk.ListItemStatusNone, "#room /au | /all-users: list all users in #room"},
		{slk.ListItemStatusNone, "#room /p | /pins      : list pins of #room"},
		{slk.ListItemStatusNone, "#room /f | /files     : list files of #room"},
		{slk.ListItemStatusNone, "#room !path <comment> : upload file to #room"},

		{slk.ListItemStatusTitle, "Listings"},
		{slk.ListItemStatusNone, "users | u        : list online users"},
		{slk.ListItemStatusNone, "all-users | au   : list all users"},
		{slk.ListItemStatusNone, "channels | c     : list joined channels"},
		{slk.ListItemStatusNone, "all-channels | ac: list all channels"},
	}
)

func isSpace(r rune) bool {
	return r == ' '
}

func trimFields(i []string) string {
	return strings.TrimSpace(strings.Join(i, " "))
}

type slek struct {
	c         *slk.Slk
	t         *output.Term
	input     chan string
	editorCmd string
	quit      chan bool
}

func newSlek(token, editorCmd string, ntfy time.Duration) *slek {
	t, input := output.NewTerm("slk", "", time.Second*5, ntfy)
	c := slk.NewSlk(
		token,
		t,
	)

	return &slek{c, t, input, strings.TrimSpace(editorCmd), make(chan bool)}
}

func (s *slek) normalCommand(cmd string, args []string) bool {
	switch cmd {
	case "?", "h", "help", "/help":
		s.t.List(help, false)
	case "quit", "exit":
		s.quit <- true
		return true
	case "channels", "c":
		s.c.List(slk.TypeChannel, true)
		return true
	case "all-channels", "ac":
		s.c.List(slk.TypeChannel, false)
		return true
	case "users", "u":
		s.c.List(slk.TypeUser, true)
		return true
	case "all-users", "au":
		s.c.List(slk.TypeUser, false)
		return true
	}
	return false
}

func (s *slek) entityCommand(e slk.Entity, args []string) bool {
	if len(args) == 0 {
		return true
	}

	switch {
	case len(args[0]) > 0 && args[0][0] == '!':
		i := 1
		query := args[0][1:]
		for ; i < len(args); i++ {
			if args[i-1][len(args[i-1])-1:] != "\\" {
				break
			}

			query += " " + args[i]
		}

		if len(args) < i+1 {
			args = []string{}
		} else {
			args = args[i:]
		}

		comment := trimFields(args)
		path := s.fuzzyPath(
			e.GetQualifiedName()+" !",
			query,
			comment,
		)

		if path != "" {
			s.c.Upload(e, path, "", comment)
		}

		return true
	}

	switch args[0] {
	case "/history", "/hist", "/h":
		var n int
		if len(args) > 1 {
			n, _ = strconv.Atoi(args[1])
		}

		if n == 0 || n > 100 {
			n = 10
		}

		s.c.History(e, n)
		return true
	case "/p", "/pins":
		s.c.Pins(e)
		return true
	case "/f", "/files":
		s.c.Uploads(e)
		return true
	case "/u", "/users":
		s.c.Members(e, true)
		return true
	case "/au", "/all-users":
		s.c.Members(e, false)
		return true
	case "/join":
		s.c.Join(e)
		return true
	case "/leave":
		s.c.Leave(e)
		return true
	case "/e":
		s.editor(e.GetQualifiedName() + " ")
		return true
	}

	if args[0][0] == '/' {
		s.t.Warn("No such command")
		return true
	}

	return false
}

func (s *slek) run() error {
	if err := s.t.Init(); err != nil {
		return err
	}

	termErr := make(chan error, 0)
	go func() {
		termErr <- s.t.Run()
	}()

	slkErr := make(chan error, 0)

	go func() {
		s.t.Notice("Connecting...")
		if err := s.c.Init(); err != nil {
			slkErr <- err
			return
		}
		s.t.SetUsername(s.c.GetUsername())

		s.t.Notice("Fetching history...")
		for _, e := range s.c.Joined() {
			s.c.Unread(e)
		}

		for _, e := range s.c.IMs() {
			s.c.Unread(e)
		}

		slkErr <- s.c.Run()
	}()

	types := map[byte]slk.EntityType{
		'@': slk.TypeUser,
		'#': slk.TypeChannel,
	}

	s.t.BindKey(gocui.KeyCtrlE, func() error {
		s.editor(s.t.GetInput())
		return nil
	})

	go func() {
		for i := range s.input {
			args := strings.FieldsFunc(strings.TrimSpace(string(i)), isSpace)
			if len(args) == 0 {
				continue
			}

			cmd := args[0]
			args = args[1:]

			eType := types[cmd[0]]

			if eType == "" {
				s.normalCommand(cmd, args)
				continue
			}

			e := s.fuzzy(eType, cmd[1:], args)
			if e == nil {
				continue
			}

			s.t.SetInput(
				fmt.Sprintf("%s%s ", string(cmd[0]), e.GetName()),
				-1,
				-1,
				false,
			)

			if s.entityCommand(e, args) {
				continue
			}

			msg := trimFields(args)
			if err := s.c.Post(e, msg); err != nil {
				s.t.SetInput(msg, -1, -1, false)
			}
		}
	}()

	for {
		select {
		case err := <-slkErr:
			s.t.Quit()
			<-termErr
			return err
		case err := <-termErr:
			s.c.Quit()
			return err
		case <-s.quit:
			s.t.Quit()
		}
	}
}

func main() {
	var defaultFile string
	if u, err := user.Current(); err == nil {
		defaultFile = filepath.Join(u.HomeDir, ".slek")
	}

	flFile := flag.String("c", defaultFile, "Path to slek config file")
	flag.Parse()
	file := *flFile
	conf, err := config.Run(file, file == defaultFile && defaultFile != "")
	if err != nil {
		if err == config.ErrFirstRun {
			return
		}

		stderr.Fatal(err)
	}

	if conf.NotificationTimeout == 0 {
		conf.NotificationTimeout = 2500
	}

	ntfy := time.Duration(conf.NotificationTimeout * 1e6)
	err = newSlek(conf.Token, conf.EditorCmd, ntfy).run()
	if err != nil {
		stderr.Fatal(err)
	}
}
