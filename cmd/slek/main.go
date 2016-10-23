package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frizinak/slek/cmd/config"
	"github.com/frizinak/slek/output"
	"github.com/frizinak/slek/slk"
)

var (
	stderr = log.New(os.Stderr, "", 0)
	help   = []slk.ListItems{
		{
			{slk.ListItemStatusNone, "quit             : quit slek"},
			{slk.ListItemStatusNone, "exit             : quit slek"},
		},
		{
			{slk.ListItemStatusNone, "@user    msg     :  im user"},
			{slk.ListItemStatusNone, "#channel msg     :  post channel message"},
			{slk.ListItemStatusNone, "#group   msg     :  post group message"},
		},
		{
			{slk.ListItemStatusNone, "@user    /h (n)  :  get history (n items)"},
			{slk.ListItemStatusNone, "#channel /h (n)  :  get history (n items)"},
			{slk.ListItemStatusNone, "#group   /h (n)  :  get history (n items)"},
		},
		{
			{slk.ListItemStatusNone, "users | u        :  list active users"},
			{slk.ListItemStatusNone, "all-users | au   :  list all users"},
			{slk.ListItemStatusNone, "channels | c     :  list joined channels"},
			{slk.ListItemStatusNone, "all-channels | ac:  list all channels"},
		},
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

func newSlek(username, token, editorCmd string) *slek {
	t, input := output.NewTerm("slk", username, time.Second*5)
	c := slk.NewSlk(
		username,
		token,
		t,
	)

	return &slek{c, t, input, strings.TrimSpace(editorCmd), make(chan bool)}
}

func (s *slek) fuzzy(
	eType slk.EntityType,
	query,
	prefix string,
	args []string,
) slk.Entity {
	opts := s.c.Fuzzy(eType, query)

	if eType == "" {
		s.t.Notice(fmt.Sprintf("No type '%s'", eType))
		return nil
	}

	if len(opts) == 1 {
		// autocomplete and bail
		s.t.SetInput(fmt.Sprintf("%s%s ", prefix, opts[0].GetName()), -1, false)
		return opts[0]
	}

	for i := range opts {
		// multiple matches
		if query == opts[i].GetName() {
			s.t.SetInput(
				fmt.Sprintf("%s%s ", prefix, opts[0].GetName()),
				-1,
				false,
			)
			return opts[i]
		}
	}

	if len(opts) == 0 {
		s.t.Notice(fmt.Sprintf("No such %s", eType))
	} else if len(opts) > 1 {
		names := make([]string, 0, len(opts))
		for i := range opts {
			names = append(names, opts[i].GetName())
		}
		s.t.Notice(
			fmt.Sprintf(
				"Did you mean any of: %s?",
				strings.Join(names, ", "),
			),
		)

		trgt := fmt.Sprintf("%s%s", prefix, opts[0].GetName())
		s.t.SetInput(
			fmt.Sprintf("%s %s", trgt, trimFields(args)),
			// TODO counting bytes bruh, @see term.go todo
			len(trgt),
			false,
		)
	}

	return nil
}

func (s *slek) normalCommand(cmd string, args []string) bool {
	switch cmd {
	case "?", "h", "help", "/help":
		s.t.Notice("HELP")
		helps := []string{"General", "Messages", "History", "Listings"}
		for i, title := range helps {
			s.t.List(title, help[i])
		}
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

func (s *slek) editor(prefix string) {
	if s.editorCmd == "" {
		s.t.Warn("No editor command defined")
	}

	file, err := ioutil.TempFile(os.TempDir(), "slek-edit-")
	if file != nil {
		defer os.Remove(file.Name())
		defer file.Close()
	}

	if err != nil {
		s.t.Warn(err.Error())
		return
	}

	file.WriteString(prefix)

	cmd := strings.Replace(s.editorCmd, "{}", file.Name(), -1)

	c := exec.Command("sh", "-c", cmd)
	if err := c.Run(); err != nil {
		s.t.Warn(fmt.Sprintf("Editor command failed: %s", err.Error()))
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		s.t.Warn(
			fmt.Sprintf(
				"Editor command succeeded but could not seek to beginning of file: %s",
				err.Error(),
			),
		)
		return
	}

	data, err := ioutil.ReadAll(file)
	if err != nil {
		s.t.Warn(
			fmt.Sprintf(
				"Editor command succeeded but could not read temp file: %s",
				err.Error(),
			),
		)
		return
	}

	s.t.SetInput(string(data), -1, true)
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

	var init sync.Mutex
	init.Lock()
	go func() {
		if err := s.c.Init(); err != nil {
			slkErr <- err
			return
		}

		for _, e := range s.c.Joined() {
			s.c.Unread(e)
		}

		for _, e := range s.c.IMs() {
			s.c.Unread(e)
		}

		init.Unlock()
		slkErr <- s.c.Run()
	}()

	types := map[byte]slk.EntityType{
		'@': slk.TypeUser,
		'#': slk.TypeChannel,
	}

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

			e := s.fuzzy(eType, cmd[1:], string(cmd[0]), args)
			if e == nil {
				continue
			}

			s.t.SetInput(
				fmt.Sprintf("%s%s ", string(cmd[0]), e.GetName()),
				-1,
				false,
			)

			if s.entityCommand(e, args) {
				continue
			}

			s.c.Post(e, trimFields(args))
		}
	}()

	// We can't exit before starting slack.
	s.t.Notice("Fetching metadata")
	init.Lock()
	s.t.Info("Fetched metadata")

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

	return nil
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

	err = newSlek(conf.Username, conf.Token, conf.EditorCmd).run()
	if err != nil {
		stderr.Fatal(err)
	}
}
