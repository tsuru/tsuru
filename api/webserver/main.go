package main

import (
	"flag"
	"github.com/bmizerany/pat"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/api/auth"
	"github.com/timeredbull/tsuru/api/service"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/log"
	"github.com/timeredbull/tsuru/repository"
	stdlog "log"
	"log/syslog"
	"net/http"
)

func main() {
	var err error
	log.Target, err = syslog.NewLogger(syslog.LOG_INFO, stdlog.LstdFlags)
	if err != nil {
		panic(err)
	}
	configFile := flag.String("config", "/etc/tsuru/tsuru.conf", "tsuru config file")
	dry := flag.Bool("dry", false, "dry-run: does not start the server (for testing purpose)")
	flag.Parse()
	err = config.ReadConfigFile(*configFile)
	if err != nil {
		log.Panic(err.Error())
	}
	connString, err := config.GetString("database:url")
	if err != nil {
		panic(err)
	}
	dbName, err := config.GetString("database:name")
	if err != nil {
		panic(err)
	}
	db.Session, err = db.Open(connString, dbName)
	if err != nil {
		log.Panic(err.Error())
	}
	defer db.Session.Close()

	repository.RunAgent()
	m := pat.New()

	m.Post("/services", AuthorizationRequiredHandler(service.CreateHandler))
	m.Post("/services/instances", AuthorizationRequiredHandler(service.CreateInstanceHandler))
	m.Get("/services", AuthorizationRequiredHandler(service.ServicesHandler))
	m.Del("/services/:name", AuthorizationRequiredHandler(service.DeleteHandler))
	// m.Post("/services/bind", AuthorizationRequiredHandler(service.BindHandler))
	// m.Post("/services/unbind", AuthorizationRequiredHandler(service.UnbindHandler))
	m.Put("/services/:service/:team", AuthorizationRequiredHandler(service.GrantAccessToTeamHandler))
	m.Del("/services/:service/:team", AuthorizationRequiredHandler(service.RevokeAccessFromTeamHandler))

	m.Del("/apps/:name", AuthorizationRequiredHandler(app.AppDelete))
	m.Get("/apps/:name/repository/clone", Handler(app.CloneRepositoryHandler))
	m.Get("/apps/:name", AuthorizationRequiredHandler(app.AppInfo))
	m.Post("/apps/:name/run", AuthorizationRequiredHandler(app.RunCommand))
	m.Get("/apps/:name/env", AuthorizationRequiredHandler(app.GetEnv))
	m.Post("/apps/:name/env", AuthorizationRequiredHandler(app.SetEnv))
	m.Del("/apps/:name/env", AuthorizationRequiredHandler(app.UnsetEnv))
	m.Get("/apps", AuthorizationRequiredHandler(app.AppList))
	m.Post("/apps", AuthorizationRequiredHandler(app.CreateAppHandler))
	m.Put("/apps/:app/:team", AuthorizationRequiredHandler(app.GrantAccessToTeamHandler))
	m.Del("/apps/:app/:team", AuthorizationRequiredHandler(app.RevokeAccessFromTeamHandler))
	m.Get("/apps/:name/log", AuthorizationRequiredHandler(app.AppLog))

	m.Post("/users", Handler(auth.CreateUser))
	m.Post("/users/:email/tokens", Handler(auth.Login))
	m.Post("/users/keys", AuthorizationRequiredHandler(auth.AddKeyToUser))
	m.Del("/users/keys", AuthorizationRequiredHandler(auth.RemoveKeyFromUser))

	m.Post("/teams", AuthorizationRequiredHandler(auth.CreateTeam))
	m.Put("/teams/:team/:user", AuthorizationRequiredHandler(auth.AddUserToTeam))
	m.Del("/teams/:team/:user", AuthorizationRequiredHandler(auth.RemoveUserFromTeam))

	listen, err := config.GetString("listen")
	if err != nil {
		panic(err)
	}
	if !*dry {
		log.Fatal(http.ListenAndServe(listen, m))
	}
}
