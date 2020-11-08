package main

import (
	"encoding/json"

	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/executil"
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

func (self *ShellConfig) Do() (interface{}, error) {
	defaults.SetDefaults(self)

	var cmd *executil.Cmd

	if self.Shell {
		cmd = executil.ShellCommand(
			executil.Join(self.Command),
		)
	} else {
		cmd = executil.Command(
			self.Command[0],
			self.Command[1:]...,
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
