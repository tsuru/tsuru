package api

import (
	"fmt"
	"net"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/luizbafilho/fusis/api/types"
)

// ApiService ...
type ApiService struct {
	*gin.Engine
	balancer Balancer
	env      string
}

type Balancer interface {
	GetServices() []types.Service
	AddService(*types.Service) error
	GetService(string) (*types.Service, error)
	DeleteService(string) error
	AddDestination(*types.Service, *types.Destination) error
	GetDestination(string) (*types.Destination, error)
	DeleteDestination(*types.Destination) error
	IsLeader() bool
	GetLeader() string
}

//NewAPI ...
func NewAPI(balancer Balancer) ApiService {
	gin.SetMode(gin.ReleaseMode)
	as := ApiService{
		Engine:   gin.Default(),
		balancer: balancer,
		env:      getEnv(),
	}

	as.registerRedirectMiddleware()
	as.registerRoutes()
	return as
}

func (as ApiService) registerRoutes() {
	as.GET("/services", as.serviceList)
	as.GET("/services/:service_name", as.serviceGet)
	as.POST("/services", as.serviceCreate)
	as.DELETE("/services/:service_name", as.serviceDelete)
	as.POST("/services/:service_name/destinations", as.destinationCreate)
	as.DELETE("/services/:service_name/destinations/:destination_name", as.destinationDelete)
}

func redirectMiddleware(b Balancer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if b.IsLeader() {
			c.Next()
		} else {
			c.Abort()

			host, _, _ := net.SplitHostPort(b.GetLeader())
			c.Redirect(307, fmt.Sprintf("http://%s:8000%s", host, c.Request.URL))
		}
	}
}

func (as ApiService) registerRedirectMiddleware() {
	as.Use(redirectMiddleware(as.balancer))
}

func (as ApiService) Serve() {
	as.Run("0.0.0.0:8000")
}

func getEnv() string {
	env := os.Getenv("FUSIS_ENV")
	if env == "" {
		env = "development"
	}
	return env
}
