package apps

type App struct {
	Name string
	Framework string
	State string
}

func (app *App) Create() error {
	app.State = "Pending"
	return nil
}
