package cmd

type AppCommand struct {
}

func (c *AppCommand) Run() error {
	return nil
}

func (c *AppCommand) Info() *Info {
	return &Info{Name: "apps"}
}
