package main

import (
	"flag"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/frizinak/slek/cmd/config"
	"github.com/frizinak/slek/output"
	"github.com/frizinak/slek/slk"
)

var stderr = log.New(os.Stderr, "", 0)

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

	if conf.TimeFormat == "" {
		conf.TimeFormat = "Jan 02 15:04:05"
	}

	t := output.NewStdout("", conf.TimeFormat)
	c := slk.NewSlk(
		conf.Token,
		conf.TimeFormat,
		t,
	)

	if err := c.Init(); err != nil {
		panic(err)
	}

	t.SetUsername(c.Username())

	if err := c.Run(); err != nil {
		panic(err)
	}
}
