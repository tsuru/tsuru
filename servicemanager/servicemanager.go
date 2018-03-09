package servicemanager

import (
	"github.com/tsuru/tsuru/types/app"
	"github.com/tsuru/tsuru/types/auth"
)

var (
	Team auth.TeamService
	Plan app.PlanService
)
