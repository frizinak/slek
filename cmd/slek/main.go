package main

import (
	"flag"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/frizinak/gocui"
	"github.com/frizinak/slek/cmd/config"
	"github.com/frizinak/slek/cmd/slek/assets"
	"github.com/frizinak/slek/output"
	"github.com/frizinak/slek/slk"
)

var (
	stderr = log.New(os.Stderr, "", 0)
	help   = slk.ListItems{
		{slk.ListItemStatusTitle, "HELP (#room = @user #group or #channel)"},

		{slk.ListItemStatusTitle, "General"},
		{slk.ListItemStatusNone, "quit             : quit slek"},
		{slk.ListItemStatusNone, "exit             : quit slek"},
		{slk.ListItemStatusNone, "about            : about slek"},
		{slk.ListItemStatusNone, "clear            : clear the chat screen"},

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
		{slk.ListItemStatusNone, "unread | ur      : list rooms with unread messages"},
		{slk.ListItemStatusNone, "users | u        : list online users"},
		{slk.ListItemStatusNone, "all-users | au   : list all users"},
		{slk.ListItemStatusNone, "channels | c     : list joined channels"},
		{slk.ListItemStatusNone, "all-channels | ac: list all channels"},

		{slk.ListItemStatusTitle, "Keybinds"},
		{slk.ListItemStatusNone, "<C-q>: quit"},
		{slk.ListItemStatusNone, "<C-e>: spawn editor command"},
		{slk.ListItemStatusNone, "<C-u>: go to random room with unread messages"},
	}
)

func isSpace(r rune) bool {
	return r == ' '
}

func trimFields(i []string) string {
	return strings.TrimSpace(strings.Join(i, " "))
}

func getIcon() string {
	raw, err := assets.Asset("slek.png")
	if err != nil {
		return ""
	}

	iconPath := path.Join(os.TempDir(), "slek-icon-iHFyCPH8eQ.png")
	f, err := os.Create(iconPath)
	if f != nil {
		defer f.Close()
	}

	if err != nil {
		return ""
	}

	_, err = f.Write(raw)
	if err != nil {
		return ""
	}

	return iconPath
}

type slek struct {
	c         *slk.Slk
	t         *output.Term
	input     chan string
	editorCmd string
	quit      chan bool
}

func newSlek(token, tFormat, editorCmd string, ntfy time.Duration) *slek {

	t, input := output.NewTerm(
		"slek",
		getIcon(),
		"",
		tFormat,
		time.Second*5,
		ntfy,
	)
	c := slk.NewSlk(
		token,
		tFormat,
		t,
	)

	return &slek{c, t, input, strings.TrimSpace(editorCmd), make(chan bool)}
}

func (s *slek) normalCommand(cmd string, args []string) bool {
	switch cmd {
	case "?", "h", "help", "/help":
		s.t.List(help, false)
	case "about":
		about, err := assets.Asset("about")
		if err != nil {
			s.t.Warn(err.Error())
			return true
		}

		s.t.Meta(string(about))
		return true
	case "quit", "exit":
		s.quit <- true
		return true
	case "clear":
		s.t.Clear()
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

	case "unread", "ur":
		s.c.ListUnread()
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
			e.QualifiedName()+" !",
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
		s.editor(e.QualifiedName() + " ")
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
		s.t.SetUsername(s.c.Username())
		slkErr <- s.c.Run()
	}()

	types := map[byte]slk.EntityType{
		'@': slk.TypeUser,
		'#': slk.TypeChannel,
	}

	s.t.BindKey(gocui.KeyCtrlE, func() error {
		s.editor(s.t.Input())
		return nil
	})

	s.t.BindKey(gocui.KeyCtrlU, func() error {
		e, err := s.c.NextUnread()
		if err != nil {
			s.t.Notice(err.Error())
			return nil
		}

		s.c.Switch(e)
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

			// cmd == cmd[0] == a valid entity prefix.
			if len(cmd) == 1 && len(args) == 0 {
				if active, err := s.c.Active(); err == nil {
					s.t.SetInput(
						active.QualifiedName()+" ",
						-1,
						-1,
						false,
					)
					continue
				}
			}

			e := s.fuzzy(eType, cmd[1:], args)
			if e == nil {
				continue
			}

			s.c.Switch(e)

			s.t.SetInput(
				e.QualifiedName()+" ",
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
	if conf.TimeFormat == "" {
		conf.TimeFormat = "Jan 02 15:04:05"
	}

	ntfy := time.Duration(conf.NotificationTimeout * 1e6)
	err = newSlek(conf.Token, conf.TimeFormat, conf.EditorCmd, ntfy).run()
	if err != nil {
		stderr.Fatal(err)
	}
}
