package main

import (
	"encoding/json"

	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/executil"
	"github.com/ghetzel/go-stockutil/sliceutil"
)

type ShellConfig struct {
	ID      string                 `yaml:"id"`
	Command []string               `yaml:"command"`
	Env     map[string]interface{} `yaml:"env"`
	Shell   bool                   `yaml:"shell"`
}

func (self *ShellConfig) Key() string {
	return self.ID
}

func (self *ShellConfig) Do(page *Page) (interface{}, error) {
	defaults.SetDefaults(self)

	var cmd *executil.Cmd
	var args = sliceutil.CompactString(self.Command)

	for i, arg := range args {
		if v, err := page.eval(arg); err == nil {
			args[i] = v.String()
		}
	}

	if self.Shell {
		cmd = executil.ShellCommand(
			executil.Join(args),
		)
	} else {
		cmd = executil.Command(
			args[0],
			args[1:]...,
		)
	}

	cmd.InheritEnv = true

	for k, v := range self.Env {
		cmd.SetEnv(k, v)
	}

	if raw, err := cmd.Output(); err == nil {
		var out interface{}

		if err := json.Unmarshal(raw, &out); err == nil {
			return out, nil
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}
