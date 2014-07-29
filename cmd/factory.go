package cmd

import (
	"errors"
)

type Factory interface {
	Create(name string) (Cmd, error)
}

type factory struct {
	commands map[string]Cmd
}

func NewFactory() Factory {
	return &factory{
		commands: map[string]Cmd{
			"deployment": NewDeploymentCmd(),
		},
	}
}

func (f *factory) Create(name string) (Cmd, error) {
	if f.commands[name] == nil {
		return nil, errors.New("Invalid command name")
	}

	return f.commands[name], nil
}
