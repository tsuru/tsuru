package cmd

type AppCommand struct {
	Name string
}

func (c *AppCommand) Run() error {
	return nil
}

func (c *AppCommand) Info() *Info {
	return &Info{Name: c.Name}
}
