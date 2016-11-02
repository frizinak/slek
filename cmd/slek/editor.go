package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

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

	s.t.SetInput(string(data), -1, -1, true)
}
