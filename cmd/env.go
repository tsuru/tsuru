package cmd

type Env struct{}

func (c *Env) Info() *Info {
	return &Info{
		Name:  "env",
		Usage: "env (get|set|unset)",
		Desc:  "manage instance's environment variables.",
	}
}

type EnvGet struct{}

func (c *EnvGet) Info() *Info {
	return &Info{
		Name:  "get",
		Usage: "env get appname envname",
		Desc:  "retrieve environment variables for an app.",
	}
}
